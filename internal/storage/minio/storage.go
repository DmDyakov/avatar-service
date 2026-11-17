package minio

import (
	"avatar-service/internal/config"
	"avatar-service/internal/domain"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
)

// Storage реализует работу с файловым хранилищем MinIO.
type Storage struct {
	client *minio.Client
	bucket string
}

// NewStorage создаёт storage и гарантирует, что бакет существует.
func NewStorage(cfg config.S3Config) (*Storage, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	s := &Storage{
		client: client,
		bucket: cfg.Bucket,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	if err := s.EnsureBucket(ctx); err != nil {
		return nil, fmt.Errorf("ensure bucket: %w", err)
	}

	return s, nil
}

// EnsureBucket создаёт бакет, если его нет.
func (s *Storage) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if exists {
		return nil
	}

	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}

	return nil
}

// Ping проверяет доступность S3.
func (s *Storage) Ping(ctx context.Context) error {
	_, err := s.client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("s3 ping: %w", err)
	}
	return nil
}

// Upload загружает файл.
func (s *Storage) Upload(
	ctx context.Context,
	key string,
	reader io.Reader,
	size int64,
	contentType string,
) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=86400",
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

// Download скачивает файл.
func (s *Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}

	if _, err := obj.Stat(); err != nil {
		obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("stat object: %w", err)
	}

	return obj, nil
}

// Delete удаляет файл.
func (s *Storage) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}
