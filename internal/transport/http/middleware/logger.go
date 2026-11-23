// Package middleware содержит HTTP middleware.
package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// RequestLogger логирует каждый HTTP-запрос через zap.Logger:
// метод, путь, статус-код, длительность, адрес и User-Agent.
func RequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("request completed",
				zap.String("http.method", r.Method),
				zap.String("http.path", r.URL.Path),
				zap.Int("http.status_code", ww.Status()),
				zap.Duration("http.duration_ms", time.Since(start)),
				zap.String("http.remote_addr", r.RemoteAddr),
				zap.String("http.user_agent", r.UserAgent()),
			)
		})
	}
}