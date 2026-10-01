// Package discordmail is Discord's Interactions Endpoint for the /re slash
// command: staff reply to a mail-routes-originated inquiry by using /re
// inside its forum thread, entirely over HTTP — no long-running bot
// process, matching Discord's Interactions model (see
// https://discord.com/developers/docs/interactions/receiving-and-responding).
package discordmail

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"sta-backend/internal/emailinquiries"
)

// interaction types/response types this handler cares about. Full list:
// https://discord.com/developers/docs/interactions/receiving-and-responding#interaction-object-interaction-type
const (
	interactionTypePing               = 1
	interactionTypeApplicationCommand = 2

	responseTypePong                             = 1
	responseTypeChannelMessageWithSource         = 4
	responseTypeDeferredChannelMessageWithSource = 5
	discordFollowupRequestTimeout                = 15 * time.Second
	discordFollowupBackgroundTimeout             = 30 * time.Second
)

// ReplyService is the one method this package needs from
// emailinquiries.Service — kept as an interface so this package doesn't
// import the concrete type, and so tests can fake it.
type ReplyService interface {
	ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (emailinquiries.Inquiry, error)
}

type Handler struct {
	publicKey ed25519.PublicKey
	reply     ReplyService
}

func NewHandler(publicKeyHex string, reply ReplyService) (*Handler, error) {
	if reply == nil {
		return nil, errors.New("discordmail handler dependencies are missing")
	}
	key, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("discord application public key is invalid")
	}
	return &Handler{publicKey: ed25519.PublicKey(key), reply: reply}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/internal/discord/interactions", h.interact)
}

func (h *Handler) interact(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !h.verifySignature(r, body) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var payload struct {
		Type          int    `json:"type"`
		Token         string `json:"token"`
		ApplicationID string `json:"application_id"`
		Channel       struct {
			ID string `json:"id"`
		} `json:"channel"`
		Member struct {
			User struct {
				ID         string `json:"id"`
				GlobalName string `json:"global_name"`
				Username   string `json:"username"`
			} `json:"user"`
		} `json:"member"`
		Data struct {
			Name    string `json:"name"`
			Options []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if payload.Type == interactionTypePing {
		writeJSON(w, map[string]any{"type": responseTypePong})
		return
	}

	if payload.Type != interactionTypeApplicationCommand || payload.Data.Name != "re" {
		writeJSON(w, interactionErrorResponse("這個指令不適用於這裡"))
		return
	}

	var message, signature string
	for _, opt := range payload.Data.Options {
		switch opt.Name {
		case "message":
			message = opt.Value
		case "signature":
			signature = opt.Value
		}
	}
	staffName := payload.Member.User.GlobalName
	if staffName == "" {
		staffName = payload.Member.User.Username
	}
	if staffName == "" {
		staffName = "工作人員"
	}

	// Discord gives us 3 seconds to ACK, but ReplyByDiscord's SMTP send
	// (plus DB writes) can take longer than that — especially on a cold
	// connection — which is what produced "the application did not
	// respond" even though the email actually went out. Deferring buys up
	// to 15 minutes via the followup-message endpoint instead of racing
	// the 3s deadline.
	writeJSON(w, map[string]any{
		"type": responseTypeDeferredChannelMessageWithSource,
		"data": map[string]any{"flags": 64},
	})
	go h.finishReply(payload.ApplicationID, payload.Token, payload.Channel.ID, staffName, payload.Member.User.ID, message, signature)
}

// finishReply does the actual work (ReplyByDiscord) after the interaction
// has already been ACKed, then edits the deferred response with the real
// result — runs on its own background context since the request that
// spawned it is already finished.
func (h *Handler) finishReply(applicationID, token, threadID, staffName, staffDiscordID, message, signature string) {
	ctx, cancel := context.WithTimeout(context.Background(), discordFollowupBackgroundTimeout)
	defer cancel()
	_, err := h.reply.ReplyByDiscord(ctx, threadID, staffName, staffDiscordID, message, signature)
	var content string
	switch {
	case err == nil:
		content = "✅ 回覆已寄出"
	case errors.Is(err, emailinquiries.ErrNotFound):
		// /re only makes sense inside a thread this system created — see
		// the package doc comment. Using it anywhere else (a random forum
		// post, a staff-only discussion channel, ...) has no inquiry to
		// resolve to, so there's nothing to reply to.
		content = "這個指令只能在信件討論串裡使用。"
	case errors.Is(err, emailinquiries.ErrInvalidInput):
		content = "回覆內容跟簽名都不能是空白。"
	default:
		content = "回覆寄送失敗，請稍後再試一次。"
	}
	h.editFollowupMessage(ctx, applicationID, token, content)
}

// editFollowupMessage replaces the deferred "..." placeholder Discord shows
// with the real outcome, via the same interaction-token-authenticated
// webhook endpoint Discord itself uses — no bot token required.
func (h *Handler) editFollowupMessage(ctx context.Context, applicationID, token, content string) {
	body, err := json.Marshal(map[string]any{"content": content})
	if err != nil {
		return
	}
	endpoint := "https://discord.com/api/v10/webhooks/" + url.PathEscape(applicationID) + "/" + url.PathEscape(token) + "/messages/@original"
	request, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: discordFollowupRequestTimeout}
	response, err := client.Do(request)
	if err != nil {
		return
	}
	_ = response.Body.Close()
}

func interactionErrorResponse(text string) map[string]any {
	return map[string]any{
		"type": responseTypeChannelMessageWithSource,
		"data": map[string]any{
			"content": text,
			"flags":   64, // ephemeral: only the invoking staff member sees this
		},
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) verifySignature(r *http.Request, body []byte) bool {
	signatureHex := r.Header.Get("X-Signature-Ed25519")
	timestamp := r.Header.Get("X-Signature-Timestamp")
	if signatureHex == "" || timestamp == "" {
		return false
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}
	message := append([]byte(timestamp), body...)
	return ed25519.Verify(h.publicKey, message, signature)
}
