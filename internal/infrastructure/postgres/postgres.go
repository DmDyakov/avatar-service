// Package postgres реализует подключение к PostgreSQL.
package postgres

import (
	"avatar-service/internal/config"
	"avatar-service/migrations"
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

// NewPool создаёт пул подключений к PostgreSQL и применяет миграции.
func NewPool(cfg config.PostgresConfig, logger *zap.Logger) (*pgxpool.Pool, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgres: DSN is required")
	}

	if err := runMigrations(cfg, logger); err != nil {
		return nil, fmt.Errorf("postgres: failed to run migrations: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to parse config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	poolConfig.MinConns = int32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
	poolConfig.MaxConnIdleTime = cfg.ConnMaxIdleTime

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to create pool: %w", err)
	}

	if err := pingWithRetry(pool, cfg, logger); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: failed to establish connection: %w", err)
	}

	logger.Info("postgres connection established")

	return pool, nil
}

// runMigrations применяет миграции к базе данных через отдельное подключение.
//
// Для миграций используется database/sql с драйвером pgx stdlib,
// так как библиотека golang-migrate требует *sql.DB для создания
// драйвера миграций. Основной пул приложения (pgxpool) создаётся
// отдельно после успешного применения миграций.
//
// Миграции выполняются с таймаутом cfg.MigrationTimeout.
// Если за это время миграции не успевают примениться,
// возвращается ошибка.
func runMigrations(cfg config.PostgresConfig, logger *zap.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return fmt.Errorf("failed to open migration connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping postgres for migrations: %w", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	driver, err := pgx.WithInstance(db, &pgx.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Up()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("failed to run migrations: %w", err)
		}

		if err == migrate.ErrNoChange {
			logger.Info("database schema is up to date")
		} else {
			logger.Info("migrations applied successfully")
		}
		return nil
	case <-time.After(cfg.MigrationTimeout):
		return fmt.Errorf("migration timeout after %s", cfg.MigrationTimeout)
	}
}

// pingWithRetry проверяет подключение с повторными попытками.
func pingWithRetry(pool *pgxpool.Pool, cfg config.PostgresConfig, logger *zap.Logger) error {
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
		err := pool.Ping(ctx)
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
		logger.Warn("failed to ping postgres",
			zap.Int("attempt", attempt),
			zap.Int("max_retries", cfg.MaxRetries),
			zap.Error(err),
		)

		if attempt < cfg.MaxRetries {
			time.Sleep(cfg.RetryInterval)
		}
	}

	return fmt.Errorf("after %d attempts: %w", cfg.MaxRetries, lastErr)
}
