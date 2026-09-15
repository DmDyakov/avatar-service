package repository

import (
	"context"

	"avatar-service/internal/domain"
)

// AvatarRepository — реализация репозитория аватарок.
type AvatarRepository struct{}

// NewAvatarRepository создаёт репозиторий аватарок.
func NewAvatarRepository() *AvatarRepository {
	return &AvatarRepository{}
}

// Create сохраняет новую аватарку.
func (r *AvatarRepository) Create(ctx context.Context, avatar *domain.Avatar) error {
	// TODO: реализовать
	return nil
}

// GetByID получает аватарку по ID.
func (r *AvatarRepository) GetByID(ctx context.Context, id string) (*domain.Avatar, error) {
	// TODO: реализовать
	return nil, domain.ErrNotFound
}

// GetUserAvatar получает последнюю аватарку пользователя.
func (r *AvatarRepository) GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, error) {
	// TODO: реализовать
	return nil, domain.ErrNotFound
}

// ListByUserID получает все аватарки пользователя.
func (r *AvatarRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error) {
	// TODO: реализовать
	return []*domain.Avatar{}, nil
}

// Update обновляет аватарку.
func (r *AvatarRepository) Update(ctx context.Context, avatar *domain.Avatar) error {
	// TODO: реализовать
	return domain.ErrNotFound
}

// SoftDelete мягко удаляет аватарку.
func (r *AvatarRepository) SoftDelete(ctx context.Context, id string) error {
	// TODO: реализовать
	return domain.ErrNotFound
}
