package http

import (
	"avatar-service/internal/config"
	"context"
	"errors"
	"net/http"
	"time"

	"avatar-service/internal/transport/http/middleware"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type Server struct {
	*http.Server
	logger          *zap.Logger
	shutdownTimeout time.Duration
	generalLimiter  *middleware.RateLimiter
	uploadLimiter   *middleware.RateLimiter
}

type HealthHandler interface {
	Live(w http.ResponseWriter, r *http.Request)
	Ready(w http.ResponseWriter, r *http.Request)
}

type AvatarHandler interface {
	Upload(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	GetUserAvatar(w http.ResponseWriter, r *http.Request)
	GetMetadata(w http.ResponseWriter, r *http.Request)
	ListByUserID(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	DeleteUserAvatar(w http.ResponseWriter, r *http.Request)
	PostUpload(w http.ResponseWriter, r *http.Request)
}

func New(
	cfg *config.HTTPServerConfig,
	rateLimitCfg config.RateLimitConfig,
	healthHandler HealthHandler,
	avatarHandler AvatarHandler,
	logger *zap.Logger,
) *Server {
	generalLimiter := middleware.NewRateLimiter(rateLimitCfg.RPS, rateLimitCfg.Burst, logger)
	uploadLimiter := middleware.NewRateLimiter(rateLimitCfg.UploadRPS, rateLimitCfg.UploadBurst, logger)

	r := chi.NewRouter()
	r.Use(chimw.StripSlashes)

	// Health — без rate limit (K8s probes).
	r.Get("/live", healthHandler.Live)
	r.Get("/ready", healthHandler.Ready)
	r.Get("/health", healthHandler.Ready)

	// Web — без rate limit (не JSON API).
	r.Get("/web/upload", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/static/index.html")
	})
	r.Post("/web/upload", avatarHandler.PostUpload)
	r.Get("/web/gallery/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/static/gallery.html")
	})

	// API — общий лимит на чтение, отдельный (строже) на upload.
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(generalLimiter.Middleware)

			r.Get("/avatars/{id}", avatarHandler.Get)
			r.Delete("/avatars/{id}", avatarHandler.Delete)
			r.Get("/avatars/{id}/metadata", avatarHandler.GetMetadata)
			r.Get("/users/{user_id}/avatar", avatarHandler.GetUserAvatar)
			r.Delete("/users/{user_id}/avatar", avatarHandler.DeleteUserAvatar)
			r.Get("/users/{user_id}/avatars", avatarHandler.ListByUserID)
		})

		r.With(uploadLimiter.Middleware).Post("/avatars", avatarHandler.Upload)
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
		generalLimiter:  generalLimiter,
		uploadLimiter:   uploadLimiter,
	}
}

func (s *Server) Run(ctx context.Context) error {
	// Фоновая очистка простаивающих IP.
	go s.generalLimiter.Sweep(ctx)
	go s.uploadLimiter.Sweep(ctx)

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
