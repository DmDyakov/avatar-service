// Package logger предоставляет функции для логирования.
package logger

import (
	"go.uber.org/zap"
)

type Config interface {
	IsProd() bool
}

// New создаёт логгер в зависимости от окружения.
func New(cfg Config) (*zap.Logger, error) {
	if cfg.IsProd() {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}
