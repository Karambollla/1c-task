package application

import (
	"context"
	"testing"

	"edition/internal/domain"
)

type fakeRepository struct {
	clientID string
	noteID   string
}

func (f *fakeRepository) Authenticate(context.Context, string, string) (string, error) {
	return domain.RoleReporter, nil
}

func (f *fakeRepository) Submit(_ context.Context, clientID, noteID, _, _, _ string) (domain.Receipt, error) {
	f.clientID, f.noteID = clientID, noteID
	return domain.Receipt{Status: "accepted", NoteID: noteID, StoryID: 7}, nil
}

func (f *fakeRepository) Pending(context.Context, string) ([]domain.Notification, error) {
	return nil, nil
}

func (f *fakeRepository) Ack(context.Context, string, int64) error {
	return nil
}

func (f *fakeRepository) Stats(context.Context, string, bool) ([]domain.ClientStats, error) {
	return nil, nil
}

func TestSubmitDelegatesToRepositoryPort(t *testing.T) {
	repo := &fakeRepository{}
	service := New(repo)
	receipt, err := service.Submit(context.Background(), "source", "note-1", "city", "topic", "2026-10-18T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "accepted" || repo.clientID != "source" || repo.noteID != "note-1" {
		t.Fatalf("receipt=%#v, repository=%#v", receipt, repo)
	}
}
