// Package discordmail is Discord's Interactions Endpoint for the /re slash
// command: staff reply to a mail-routes-originated inquiry by using /re
// inside its forum thread, entirely over HTTP — no long-running bot
// process, matching Discord's Interactions model (see
// https://discord.com/developers/docs/interactions/receiving-and-responding).
//
// The full flow spans several separate interactions, since Discord has no
// single round trip that covers "open a form, let them review it, then
// act": /re opens a modal (a slash command's own text options are
// single-line, no way to type a real paragraph break — see the modal's
// doc comment) → submitting it stashes the draft (see DraftStore) and
// shows an ephemeral preview embed with 送出/編輯/刪除 buttons, so a reply
// can't fly out just from submitting the form → a button press is a
// further, independent interaction that looks the draft back up to
// actually send, edit in place, or discard it. 編輯 and 送出 both update
// that same preview message/embed rather than posting a new one each
// time — Discord supports responding to a modal submission with
// UPDATE_MESSAGE when the modal itself was opened from a button on that
// message, which is what lets 編輯 revise the same embed in place instead
// of leaving a trail of messages behind.
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
	"strings"
	"time"

	"sta-backend/internal/emailinquiries"
)

// interaction types/response types this handler cares about. Full list:
// https://discord.com/developers/docs/interactions/receiving-and-responding#interaction-object-interaction-type
const (
	interactionTypePing               = 1
	interactionTypeApplicationCommand = 2
	interactionTypeMessageComponent   = 3
	interactionTypeModalSubmit        = 5

	responseTypePong                             = 1
	responseTypeChannelMessageWithSource         = 4
	responseTypeDeferredChannelMessageWithSource = 5
	responseTypeDeferredUpdateMessage            = 6
	responseTypeUpdateMessage                    = 7
	responseTypeModal                            = 9

	discordFollowupRequestTimeout    = 15 * time.Second
	discordFollowupBackgroundTimeout = 30 * time.Second

	buttonStylePrimary = 1
	buttonStyleSuccess = 3
	buttonStyleDanger  = 4

	// Embed colors mirror the state of a reply: pending review, sent, or
	// failed to send (刪除 removes the message outright rather than
	// recoloring it — see handleDraftAction's "delete" case).
	embedColorPreview = 0xFEE75C // Discord's own "pending" yellow
	embedColorSent    = 0x57F287 // success green
	embedColorFailed  = 0xED4245 // danger red

	// replyModalCustomID identifies a fresh /re's modal submission (no
	// existing draft/message yet). An 編輯 reopen instead uses
	// replyModalCustomID + ":" + the draft id (see modalEditCustomID) so
	// its submission can be told apart and routed back to updating that
	// same draft/message in place.
	replyModalCustomID = "re_reply_modal"
	// draftCustomIDPrefix namespaces the 送出/編輯/刪除 buttons' custom_id
	// (draftCustomIDPrefix + action + ":" + draft id) from any other
	// component this application might ever add.
	draftCustomIDPrefix = "re_draft:"
)

func modalEditCustomID(draftID string) string {
	return replyModalCustomID + ":" + draftID
}

// ReplyService is the one method this package needs from
// emailinquiries.Service — kept as an interface so this package doesn't
// import the concrete type, and so tests can fake it.
type ReplyService interface {
	ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (emailinquiries.Inquiry, error)
}

type Handler struct {
	publicKey ed25519.PublicKey
	reply     ReplyService
	drafts    DraftStore
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

// ConfigureDraftStore wires the 送出/編輯/刪除 preview flow. Without it (e.g.
// Redis isn't configured), a modal submission falls back to sending
// immediately, same as before this safeguard existed — degraded, not
// broken.
func (h *Handler) ConfigureDraftStore(drafts DraftStore) {
	h.drafts = drafts
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
			Name       string `json:"name"`
			CustomID   string `json:"custom_id"`
			Components []struct {
				Components []struct {
					CustomID string `json:"custom_id"`
					Value    string `json:"value"`
				} `json:"components"`
			} `json:"components"`
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

	staffName := payload.Member.User.GlobalName
	if staffName == "" {
		staffName = payload.Member.User.Username
	}
	if staffName == "" {
		staffName = "工作人員"
	}

	switch {
	case payload.Type == interactionTypeApplicationCommand && payload.Data.Name == "re":
		// A slash command's text options are single-line Discord-side — no
		// amount of escaping lets a staff member press Enter for a new
		// paragraph in one. A modal's paragraph-style text input is the one
		// Discord component that actually accepts real line breaks, so /re
		// opens one instead of taking message/signature as command options
		// directly. A modal must be the interaction's FIRST response (it
		// can't follow a deferral), so this returns immediately with
		// nothing deferred yet — the actual send happens only after the
		// preview's 送出 button, below.
		writeJSON(w, replyModalResponse(replyModalCustomID, "", ""))
		return

	case payload.Type == interactionTypeModalSubmit && payload.Data.CustomID == replyModalCustomID:
		message, signature := extractModalFields(payload.Data.Components)
		h.handleFreshModalSubmit(w, r, payload.ApplicationID, payload.Token, payload.Channel.ID, staffName, payload.Member.User.ID, message, signature)
		return

	case payload.Type == interactionTypeModalSubmit && strings.HasPrefix(payload.Data.CustomID, replyModalCustomID+":"):
		draftID := strings.TrimPrefix(payload.Data.CustomID, replyModalCustomID+":")
		message, signature := extractModalFields(payload.Data.Components)
		h.handleEditModalSubmit(w, r, draftID, message, signature)
		return

	case payload.Type == interactionTypeMessageComponent && strings.HasPrefix(payload.Data.CustomID, draftCustomIDPrefix):
		action, id, ok := parseDraftCustomID(payload.Data.CustomID)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.handleDraftAction(w, r, payload.ApplicationID, payload.Token, action, id)
		return

	default:
		writeJSON(w, interactionErrorResponse("這個指令不適用於這裡"))
	}
}

func extractModalFields(rows []struct {
	Components []struct {
		CustomID string `json:"custom_id"`
		Value    string `json:"value"`
	} `json:"components"`
}) (message, signature string) {
	for _, row := range rows {
		for _, field := range row.Components {
			switch field.CustomID {
			case "message":
				message = strings.TrimSpace(field.Value)
			case "signature":
				signature = strings.TrimSpace(field.Value)
			}
		}
	}
	return message, signature
}

// handleFreshModalSubmit handles the first time a given reply is drafted —
// no message/draft exists yet, so this always creates both. Any earlier,
// not-yet-sent draft for the same thread is superseded (and its now-stale
// preview message deleted, best-effort) rather than left dangling.
func (h *Handler) handleFreshModalSubmit(w http.ResponseWriter, r *http.Request, applicationID, token, threadID, staffName, staffDiscordID, message, signature string) {
	if message == "" || signature == "" {
		writeJSON(w, interactionErrorResponse("回覆內容跟簽名都不能是空白。"))
		return
	}
	draft := replyDraft{
		ThreadID: threadID, StaffName: staffName, StaffDiscordID: staffDiscordID,
		Message: message, Signature: signature,
		ApplicationID: applicationID, Token: token,
	}
	if h.drafts == nil {
		// No draft store configured — skip the preview safeguard entirely
		// rather than silently dropping the reply.
		writeJSON(w, map[string]any{
			"type": responseTypeDeferredChannelMessageWithSource,
			"data": map[string]any{"flags": 64},
		})
		go h.finishReply(applicationID, token, draft, nil)
		return
	}
	id, err := newDraftID()
	if err != nil {
		writeJSON(w, interactionErrorResponse("建立預覽失敗，請再試一次。"))
		return
	}
	superseded, err := h.drafts.SaveDraft(r.Context(), id, draft)
	if err != nil {
		writeJSON(w, interactionErrorResponse("建立預覽失敗，請再試一次。"))
		return
	}
	if superseded != nil {
		// Best-effort: only works inside Discord's ~15 minute
		// interaction-token window (see replyDraft's doc comment) — past
		// that there's no API call that can remove it, so a failure here
		// is silently ignored rather than surfaced.
		go h.deletePreviewMessage(context.Background(), superseded.ApplicationID, superseded.Token)
	}
	writeJSON(w, map[string]any{
		"type": responseTypeChannelMessageWithSource,
		"data": draftPreviewMessageData(id, draft, true),
	})
}

// handleEditModalSubmit handles 編輯's reopened modal. Because that modal
// was launched from a button on the preview message, Discord lets this
// response update that same message (UPDATE_MESSAGE) instead of posting a
// new one — so an edit revises the existing embed in place.
func (h *Handler) handleEditModalSubmit(w http.ResponseWriter, r *http.Request, draftID, message, signature string) {
	if message == "" || signature == "" {
		writeJSON(w, map[string]any{
			"type": responseTypeUpdateMessage,
			"data": map[string]any{"content": "回覆內容跟簽名都不能是空白，請重新打 /re。", "embeds": []any{}, "components": []any{}},
		})
		return
	}
	draft, found, err := h.drafts.LoadDraft(r.Context(), draftID)
	if err != nil || !found {
		writeJSON(w, map[string]any{
			"type": responseTypeUpdateMessage,
			"data": map[string]any{"content": "這份草稿已經處理過或過期了，麻煩重新打 /re。", "embeds": []any{}, "components": []any{}},
		})
		return
	}
	draft.Message = message
	draft.Signature = signature
	if err := h.drafts.UpdateDraft(r.Context(), draftID, draft); err != nil {
		writeJSON(w, map[string]any{
			"type": responseTypeUpdateMessage,
			"data": map[string]any{"content": "更新草稿失敗，請重新打 /re。", "embeds": []any{}, "components": []any{}},
		})
		return
	}
	writeJSON(w, map[string]any{
		"type": responseTypeUpdateMessage,
		"data": draftPreviewMessageData(draftID, draft, true),
	})
}

// handleDraftAction responds to one of the preview's 送出/編輯/刪除 buttons.
// h.drafts is never nil here — a draft's buttons only exist in a message
// draftPreviewMessageData itself produced, which only happens when h.drafts
// is configured.
func (h *Handler) handleDraftAction(w http.ResponseWriter, r *http.Request, applicationID, token, action, id string) {
	draft, found, err := h.drafts.LoadDraft(r.Context(), id)
	if err != nil || !found {
		writeJSON(w, map[string]any{
			"type": responseTypeUpdateMessage,
			"data": map[string]any{"content": "這份草稿已經處理過或過期了，麻煩重新打 /re。", "embeds": []any{}, "components": []any{}},
		})
		return
	}
	switch action {
	case "delete":
		// Unlike 編輯/送出 (which keep revising the same message), 刪除
		// removes it outright — nothing was sent, so there's no reason for
		// it to linger in the channel at all, discarded-looking or not.
		_ = h.drafts.DeleteDraft(r.Context(), id)
		writeJSON(w, map[string]any{"type": responseTypeDeferredUpdateMessage})
		go h.deletePreviewMessage(context.Background(), applicationID, token)
	case "edit":
		// This interaction's own response has to be the modal — Discord
		// has no response type that both opens one and edits the message
		// the button lives on. The draft is deliberately left alive (not
		// deleted): handleEditModalSubmit updates this same draft/message
		// in place once the form comes back, instead of creating a new one.
		writeJSON(w, replyModalResponse(modalEditCustomID(id), draft.Message, draft.Signature))
	case "send":
		_ = h.drafts.DeleteDraft(r.Context(), id)
		writeJSON(w, map[string]any{"type": responseTypeDeferredUpdateMessage})
		go h.finishReply(applicationID, token, draft, &draft)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// finishReply does the actual work (ReplyByDiscord) after the interaction
// has already been ACKed, then updates the deferred response with the real
// result — runs on its own background context since the request that
// spawned it is already finished. previewDraft is non-nil when there's an
// existing preview embed to recolor in place (the normal case, via the 送出
// button); nil falls back to a plain-text result (only when no draft store
// is configured at all, so there was never a preview to begin with).
func (h *Handler) finishReply(applicationID, token string, draft replyDraft, previewDraft *replyDraft) {
	ctx, cancel := context.WithTimeout(context.Background(), discordFollowupBackgroundTimeout)
	defer cancel()
	_, err := h.reply.ReplyByDiscord(ctx, draft.ThreadID, draft.StaffName, draft.StaffDiscordID, draft.Message, draft.Signature)
	var title string
	color := embedColorSent
	switch {
	case err == nil:
		title = "✅ 回覆已寄出"
	case errors.Is(err, emailinquiries.ErrNotFound):
		// /re only makes sense inside a thread this system created — see
		// the package doc comment. Using it anywhere else (a random forum
		// post, a staff-only discussion channel, ...) has no inquiry to
		// resolve to, so there's nothing to reply to.
		title, color = "這個指令只能在信件討論串裡使用。", embedColorFailed
	case errors.Is(err, emailinquiries.ErrInvalidInput):
		title, color = "回覆內容跟簽名都不能是空白。", embedColorFailed
	default:
		title, color = "回覆寄送失敗，請稍後再試一次。", embedColorFailed
	}
	if previewDraft != nil {
		h.editFollowupEmbed(ctx, applicationID, token, draftEmbed(title, color, *previewDraft))
		return
	}
	h.editFollowupMessage(ctx, applicationID, token, title)
}

// editFollowupMessage replaces the deferred placeholder with plain text —
// used only in the no-draft-store fallback, where there was never a preview
// embed to begin with.
func (h *Handler) editFollowupMessage(ctx context.Context, applicationID, token, content string) {
	h.patchOriginal(ctx, applicationID, token, map[string]any{"content": content, "embeds": []any{}, "components": []any{}})
}

// editFollowupEmbed is editFollowupMessage's embed-preserving counterpart:
// the 送出 result replaces the preview's yellow embed with a green/red one
// showing the same content, rather than discarding it for plain text.
func (h *Handler) editFollowupEmbed(ctx context.Context, applicationID, token string, embed map[string]any) {
	h.patchOriginal(ctx, applicationID, token, map[string]any{"content": "", "embeds": []map[string]any{embed}, "components": []any{}})
}

// patchOriginal edits the invoking interaction's own response, via the same
// interaction-token-authenticated webhook endpoint Discord itself uses — no
// bot token required. Works identically whether the interaction that
// deferred was a slash command or a button press (@original always refers
// to the invoking interaction's own response).
func (h *Handler) patchOriginal(ctx context.Context, applicationID, token string, data map[string]any) {
	body, err := json.Marshal(data)
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

// deletePreviewMessage removes an ephemeral preview message outright — used
// when a newer, unrelated /re draft supersedes it (see
// handleFreshModalSubmit), so a never-sent draft doesn't sit visibly in the
// channel. Only reachable within Discord's ~15 minute interaction-token
// window (same constraint as patchOriginal); a failure past that is
// expected, not an error worth surfacing anywhere, since nothing could have
// been done about it anyway.
func (h *Handler) deletePreviewMessage(ctx context.Context, applicationID, token string) {
	if applicationID == "" || token == "" {
		return
	}
	endpoint := "https://discord.com/api/v10/webhooks/" + url.PathEscape(applicationID) + "/" + url.PathEscape(token) + "/messages/@original"
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return
	}
	client := http.Client{Timeout: discordFollowupRequestTimeout}
	response, err := client.Do(request)
	if err != nil {
		return
	}
	_ = response.Body.Close()
}

// replyModalResponse opens the /re reply form: a paragraph-style (multi-line)
// field for the message body and a short single-line field for the required
// signature. Field order here is also the on-screen order. Non-empty
// message/signature pre-fill the form — used when 編輯 reopens it. customID
// is replyModalCustomID for a fresh draft, or modalEditCustomID(draftID)
// when reopened from an existing preview's 編輯 button.
func replyModalResponse(customID, message, signature string) map[string]any {
	return map[string]any{
		"type": responseTypeModal,
		"data": map[string]any{
			"custom_id": customID,
			"title":     "回覆信件",
			"components": []map[string]any{
				{
					"type": 1,
					"components": []map[string]any{
						{
							"type":       4,
							"custom_id":  "message",
							"style":      2,
							"label":      "回覆內容",
							"required":   true,
							"max_length": 3900,
							"value":      message,
						},
					},
				},
				{
					"type": 1,
					"components": []map[string]any{
						{
							"type":       4,
							"custom_id":  "signature",
							"style":      1,
							"label":      "簽名（會附在信件結尾）",
							"required":   true,
							"max_length": 100,
							"value":      signature,
						},
					},
				},
			},
		},
	}
}

// draftEmbed renders one reply draft's content as an embed, colored by
// state (pending/sent/failed/discarded) — the single building block every
// preview/result message is made of, so 編輯 and 送出 only ever change the
// title/color of the same embed shape instead of restructuring the message.
func draftEmbed(title string, color int, draft replyDraft) map[string]any {
	return map[string]any{
		"title":       title,
		"description": truncateForPreview(draft.Message),
		"color":       color,
		"fields": []map[string]any{
			{"name": "簽名", "value": draft.Signature, "inline": false},
		},
	}
}

// draftPreviewMessageData is the pending-review message body (embed + 送出/
// 編輯/刪除 buttons) shared by a fresh draft's first message and an edited
// draft's in-place update. withHint adds the "按下去才會真的寄信" reminder —
// true for both of those; finishReply's own result message never uses this
// function at all, only draftEmbed directly.
func draftPreviewMessageData(id string, draft replyDraft, withHint bool) map[string]any {
	data := map[string]any{
		"flags":  64, // ephemeral: only the invoking staff member sees this
		"embeds": []map[string]any{draftEmbed("回覆預覽", embedColorPreview, draft)},
		"components": []map[string]any{
			{
				"type": 1,
				"components": []map[string]any{
					{"type": 2, "style": buttonStyleSuccess, "label": "送出", "custom_id": draftCustomIDPrefix + "send:" + id},
					{"type": 2, "style": buttonStylePrimary, "label": "編輯", "custom_id": draftCustomIDPrefix + "edit:" + id},
					{"type": 2, "style": buttonStyleDanger, "label": "刪除", "custom_id": draftCustomIDPrefix + "delete:" + id},
				},
			},
		},
	}
	if withHint {
		data["content"] = "請確認下面的回覆內容，沒問題再按「送出」——按下去才會真的寄信。"
	}
	return data
}

// parseDraftCustomID splits "re_draft:<action>:<id>" back apart. id itself
// is a hex string (see newDraftID) so a colon inside it is never ambiguous,
// but SplitN still caps it at 3 parts out of habit.
func parseDraftCustomID(customID string) (action, id string, ok bool) {
	rest := strings.TrimPrefix(customID, draftCustomIDPrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func truncateForPreview(value string) string {
	const limit = 3900 // Discord embed description limit is 4096
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n…（過長，已截斷）"
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
