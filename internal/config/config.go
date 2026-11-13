// Package config загружает конфигурацию приложения.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
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
	RabbitMQ        RabbitMQConfig
	Worker          WorkerConfig
	RateLimit       RateLimitConfig
	CORS            CORSConfig
}

// Load — загружает конфигурацию из переменных окружения.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Собираем DSN и URL из отдельных полей. Это позволяет экранировать
	// спецсимволы в паролях (@, :, /) и держать один источник истины —
	// отдельные поля POSTGRES_USER/PASSWORD/... и RABBITMQ_USER/PASSWORD/...
	if err := cfg.Postgres.buildDSN(); err != nil {
		return nil, fmt.Errorf("build postgres DSN: %w", err)
	}
	if err := cfg.RabbitMQ.buildURL(); err != nil {
		return nil, fmt.Errorf("build rabbitmq URL: %w", err)
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
// PostgresConfig — конфигурация подключения к PostgreSQL.
//
// DSN — вычисляемое поле: заполняется в Load() методом buildDSN() из
// отдельных полей. Это позволяет экранировать спецсимволы в пароле
// (@, :, /) и держать один источник истины — поля POSTGRES_USER/PASSWORD/...
//
// POSTGRES_PASSWORD — required, без дефолта: если не задан, приложение
// падает при загрузке конфига с понятной ошибкой.
// --------------------------------------

type PostgresConfig struct {
	// DSN вычисляется в Load() из полей ниже.
	DSN string `env:"-"`

	User     string `env:"POSTGRES_USER" envDefault:"postgres"`
	Password string `env:"POSTGRES_PASSWORD,required"`
	Host     string `env:"POSTGRES_HOST" envDefault:"localhost"`
	Port     int    `env:"POSTGRES_PORT" envDefault:"5432"`
	DB       string `env:"POSTGRES_DB" envDefault:"avatars"`
	SSLMode  string `env:"POSTGRES_SSLMODE" envDefault:"disable"`

	MaxOpenConns    int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"25" validate:"gte=1"`
	MaxIdleConns    int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5" validate:"gte=0"`
	ConnMaxLifetime time.Duration `env:"POSTGRES_CONN_MAX_LIFETIME" envDefault:"5m" validate:"gt=0"`
	ConnMaxIdleTime time.Duration `env:"POSTGRES_CONN_MAX_IDLE_TIME" envDefault:"5m" validate:"gt=0"`

	ConnectTimeout   time.Duration `env:"POSTGRES_CONNECT_TIMEOUT" envDefault:"10s" validate:"gt=0"`
	MaxRetries       int           `env:"POSTGRES_MAX_RETRIES" envDefault:"5" validate:"gte=1"`
	RetryInterval    time.Duration `env:"POSTGRES_RETRY_INTERVAL" envDefault:"3s" validate:"gt=0"`
	MigrateOnStart   bool          `env:"POSTGRES_MIGRATE_ON_START" envDefault:"true"`
	MigrationTimeout time.Duration `env:"POSTGRES_MIGRATION_TIMEOUT" envDefault:"30s" validate:"gt=0"`
}

// buildDSN собирает строку подключения из отдельных полей и записывает в c.DSN.
// url.UserPassword экранирует спецсимволы в пароле.
func (c *PostgresConfig) buildDSN() error {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   "/" + c.DB,
	}

	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()

	c.DSN = u.String()
	return nil
}

// DSNRedacted возвращает строку подключения без пароля для логирования.
// Собирается вручную (не через url.String()), чтобы маска `***` не
// превращалась в `%2A%2A%2A` из-за percent-encoding в userinfo.
func (c PostgresConfig) DSNRedacted() string {
	u, err := url.Parse(c.DSN)
	if err != nil {
		return "invalid-dsn"
	}

	if u.User == nil {
		return u.String()
	}
	if _, has := u.User.Password(); !has {
		return u.String()
	}

	// Собираем вручную: scheme://username:***@host/path?query
	s := fmt.Sprintf("%s://%s:***@%s%s", u.Scheme, u.User.Username(), u.Host, u.Path)
	if u.RawQuery != "" {
		s += "?" + u.RawQuery
	}
	return s
}

// --------------------------------------
// S3Config — конфигурация S3 хранилища.
//
// S3_ACCESS_KEY и S3_SECRET_KEY — required, без дефолтов.
// --------------------------------------

type S3Config struct {
	Endpoint       string        `env:"S3_ENDPOINT" envDefault:"localhost:9000" validate:"required"`
	AccessKey      string        `env:"S3_ACCESS_KEY,required"`
	SecretKey      string        `env:"S3_SECRET_KEY,required"`
	Bucket         string        `env:"S3_BUCKET" envDefault:"avatars" validate:"required"`
	UseSSL         bool          `env:"S3_USE_SSL" envDefault:"false"`
	Region         string        `env:"S3_REGION" envDefault:"us-east-1"`
	ConnectTimeout time.Duration `env:"S3_CONNECT_TIMEOUT" envDefault:"30s" validate:"gt=0"`
	ForcePathStyle bool          `env:"S3_FORCE_PATH_STYLE" envDefault:"false"`
}

// --------------------------------------
// RabbitMQConfig — конфигурация RabbitMQ.
//
// URL — вычисляемое поле: заполняется в Load() методом buildURL() из
// отдельных полей. Это позволяет экранировать спецсимволы в пароле и
// нормализовать vhost.
//
// RABBITMQ_PASSWORD — required, без дефолта.
// --------------------------------------

type RabbitMQConfig struct {
	// URL вычисляется в Load() из полей ниже.
	URL string `env:"-"`

	User     string `env:"RABBITMQ_USER" envDefault:"guest"`
	Password string `env:"RABBITMQ_PASSWORD,required"`
	Host     string `env:"RABBITMQ_HOST" envDefault:"localhost"`
	Port     int    `env:"RABBITMQ_PORT" envDefault:"5672"`
	VHost    string `env:"RABBITMQ_VHOST" envDefault:"/"`

	Heartbeat      time.Duration `env:"RABBITMQ_HEARTBEAT" envDefault:"60s"`
	PrefetchCount  int           `env:"RABBITMQ_PREFETCH" envDefault:"10"`
	ConnectTimeout time.Duration `env:"RABBITMQ_CONNECT_TIMEOUT" envDefault:"30s"`
}

// buildURL собирает amqp://-URL из отдельных полей и записывает в c.URL.
func (c *RabbitMQConfig) buildURL() error {
	vhost := c.VHost
	if vhost == "" {
		vhost = "/"
	}
	if !strings.HasPrefix(vhost, "/") {
		vhost = "/" + vhost
	}

	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   vhost,
	}

	c.URL = u.String()
	return nil
}

// URLRedacted возвращает URL без пароля для логирования.
// Собирается вручную (не через url.String()), чтобы маска `***` не
// превращалась в `%2A%2A%2A` из-за percent-encoding в userinfo.
func (c RabbitMQConfig) URLRedacted() string {
	u, err := url.Parse(c.URL)
	if err != nil {
		return "invalid-url"
	}

	if u.User == nil {
		return u.String()
	}
	if _, has := u.User.Password(); !has {
		return u.String()
	}

	// Собираем вручную: scheme://username:***@host/path
	return fmt.Sprintf("%s://%s:***@%s%s", u.Scheme, u.User.Username(), u.Host, u.Path)
}

// --------------------------------------
// WorkerConfig — конфигурация воркера (retry-политика фоновой обработки).
// --------------------------------------

type WorkerConfig struct {
	// RetryMaxAttempts — сколько всего попыток сделать (1 = без retry).
	RetryMaxAttempts int `env:"WORKER_RETRY_MAX_ATTEMPTS" envDefault:"3" validate:"gte=1"`
	// RetryInitialBackoff — задержка перед 2-й попыткой.
	RetryInitialBackoff time.Duration `env:"WORKER_RETRY_INITIAL_BACKOFF" envDefault:"2s" validate:"gt=0"`
	// RetryMultiplier — множитель backoff (2.0 = 2s → 4s → 8s).
	RetryMultiplier float64 `env:"WORKER_RETRY_MULTIPLIER" envDefault:"2.0" validate:"gte=1"`
}

// --------------------------------------
// RateLimitConfig — лимиты запросов на API.
//
// Разделены на общий (для /api/v1/*) и upload (для POST /avatars —
// операция дорогая: S3 write, декодирование, publish в RabbitMQ).
// --------------------------------------

type RateLimitConfig struct {
	RPS         float64 `env:"RATE_LIMIT_RPS" envDefault:"10" validate:"gt=0"`
	Burst       int     `env:"RATE_LIMIT_BURST" envDefault:"20" validate:"gte=1"`
	UploadRPS   float64 `env:"RATE_LIMIT_UPLOAD_RPS" envDefault:"2" validate:"gt=0"`
	UploadBurst int     `env:"RATE_LIMIT_UPLOAD_BURST" envDefault:"5" validate:"gte=1"`
}

// --------------------------------------
// CORSConfig — настройки CORS для /api/v1.
//
// AllowedOrigins читается из env как строка через запятую; envSeparator
// разбивает на []string. Дефолт — dev-origins (форма на :3000,
// сервер на :8080).
// --------------------------------------

type CORSConfig struct {
	AllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:3000,http://localhost:8080" envSeparator:","`
}
