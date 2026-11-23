// Package logger предоставляет функции для логирования.
package logger

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"

	"avatar-service/internal/config"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New создаёт логгер, который пишет и в консоль, и в OpenTelemetry
// Collector (если задан cfg.OTel.Endpoint).
//
// Возвращает cleanup, который нужно вызвать при остановке сервиса —
// он сбрасывает буферы батч-процессора.
//
// Ошибки cleanup (включая провал OTel Shutdown) стоит залогировать вызывающему.
func New(production bool, cfg config.LoggerConfig) (*zap.Logger, func() error, error) {
	var log *zap.Logger
	var err error
	if production {
		log, err = zap.NewProduction()
	} else {
		log, err = zap.NewDevelopment()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("create base logger: %w", err)
	}

	otelCore, otelProvider, err := newOTelLogger(cfg.OTel.ScopeName, cfg.OTel.Endpoint, cfg.OTel.ConnectTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("create OTel logger: %w", err)
	}

	if otelCore != nil {
		log = log.WithOptions(zap.WrapCore(func(orig zapcore.Core) zapcore.Core {
			return zapcore.NewTee(orig, otelCore)
		}))
	}

	cleanup := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		// Sync на stdout/stderr может вернуть syscall.EINVAL (fsync на pipe/tty)
		// или syscall.EPIPE при обрыве канала — для консольного sink'а это не
		// фатально, поэтому такие ошибки не возвращаем.
		syncErr := log.Sync()
		if errors.Is(syncErr, syscall.EINVAL) ||
			errors.Is(syncErr, syscall.EPIPE) {
			syncErr = nil
		}

		var shutdownErr error
		if otelProvider != nil {
			shutdownErr = otelProvider.Shutdown(ctx)
		}

		return errors.Join(syncErr, shutdownErr)
	}

	return log, cleanup, nil
}

// newOTelLogger создаёт OTel-ядро для zap и провайдер логов.
// Возвращает (nil, nil, nil), если endpoint пустой.
func newOTelLogger(scopeName, endpoint string, connectTimeout time.Duration) (zapcore.Core, *sdklog.LoggerProvider, error) {
	if endpoint == "" {
		return nil, nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	exporter, err := otlploggrpc.New(ctx, otlploggrpc.WithEndpoint(endpoint))
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create resource: %w", err)
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	core := otelzap.NewCore(scopeName,
		otelzap.WithLoggerProvider(provider),
	)

	return core, provider, nil
}
