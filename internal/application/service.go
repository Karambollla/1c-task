package application

import (
	"context"

	"edition/internal/domain"
)

type Repository interface {
	Authenticate(context.Context, string, string) (string, error)
	Submit(context.Context, string, string, string, string, string) (domain.Receipt, error)
	Pending(context.Context, string) ([]domain.Notification, error)
	Ack(context.Context, string, int64) error
	Stats(context.Context, string, bool) ([]domain.ClientStats, error)
}

type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Authenticate(ctx context.Context, id, secret string) (string, error) {
	return s.repo.Authenticate(ctx, id, secret)
}

func (s *Service) Submit(ctx context.Context, clientID, noteID, section, topic, occurredAt string) (domain.Receipt, error) {
	return s.repo.Submit(ctx, clientID, noteID, section, topic, occurredAt)
}

func (s *Service) Pending(ctx context.Context, clientID string) ([]domain.Notification, error) {
	return s.repo.Pending(ctx, clientID)
}

func (s *Service) Ack(ctx context.Context, clientID string, messageID int64) error {
	return s.repo.Ack(ctx, clientID, messageID)
}

func (s *Service) Stats(ctx context.Context, clientID string, all bool) ([]domain.ClientStats, error) {
	return s.repo.Stats(ctx, clientID, all)
}
