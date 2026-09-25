package notifications

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/email"
)

type fakeEmailStore struct {
	tasks  []EmailTask
	sent   int
	failed int
}

func (s *fakeEmailStore) ClaimEmailOutbox(context.Context, int) ([]EmailTask, error) {
	return s.tasks, nil
}

func (s *fakeEmailStore) MarkEmailSent(context.Context, uuid.UUID) error {
	s.sent++
	return nil
}

func (s *fakeEmailStore) MarkEmailFailed(context.Context, uuid.UUID, string) error {
	s.failed++
	return nil
}

type fakeEmailSender struct {
	messages []email.Message
	err      error
}

func (s *fakeEmailSender) Send(_ context.Context, message email.Message) error {
	s.messages = append(s.messages, message)
	return s.err
}

func TestEmailWorkerDecryptsOnlyAtDelivery(t *testing.T) {
	cipher, err := auth.NewFieldCipher(bytesKey(32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	recipient, err := cipher.Seal("user@example.test")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := cipher.Seal(`{"subject":"Test","text":"Body"}`)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeEmailStore{tasks: []EmailTask{{ID: uuid.New(), RecipientCiphertext: recipient, PayloadCiphertext: payload}}}
	sender := &fakeEmailSender{}
	worker := &EmailWorker{Store: store, Cipher: cipher, Sender: sender}
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if len(sender.messages) != 1 || sender.messages[0].To != "user@example.test" || sender.messages[0].Subject != "Test" {
		t.Fatalf("messages = %#v", sender.messages)
	}
	if store.sent != 1 || store.failed != 0 {
		t.Fatalf("sent=%d failed=%d", store.sent, store.failed)
	}
}

func TestEmailWorkerMarksDeliveryFailure(t *testing.T) {
	cipher, _ := auth.NewFieldCipher(bytesKey(32))
	recipient, _ := cipher.Seal("user@example.test")
	payload, _ := cipher.Seal(`{"subject":"Test","text":"Body"}`)
	store := &fakeEmailStore{tasks: []EmailTask{{ID: uuid.New(), RecipientCiphertext: recipient, PayloadCiphertext: payload}}}
	worker := &EmailWorker{Store: store, Cipher: cipher, Sender: &fakeEmailSender{err: errors.New("SMTP unavailable")}}
	if err := worker.ProcessOnce(context.Background()); err == nil {
		t.Fatal("ProcessOnce() error = nil, want SMTP error")
	}
	if store.sent != 0 || store.failed != 1 {
		t.Fatalf("sent=%d failed=%d", store.sent, store.failed)
	}
}

func bytesKey(length int) []byte {
	key := make([]byte, length)
	for index := range key {
		key[index] = byte(index + 1)
	}
	return key
}
