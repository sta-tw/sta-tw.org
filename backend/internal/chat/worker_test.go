package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeOutboxStore struct {
	tasks  []OutboxTask
	sent   []string
	failed []string
}

func (s *fakeOutboxStore) ClaimOutbox(context.Context, int) ([]OutboxTask, error) {
	return s.tasks, nil
}

func (s *fakeOutboxStore) MarkOutboxSent(_ context.Context, task OutboxTask, externalID string) error {
	s.sent = append(s.sent, task.ID.String()+":"+externalID)
	return nil
}

func (s *fakeOutboxStore) MarkOutboxFailed(_ context.Context, task OutboxTask, reason string) error {
	s.failed = append(s.failed, task.ID.String()+":"+reason)
	return nil
}

type fakeSender struct {
	calls int
	err   error
}

func (s *fakeSender) Send(context.Context, OutboxTask) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	return "external-1", nil
}

func TestSyncWorkerProcessOnceMarksSent(t *testing.T) {
	task := OutboxTask{ID: uuid.New(), TargetPlatform: PlatformDiscord, Operation: OperationCreate, Body: "hello"}
	store := &fakeOutboxStore{tasks: []OutboxTask{task}}
	sender := &fakeSender{}
	worker := &SyncWorker{Store: store, Senders: map[Platform]PlatformSender{PlatformDiscord: sender}}

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
	if len(store.sent) != 1 || len(store.failed) != 0 {
		t.Fatalf("sent = %#v failed = %#v", store.sent, store.failed)
	}
}

func TestSyncWorkerProcessOnceMarksFailed(t *testing.T) {
	task := OutboxTask{ID: uuid.New(), TargetPlatform: PlatformTelegram, Operation: OperationCreate, Body: "hello"}
	store := &fakeOutboxStore{tasks: []OutboxTask{task}}
	worker := &SyncWorker{
		Store:   store,
		Senders: map[Platform]PlatformSender{PlatformTelegram: &fakeSender{err: errors.New("temporary failure")}},
	}

	if err := worker.ProcessOnce(context.Background()); err == nil {
		t.Fatal("ProcessOnce() error = nil, want sender error")
	}
	if len(store.sent) != 0 || len(store.failed) != 1 {
		t.Fatalf("sent = %#v failed = %#v", store.sent, store.failed)
	}
}
