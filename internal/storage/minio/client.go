package minio

import (
	"fmt"

	"avatar-service/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// newClient создаёт клиент MinIO.
//
// BucketLookup задаётся явно, а не через Auto:
//   - Path — MinIO, часть Ceph (http://minio:9000/avatars/{key});
//   - DNS  — AWS S3, RU-провайдеры (http://avatars.s3.amazonaws.com/{key}).
//
// Auto в compose падает: virtual-hosted резолвит DNS avatars.minio,
// которого в bridge-сети нет.
func newClient(cfg config.S3Config) (*minio.Client, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}

	if cfg.ForcePathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	} else {
		opts.BucketLookup = minio.BucketLookupDNS
	}

	client, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return client, nil
}
