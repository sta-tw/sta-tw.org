package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"sta-backend/internal/auth"
	"sta-backend/internal/email"
)

type EmailWorker struct {
	Store        EmailOutboxStore
	InquiryStore InquiryStore
	Notifier     Repository
	Cipher       *auth.FieldCipher
	Sender       email.Sender
	BatchSize    int
	PollInterval time.Duration
	Logger       *slog.Logger
}

func (w *EmailWorker) ProcessOnce(ctx context.Context) error {
	if w == nil || w.Store == nil || w.Cipher == nil || w.Sender == nil {
		return errors.New("email worker is not configured")
	}
	firstErr := w.processInquiryNotifications(ctx)
	tasks, err := w.Store.ClaimEmailOutbox(ctx, w.batchSize())
	if err != nil {
		if firstErr != nil {
			return firstErr
		}
		return err
	}
	for _, task := range tasks {
		recipient, decryptErr := w.Cipher.Open(task.RecipientCiphertext)
		var message email.Message
		if decryptErr == nil {
			var payload EmailPayload
			var payloadText string
			payloadText, decryptErr = w.Cipher.Open(task.PayloadCiphertext)
			if decryptErr == nil {
				decryptErr = json.Unmarshal([]byte(payloadText), &payload)
			}
			message = email.Message{To: recipient, Subject: payload.Subject, Text: payload.Text}
		}
		if decryptErr != nil {
			if markErr := w.Store.MarkEmailFailed(ctx, task.ID, safeEmailError(decryptErr)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = decryptErr
			}
			continue
		}
		if err := w.Sender.Send(ctx, message); err != nil {
			if markErr := w.Store.MarkEmailFailed(ctx, task.ID, safeEmailError(err)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := w.Store.MarkEmailSent(ctx, task.ID); err != nil {
			if markErr := w.Store.MarkEmailFailed(ctx, task.ID, safeEmailError(err)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (w *EmailWorker) processInquiryNotifications(ctx context.Context) error {
	if w.InquiryStore == nil || w.Notifier == nil {
		return nil
	}
	tasks, err := w.InquiryStore.ClaimInquiryNotifications(ctx, w.batchSize())
	if err != nil {
		return err
	}
	var firstErr error
	for _, task := range tasks {
		title := "查榜意願回覆提醒"
		body := "你的查榜結果已有可供參考的順位資料，請登入 STA 平台填寫或確認意願。可選值為 0、20、40、60、80、100。"
		if task.InquiryRound == "acceptance_deadline" {
			title = "正取截止前意願確認"
			body = "正取截止前請登入 STA 平台確認查榜意願；未回覆不會被視為放棄。"
			if task.ResponseDeadline != nil {
				body += "本次提醒截止時間：" + task.ResponseDeadline.UTC().Format(time.RFC3339) + "。"
			}
		}
		key := "willingness:" + task.ID.String()
		var notifyErr error
		if _, notifyErr = w.Notifier.CreateInApp(ctx, task.AccountID, "willingness", key, title, body); notifyErr == nil {
			notifyErr = w.Notifier.EnqueueEmailForAccount(ctx, task.AccountID, key, title, body, "willingness")
		}
		if notifyErr != nil {
			if markErr := w.InquiryStore.MarkInquiryNotificationFailed(ctx, task.ID, safeEmailError(notifyErr)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = notifyErr
			}
			continue
		}
		if err := w.InquiryStore.MarkInquiryNotificationEnqueued(ctx, task.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (w *EmailWorker) Run(ctx context.Context) error {
	if w == nil || w.Store == nil {
		return errors.New("email worker is not configured")
	}
	interval := w.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.ProcessOnce(ctx); err != nil && w.Logger != nil {
			w.Logger.Warn("email delivery pass failed", "error", safeEmailError(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *EmailWorker) batchSize() int {
	if w.BatchSize < 1 || w.BatchSize > 100 {
		return 20
	}
	return w.BatchSize
}

func safeEmailError(err error) string {
	if err == nil {
		return "unknown email delivery error"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
