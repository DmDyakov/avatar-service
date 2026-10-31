package http

import (
	"avatar-service/internal/config"
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

type HealthHandler interface {
	Live(w http.ResponseWriter, r *http.Request)
	Ready(w http.ResponseWriter, r *http.Request)
}

type AvatarHandler interface {
	Upload(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	GetUserAvatar(w http.ResponseWriter, r *http.Request)
}

func New(
	cfg *config.HTTPServerConfig,
	healthHandler HealthHandler,
	avatarHandler AvatarHandler,
	logger *zap.Logger,
) *Server {
	r := chi.NewRouter()
	r.Use(chimw.StripSlashes)

	// Health
	r.Get("/live", healthHandler.Live)
	r.Get("/ready", healthHandler.Ready)
	r.Get("/health", healthHandler.Ready)

	// Web: форма загрузки
	r.Get("/web/upload", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/static/index.html")
	})

	// Avatar API
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/avatars", avatarHandler.Upload)
		r.Get("/avatars/{id}", avatarHandler.Get)
		r.Get("/users/{user_id}/avatar", avatarHandler.GetUserAvatar)
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
