package middleware

import (
	"net/http"
	"strings"

	"avatar-service/internal/config"

	"go.uber.org/zap"
)

const (
	corsAllowMethods = "GET, POST, DELETE, OPTIONS"
	corsAllowHeaders = "X-User-ID, Content-Type"
	corsMaxAge       = "3600"
)

// CORS возвращает middleware для кросс-доменных запросов к /api/v1.
func CORS(cfg config.CORSConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowed[strings.TrimRight(o, "/")] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")

			// Origin отсутствует — не CORS-запрос (curl, сервер-сервер).
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			_, isAllowed := allowed[origin]

			// Preflight — отвечаем сразу, до rate limit и хендлеров.
			if r.Method == http.MethodOptions {
				if isAllowed {
					writeCORSHeaders(w, origin)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if isAllowed {
				writeCORSHeaders(w, origin)
			} else {
				logger.Debug("cors: origin not allowed",
					zap.String("origin", origin),
					zap.String("path", r.URL.Path),
				)
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeCORSHeaders(w http.ResponseWriter, origin string) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
	w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
	w.Header().Set("Access-Control-Max-Age", corsMaxAge)
	w.Header().Add("Vary", "Origin")
}
