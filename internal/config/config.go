// Package config загружает конфигурацию приложения.
package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// --------------------------------------
// Config — общая конфигурация приложения
// --------------------------------------

type Config struct {
	AppEnv          string        `env:"APP_ENV" envDefault:"dev" validate:"oneof=dev staging prod"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s" validate:"gt=0"`
	HTTPServer      HTTPServerConfig
	Postgres        PostgresConfig
	S3              S3Config
}

// Load — загружает конфигурацию из переменных окружения.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}

// IsProd — проверяет, является ли окружение production
func (c *Config) IsProd() bool {
	return c.AppEnv == "prod"
}

// IsStaging — проверяет, является ли окружение staging
func (c *Config) IsStaging() bool {
	return c.AppEnv == "staging"
}

// IsDev — проверяет, является ли окружение development
func (c *Config) IsDev() bool {
	return c.AppEnv == "dev"
}

// --------------------------------------
// HTTPServerConfig — конфигурация HTTP сервера
// --------------------------------------

type HTTPServerConfig struct {
	Address         string        `env:"HTTP_ADDRESS" envDefault:"localhost:8080" validate:"required"`
	ReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s" validate:"gt=0"`
	WriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"5s" validate:"gt=0"`
	IdleTimeout     time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"30s" validate:"gt=0"`
	ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"5s" validate:"gt=0"`
	TLSCertFile     string        `env:"TLS_CERT_FILE"`
	TLSKeyFile      string        `env:"TLS_KEY_FILE"`
}

// --------------------------------------
// PostgresConfig — конфигурация подключения к PostgreSQL
// --------------------------------------

type PostgresConfig struct {
	DSN string `env:"POSTGRES_DSN,required,notEmpty"`

	MaxOpenConns    int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"25" validate:"gte=1"`
	MaxIdleConns    int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5" validate:"gte=0"`
	ConnMaxLifetime time.Duration `env:"POSTGRES_CONN_MAX_LIFETIME" envDefault:"5m" validate:"gt=0"`
	ConnMaxIdleTime time.Duration `env:"POSTGRES_CONN_MAX_IDLE_TIME" envDefault:"5m" validate:"gt=0"`

	ConnectTimeout time.Duration `env:"POSTGRES_CONNECT_TIMEOUT" envDefault:"10s" validate:"gt=0"`
	MaxRetries     int           `env:"POSTGRES_MAX_RETRIES" envDefault:"5" validate:"gte=1"`
	RetryInterval  time.Duration `env:"POSTGRES_RETRY_INTERVAL" envDefault:"3s" validate:"gt=0"`

	MigrateOnStart   bool          `env:"POSTGRES_MIGRATE_ON_START" envDefault:"true"`
	MigrationTimeout time.Duration `env:"POSTGRES_MIGRATION_TIMEOUT" envDefault:"30s" validate:"gt=0"`
}

// DSNRedacted возвращает строку подключения без пароля для логирования.
func (c PostgresConfig) DSNRedacted() string {
	u, err := url.Parse(c.DSN)
	if err != nil {
		return "invalid-dsn"
	}

	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
	}

	return u.String()
}

// --------------------------------------
// S3Config — конфигурация S3 хранилища
// --------------------------------------

type S3Config struct {
	Endpoint       string        `env:"S3_ENDPOINT" envDefault:"localhost:9000" validate:"required"`
	AccessKey      string        `env:"S3_ACCESS_KEY" envDefault:"minioadmin" validate:"required"`
	SecretKey      string        `env:"S3_SECRET_KEY" envDefault:"minioadmin" validate:"required"`
	Bucket         string        `env:"S3_BUCKET" envDefault:"avatars" validate:"required"`
	UseSSL         bool          `env:"S3_USE_SSL" envDefault:"false"`
	Region         string        `env:"S3_REGION" envDefault:"us-east-1"`
	ConnectTimeout time.Duration `env:"S3_CONNECT_TIMEOUT" envDefault:"30s" validate:"gt=0"`
}
