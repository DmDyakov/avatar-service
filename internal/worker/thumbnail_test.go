package worker_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"avatar-service/internal/broker/rabbitmq"
	"avatar-service/internal/config"
	"avatar-service/internal/domain"
	avatarrepo "avatar-service/internal/repository/postgres/avatar"
	"avatar-service/internal/storage/minio"
	"avatar-service/internal/worker"
	"avatar-service/migrations"

	"github.com/disintegration/imaging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

const testBucket = "avatars"

var (
	testPool      *pgxpool.Pool
	testRepo      *avatarrepo.AvatarRepository
	testStorage   *minio.Storage
	testRMQCfg    config.RabbitMQConfig
	testWorkerCfg config.WorkerConfig
	testCleanups  []func()
)

// TestMain поднимает общий стек (PG, MinIO, RabbitMQ) один раз.
// -short пропускает все интеграционные тесты (CI без Docker).
func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(0)
	}

	ctx := context.Background()

	if err := setup(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "setup: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	for i := len(testCleanups) - 1; i >= 0; i-- {
		testCleanups[i]()
	}

	os.Exit(code)
}

func setup(ctx context.Context) error {
	// --- Postgres ---
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("avatars_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(20*time.Second)),
	)
	if err != nil {
		return fmt.Errorf("start postgres: %w", err)
	}
	testCleanups = append(testCleanups, func() { _ = pgContainer.Terminate(ctx) })

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("pg connstr: %w", err)
	}

	testPool, err = pgxpool.New(ctx, pgConnStr)
	if err != nil {
		return fmt.Errorf("pg pool: %w", err)
	}
	testCleanups = append(testCleanups, testPool.Close)

	sqlBytes, err := migrations.FS.ReadFile("000001_create_avatars.up.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if _, err := testPool.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}

	testRepo = avatarrepo.NewAvatarRepository(testPool)

	// --- MinIO ---
	minioContainer, err := tcminio.Run(ctx,
		"pgsty/minio:latest",
		tcminio.WithUsername("minioadmin"),
		tcminio.WithPassword("minioadmin"),
	)
	if err != nil {
		return fmt.Errorf("start minio: %w", err)
	}
	testCleanups = append(testCleanups, func() { _ = minioContainer.Terminate(ctx) })

	minioEndpoint, err := minioContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("minio connstr: %w", err)
	}
	minioEndpoint = strings.TrimPrefix(minioEndpoint, "http://")

	testStorage, err = minio.NewStorage(config.S3Config{
		Endpoint:       minioEndpoint,
		AccessKey:      "minioadmin",
		SecretKey:      "minioadmin",
		Bucket:         testBucket,
		UseSSL:         false,
		Region:         "us-east-1",
		ConnectTimeout: 30 * time.Second,
		ForcePathStyle: true,
	})
	if err != nil {
		return fmt.Errorf("minio storage: %w", err)
	}

	// --- RabbitMQ ---
	rmqContainer, err := tcrabbit.Run(ctx,
		"rabbitmq:3.13-alpine",
		tcrabbit.WithAdminUsername("guest"),
		tcrabbit.WithAdminPassword("guest"),
	)
	if err != nil {
		return fmt.Errorf("start rabbitmq: %w", err)
	}
	testCleanups = append(testCleanups, func() { _ = rmqContainer.Terminate(ctx) })

	rmqHost, err := rmqContainer.Host(ctx)
	if err != nil {
		return fmt.Errorf("rmq host: %w", err)
	}
	rmqPort, err := rmqContainer.MappedPort(ctx, "5672/tcp")
	if err != nil {
		return fmt.Errorf("rmq port: %w", err)
	}
	testRMQCfg = config.RabbitMQConfig{
		URL:            fmt.Sprintf("amqp://guest:guest@%s:%s/", rmqHost, rmqPort.Port()),
		Heartbeat:      10 * time.Second,
		PrefetchCount:  10,
		ConnectTimeout: 30 * time.Second,
	}

	// Быстрый backoff в тестах: 10ms × 2 = 10ms → 20ms. Не 2s → 4s.
	testWorkerCfg = config.WorkerConfig{
		RetryMaxAttempts:    3,
		RetryInitialBackoff: 10 * time.Millisecond,
		RetryMultiplier:     2.0,
	}

	return nil
}

// resetState чистит БД перед каждым тестом.
func resetState(t *testing.T) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), "TRUNCATE avatars CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// makeTestJPEG генерирует валидный JPEG нужного размера.
func makeTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := imaging.New(width, height, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buf bytes.Buffer
	require.NoError(t, imaging.Encode(&buf, img, imaging.JPEG))
	return buf.Bytes()
}

// createAvatar создаёт запись в БД и загружает оригинал в S3.
func createAvatar(t *testing.T, id string) *domain.Avatar {
	t.Helper()
	ctx := context.Background()

	avatar := &domain.Avatar{
		ID:               id,
		UserID:           "test-user",
		FileName:         "avatar.jpg",
		MimeType:         "image/jpeg",
		SizeBytes:        200,
		S3Key:            "avatars/test-user/" + id,
		UploadStatus:     domain.UploadStatusUploaded,
		ProcessingStatus: domain.ProcessingStatusPending,
		Attempts:         0,
	}
	require.NoError(t, testRepo.Create(ctx, avatar))

	jpeg := makeTestJPEG(t, 400, 400)
	require.NoError(t, testStorage.Upload(ctx, avatar.S3Key, bytes.NewReader(jpeg), int64(len(jpeg)), "image/jpeg"))

	return avatar
}

// waitForStatus поллит БД до появления нужного статуса или таймаута.
func waitForStatus(t *testing.T, id string, want domain.ProcessingStatus, timeout time.Duration) *domain.Avatar {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		avatar, err := testRepo.GetByID(context.Background(), id)
		if err == nil && avatar.ProcessingStatus == want {
			return avatar
		}
		time.Sleep(100 * time.Millisecond)
	}
	last, _ := testRepo.GetByID(context.Background(), id)
	t.Fatalf("timeout waiting for status %q, last: %+v", want, last)
	return nil
}

// ---------- E2E через RabbitMQ ----------

func TestWorkerE2E(t *testing.T) {
	resetState(t)

	avatar := createAvatar(t, "11111111-1111-1111-1111-111111111111")
	logger := zap.NewNop()

	publisher, err := rabbitmq.NewPublisher(testRMQCfg, logger)
	require.NoError(t, err)
	defer func() { _ = publisher.Close() }()

	consumer, err := rabbitmq.NewConsumer(testRMQCfg, logger)
	require.NoError(t, err)
	defer func() { _ = consumer.Close() }()

	handler := worker.NewThumbnailHandler(testRepo, testStorage, logger, testWorkerCfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerDone := make(chan error, 1)
	go func() { consumerDone <- consumer.Run(ctx, handler.Handle) }()

	// Даём consumer'у подписаться.
	time.Sleep(500 * time.Millisecond)

	event := domain.AvatarUploadEvent{
		AvatarID: avatar.ID,
		UserID:   avatar.UserID,
		S3Key:    avatar.S3Key,
	}
	require.NoError(t, publisher.PublishUploadEvent(ctx, event))

	updated := waitForStatus(t, avatar.ID, domain.ProcessingStatusCompleted, 10*time.Second)

	require.Len(t, updated.ThumbnailS3Keys, 2)
	assert.Contains(t, updated.ThumbnailS3Keys, "100x100")
	assert.Contains(t, updated.ThumbnailS3Keys, "300x300")

	for size, key := range updated.ThumbnailS3Keys {
		reader, err := testStorage.Download(ctx, key)
		require.NoError(t, err, "download %s", size)
		img, err := imaging.Decode(reader, imaging.AutoOrientation(false))
		require.NoError(t, err, "decode %s", size)
		_ = reader.Close()

		switch size {
		case "100x100":
			assert.Equal(t, 100, img.Bounds().Dx())
			assert.Equal(t, 100, img.Bounds().Dy())
		case "300x300":
			assert.Equal(t, 300, img.Bounds().Dx())
			assert.Equal(t, 300, img.Bounds().Dy())
		}
	}

	cancel()
	select {
	case <-consumerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop in time")
	}
}

// ---------- Прямой вызов Process ----------

func TestThumbnailHandler_HappyPath(t *testing.T) {
	resetState(t)

	avatar := createAvatar(t, "22222222-2222-2222-2222-222222222222")
	handler := worker.NewThumbnailHandler(testRepo, testStorage, zap.NewNop(), testWorkerCfg)

	event := domain.AvatarUploadEvent{AvatarID: avatar.ID, UserID: avatar.UserID, S3Key: avatar.S3Key}
	require.NoError(t, handler.Process(context.Background(), event))

	updated, err := testRepo.GetByID(context.Background(), avatar.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ProcessingStatusCompleted, updated.ProcessingStatus)
	assert.Equal(t, 0, updated.Attempts)
	require.Len(t, updated.ThumbnailS3Keys, 2)
}

func TestThumbnailHandler_Idempotency(t *testing.T) {
	resetState(t)

	avatar := createAvatar(t, "33333333-3333-3333-3333-333333333333")
	handler := worker.NewThumbnailHandler(testRepo, testStorage, zap.NewNop(), testWorkerCfg)
	event := domain.AvatarUploadEvent{AvatarID: avatar.ID, UserID: avatar.UserID, S3Key: avatar.S3Key}

	require.NoError(t, handler.Process(context.Background(), event))

	first, err := testRepo.GetByID(context.Background(), avatar.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ProcessingStatusCompleted, first.ProcessingStatus)
	firstUpdatedAt := first.UpdatedAt

	time.Sleep(50 * time.Millisecond)

	require.NoError(t, handler.Process(context.Background(), event))

	second, err := testRepo.GetByID(context.Background(), avatar.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ProcessingStatusCompleted, second.ProcessingStatus)
	assert.Equal(t, firstUpdatedAt, second.UpdatedAt, "updated_at не должен меняться при skip")
}

func TestThumbnailHandler_ErrorOnDownload(t *testing.T) {
	resetState(t)

	// Запись в БД есть, файла в S3 нет.
	avatar := &domain.Avatar{
		ID:               "44444444-4444-4444-4444-444444444444",
		UserID:           "test-user",
		FileName:         "avatar.jpg",
		MimeType:         "image/jpeg",
		SizeBytes:        100,
		S3Key:            "avatars/test-user/missing-file",
		UploadStatus:     domain.UploadStatusUploaded,
		ProcessingStatus: domain.ProcessingStatusPending,
		Attempts:         0,
	}
	require.NoError(t, testRepo.Create(context.Background(), avatar))

	handler := worker.NewThumbnailHandler(testRepo, testStorage, zap.NewNop(), testWorkerCfg)
	event := domain.AvatarUploadEvent{AvatarID: avatar.ID, UserID: avatar.UserID, S3Key: avatar.S3Key}

	err := handler.Process(context.Background(), event)
	require.Error(t, err)

	updated, getErr := testRepo.GetByID(context.Background(), avatar.ID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.ProcessingStatusFailed, updated.ProcessingStatus)
	assert.Equal(t, 1, updated.Attempts, "attempts=1 после исчерпания retry, не 3")
}

// ---------- Retry ----------

// flakyStorage падает на Download заданное число раз, затем успешен.
type flakyStorage struct {
	mu           sync.Mutex
	failuresLeft int
	calls        int
	jpeg         []byte
}

func (s *flakyStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.failuresLeft > 0 {
		s.failuresLeft--
		return nil, fmt.Errorf("simulated storage failure")
	}
	return io.NopCloser(bytes.NewReader(s.jpeg)), nil
}

func (s *flakyStorage) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	return nil
}

func TestThumbnailHandler_Retry_EventuallySucceeds(t *testing.T) {
	resetState(t)

	avatar := createAvatar(t, "55555555-5555-5555-5555-555555555555")
	fs := &flakyStorage{failuresLeft: 2, jpeg: makeTestJPEG(t, 100, 100)}
	handler := worker.NewThumbnailHandler(testRepo, fs, zap.NewNop(), testWorkerCfg)

	event := domain.AvatarUploadEvent{AvatarID: avatar.ID, UserID: avatar.UserID, S3Key: avatar.S3Key}

	require.NoError(t, handler.Process(context.Background(), event))

	assert.Equal(t, 3, fs.calls, "2 фейла + 1 успех = 3 вызова Download")

	updated, err := testRepo.GetByID(context.Background(), avatar.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ProcessingStatusCompleted, updated.ProcessingStatus)
	assert.Equal(t, 0, updated.Attempts, "attempts не увеличивается при успехе")
}

func TestThumbnailHandler_Retry_AllFail(t *testing.T) {
	resetState(t)

	avatar := createAvatar(t, "66666666-6666-6666-6666-666666666666")
	fs := &flakyStorage{failuresLeft: 3, jpeg: makeTestJPEG(t, 100, 100)}
	handler := worker.NewThumbnailHandler(testRepo, fs, zap.NewNop(), testWorkerCfg)

	event := domain.AvatarUploadEvent{AvatarID: avatar.ID, UserID: avatar.UserID, S3Key: avatar.S3Key}

	err := handler.Process(context.Background(), event)
	require.Error(t, err)

	assert.Equal(t, 3, fs.calls, "3 попытки — RetryMaxAttempts")

	updated, getErr := testRepo.GetByID(context.Background(), avatar.ID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.ProcessingStatusFailed, updated.ProcessingStatus)
	assert.Equal(t, 1, updated.Attempts, "attempts=1 после всех retry, не 3")
}
