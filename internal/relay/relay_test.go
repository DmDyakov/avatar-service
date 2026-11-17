package relay_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"avatar-service/internal/domain"
	"avatar-service/internal/relay"
	"avatar-service/internal/repository/postgres/outbox"
	"avatar-service/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

type countingPublisher struct {
	uploads atomic.Int32
	deletes atomic.Int32
	err     error
}

func (c *countingPublisher) PublishUploadEvent(_ context.Context, _ domain.AvatarUploadEvent) error {
	if c.err != nil {
		return c.err
	}
	c.uploads.Add(1)
	return nil
}
func (c *countingPublisher) PublishDeleteEvent(_ context.Context, _ domain.AvatarDeleteEvent) error {
	if c.err != nil {
		return c.err
	}
	c.deletes.Add(1)
	return nil
}

func setupRelayEnv(t *testing.T) (*pgxpool.Pool, outbox.Repository) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("avatars_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(20*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	for _, f := range []string{
		"000001_create_avatars.up.sql",
		"000002_create_outbox.up.sql",
	} {
		b, _ := migrations.FS.ReadFile(f)
		_, err := pool.Exec(ctx, string(b))
		require.NoError(t, err)
	}

	return pool, *outbox.NewRepository(pool)
}

func TestRelay_PollingDelivers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, repo := setupRelayEnv(t)
	pub := &countingPublisher{}
	r := relay.New(relay.Config{
		WorkerID:         "test-relay",
		BatchSize:        10,
		MaxAttempts:      3,
		PollInterval:     100 * time.Millisecond,
		StaleLockTimeout: time.Minute,
		StaleSweepPeriod: time.Minute,
	}, &repo, pub, zap.NewNop())

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	errCh := make(chan error, 1)
	go func() { errCh <- r.Run(runCtx) }()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.Write(ctx, tx, outbox.EventTypeUpload, domain.AvatarUploadEvent{
		AvatarID: "a1", UserID: "u1", S3Key: "k1",
	}))
	require.NoError(t, tx.Commit(ctx))

	require.Eventually(t, func() bool { return pub.uploads.Load() == 1 },
		2*time.Second, 50*time.Millisecond)

	var status string
	require.Eventually(t, func() bool {
		_ = pool.QueryRow(ctx, `SELECT status FROM outbox_events LIMIT 1`).Scan(&status)
		return status == "published"
	}, 2*time.Second, 50*time.Millisecond)

	runCancel()
	<-errCh
}

func TestRelay_MaxAttemptsReached(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, repo := setupRelayEnv(t)
	pub := &countingPublisher{err: assert.AnError}
	r := relay.New(relay.Config{
		WorkerID:         "test-relay",
		BatchSize:        10,
		MaxAttempts:      3,
		PollInterval:     50 * time.Millisecond,
		StaleLockTimeout: time.Minute,
		StaleSweepPeriod: time.Minute,
	}, &repo, pub, zap.NewNop())

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	go func() { _ = r.Run(runCtx) }()

	tx, _ := pool.Begin(ctx)
	require.NoError(t, repo.Write(ctx, tx, outbox.EventTypeUpload, domain.AvatarUploadEvent{
		AvatarID: "a2", UserID: "u1", S3Key: "k2",
	}))
	require.NoError(t, tx.Commit(ctx))

	var attempts int
	var status string
	require.Eventually(t, func() bool {
		_ = pool.QueryRow(ctx,
			`SELECT attempts, status FROM outbox_events WHERE payload->>'avatar_id' = 'a2' LIMIT 1`).
			Scan(&attempts, &status)
		return status == "failed" && attempts >= 3
	}, 5*time.Second, 50*time.Millisecond)
}
