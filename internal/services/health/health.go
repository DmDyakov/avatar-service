// Package health реализует сервис проверки состояния приложения.
package health

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service проверяет доступность зависимостей приложения.
type Service struct {
	pg *pgxpool.Pool
}

// New создаёт сервис проверки здоровья.
func New(pg *pgxpool.Pool) *Service {
	return &Service{pg: pg}
}

// Ping проверяет подключение к БД.
func (s *Service) Ping(ctx context.Context) error {
	return s.pg.Ping(ctx)
}
