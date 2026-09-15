package storage

import (
	"context"
	"io"
)

// Storage — реализация файлового хранилища (заглушка).
type Storage struct{}

// NewStorage создаёт storage.
func NewStorage() *Storage {
	return &Storage{}
}

// Upload загружает файл.
func (s *Storage) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	// TODO: реализовать
	return nil
}

// Download скачивает файл.
func (s *Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// TODO: реализовать
	return nil, nil
}

// Delete удаляет файл.
func (s *Storage) Delete(ctx context.Context, key string) error {
	// TODO: реализовать
	return nil
}
