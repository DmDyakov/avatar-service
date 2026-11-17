package avatar_test

import (
	"context"
	"testing"
	"time"

	"avatar-service/internal/domain"
	avatarrepo "avatar-service/internal/repository/postgres/avatar"
	"avatar-service/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestDB поднимает контейнер Postgres, накатывает миграцию
// и возвращает пул подключений. Возвращает функцию для очистки.
func setupTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("avatars_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second)),
	)
	require.NoError(t, err)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	sqlBytes, err := migrations.FS.ReadFile("000001_create_avatars.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sqlBytes))
	require.NoError(t, err)

	return pool, func() {
		pool.Close()
		_ = pgContainer.Terminate(ctx)
	}
}

func createTestAvatar(t *testing.T, repo *avatarrepo.AvatarRepository, id string) {
	t.Helper()

	avatar := &domain.Avatar{
		ID:               id,
		UserID:           "test-user",
		FileName:         "avatar.jpg",
		MimeType:         "image/jpeg",
		SizeBytes:        100,
		S3Key:            "avatars/test-user/" + id,
		UploadStatus:     domain.UploadStatusUploaded,
		ProcessingStatus: domain.ProcessingStatusPending,
		Attempts:         0,
	}
	require.NoError(t, repo.Create(context.Background(), avatar))
}

func TestAvatarRepository_UpdateProcessingStatus(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := avatarrepo.NewAvatarRepository(pool)
	ctx := context.Background()
	id := "11111111-1111-1111-1111-111111111111"

	createTestAvatar(t, repo, id)

	time.Sleep(10 * time.Millisecond)

	err := repo.UpdateProcessingStatus(ctx, id, domain.ProcessingStatusProcessing)
	require.NoError(t, err)

	updated, err := repo.GetByID(ctx, id)
	require.NoError(t, err)

	assert.Equal(t, domain.ProcessingStatusProcessing, updated.ProcessingStatus)
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt),
		"updated_at должен быть больше created_at (сработал триггер)")
}
