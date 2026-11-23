package outbox_test

import (
	"context"
	"testing"
	"time"

	"avatar-service/internal/repository/postgres/outbox"
	"avatar-service/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()

	c, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("avatars_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(20*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	conn, err := c.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, conn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	for _, f := range []string{
		"000001_create_avatars.up.sql",
		"000002_create_outbox.up.sql",
	} {
		b, err := migrations.FS.ReadFile(f)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(b))
		require.NoError(t, err)
	}
	return pool
}

func TestOutbox_WriteClaimPublish(t *testing.T) {
	pool := setupPool(t)
	repo := outbox.NewRepository(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	err = repo.Write(ctx, tx, outbox.EventTypeUpload, map[string]string{
		"avatar_id": "a1", "user_id": "u1", "s3_key": "k1",
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	events, err := repo.ClaimBatch(ctx, "worker-1", 10, 3)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, outbox.StatusInProgress, events[0].Status)
	assert.Equal(t, 1, events[0].Attempts)

	events2, err := repo.ClaimBatch(ctx, "worker-2", 10, 3)
	require.NoError(t, err)
	assert.Empty(t, events2)

	require.NoError(t, repo.MarkPublished(ctx, events[0].ID))

	events3, err := repo.ClaimBatch(ctx, "worker-1", 10, 3)
	require.NoError(t, err)
	assert.Empty(t, events3)
}

func TestOutbox_MaxAttempts(t *testing.T) {
	pool := setupPool(t)
	repo := outbox.NewRepository(pool)
	ctx := context.Background()

	tx, _ := pool.Begin(ctx)
	require.NoError(t, repo.Write(ctx, tx, outbox.EventTypeUpload, map[string]string{"x": "y"}))
	require.NoError(t, tx.Commit(ctx))

	for i := 1; i <= 3; i++ {
		events, err := repo.ClaimBatch(ctx, "w", 10, 3)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, i, events[0].Attempts)
		require.NoError(t, repo.MarkFailed(ctx, events[0].ID, "boom"))
	}

	events, err := repo.ClaimBatch(ctx, "w", 10, 3)
	require.NoError(t, err)
	assert.Empty(t, events, "permanent failed не должен выбираться")
}

func TestOutbox_ReleaseStale(t *testing.T) {
	pool := setupPool(t)
	repo := outbox.NewRepository(pool)
	ctx := context.Background()

	tx, _ := pool.Begin(ctx)
	require.NoError(t, repo.Write(ctx, tx, outbox.EventTypeUpload, map[string]string{}))
	require.NoError(t, tx.Commit(ctx))

	_, err := repo.ClaimBatch(ctx, "w", 10, 3)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE outbox_events SET locked_at = NOW() - INTERVAL '10 minutes'`)
	require.NoError(t, err)

	n, err := repo.ReleaseStale(ctx, 5*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}
