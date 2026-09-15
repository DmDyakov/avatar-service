// Package handlers содержит HTTP-обработчики запросов.
package handlers

import (
	"context"
	"net/http"

	"go.uber.org/zap"
)

//go:generate mockgen -destination=mocks/mock_health_service.go -package=mocks . HealthService
type HealthService interface {
	Ping(ctx context.Context) error
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

// HealthDB обрабатывает запрос проверки подключения к БД.
func (h *HealthHandler) HealthDB(w http.ResponseWriter, r *http.Request) {
	err := h.service.Ping(r.Context())
	if err != nil {
		h.logger.Error("Database ping failed", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

}
