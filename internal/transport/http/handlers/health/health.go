// Package handlers содержит HTTP-обработчики запросов.
package handlers

import (
	"context"
	"net/http"

	"avatar-service/internal/services/health"
	"avatar-service/internal/transport/http/handlers"

	"go.uber.org/zap"
)

//go:generate mockgen -destination=mocks/mock_health_service.go -package=mocks avatar-service/internal/transport/http/handlers HealthService
type HealthService interface {
	Check(ctx context.Context) (*health.Status, int)
}

type HealthHandler struct {
	service HealthService
	logger  *zap.Logger
}

func NewHealthHandler(service HealthService, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		service: service,
		logger:  logger,
	}
}

// Live — liveness probe. Проверяет, что процесс жив.
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Ready — readiness probe. Проверяет зависимости.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	status, code := h.service.Check(r.Context())

	if code != http.StatusOK {
		h.logger.Warn("readiness degraded",
			zap.String("status", status.Status),
			zap.Any("components", status.Components),
		)
	}

	handlers.RespondJSON(w, code, status)
}
