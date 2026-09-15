// Package http реализует HTTP-сервер.
package http

import (
	"avatar-service/internal/config"
	"avatar-service/internal/transport/http/handlers"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type Server struct {
	*http.Server
	logger          *zap.Logger
	shutdownTimeout time.Duration
}

// New создаёт HTTP-сервер с настроенными маршрутами и middleware.
func New(
	cfg *config.HTTPServerConfig,
	healthHandler *handlers.HealthHandler,
	avatarHandler *handlers.AvatarHandler,
	logger *zap.Logger,
) *Server {
	r := chi.NewRouter()
	r.Use(chimw.StripSlashes)

	// Health
	r.Get("/ping", healthHandler.HealthDB)

	// Avatar API
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/avatars", avatarHandler.Upload)
		r.Get("/avatars/{id}", avatarHandler.Get)
		r.Get("/avatars/{id}/metadata", avatarHandler.GetMetadata)
		r.Delete("/avatars/{id}", avatarHandler.Delete)
		r.Get("/users/{user_id}/avatar", avatarHandler.GetUserAvatar)
		r.Get("/users/{user_id}/avatars", avatarHandler.ListByUserID)
	})

	srv := &http.Server{
		Addr:         cfg.Address,
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &Server{
		Server:          srv,
		logger:          logger,
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

// Run запускает HTTP-сервер.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("HTTP server started", zap.String("addr", s.Addr))

	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()

	<-ctx.Done()
	s.logger.Info("Shutting down HTTP server...")
	shCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.Shutdown(shCtx); err != nil {
		s.logger.Error("HTTP server failed to shutdown", zap.Error(err))
	}
	s.logger.Info("HTTP server stopped gracefully")
	return nil
}
