package publisher

import (
	"context"

	"avatar-service/internal/domain"
)

// Publisher — реализация публикации событий (заглушка).
type Publisher struct{}

// NewPublisher создаёт publisher.
func NewPublisher() *Publisher {
	return &Publisher{}
}

// PublishAvatarUploaded публикует событие о загрузке.
func (p *Publisher) PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadedEvent) error {
	// TODO: реализовать
	return nil
}

// PublishAvatarDeleted публикует событие об удалении.
func (p *Publisher) PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeletedEvent) error {
	// TODO: реализовать
	return nil
}
