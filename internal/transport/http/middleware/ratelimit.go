// Package middleware содержит HTTP-middleware.
package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"avatar-service/internal/transport/http/handlers"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

const (
	sweepInterval     = 5 * time.Minute
	entryTTL          = 10 * time.Minute
	retryAfterSeconds = 1
)

// RateLimiter — per-IP rate limiting на token bucket.
//
// Хранит *rate.Limiter для каждого IP; старые записи чистит фоновым
// Sweep'ом, чтобы map не рос бесконечно при потоке уникальных IP.
type RateLimiter struct {
	logger *zap.Logger
	rps    float64
	burst  int

	mu      sync.Mutex
	entries map[string]*rateEntry
}

type rateEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter создаёт лимитер: rps запросов в секунду, burst — всплеск.
func NewRateLimiter(rps float64, burst int, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		logger:  logger,
		rps:     rps,
		burst:   burst,
		entries: make(map[string]*rateEntry),
	}
}

// Middleware ограничивает запросы per-IP.
//
// При превышении — 429 с Retry-After и JSON-телом (ТЗ п. 4).
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			rl.logger.Warn("rate limit exceeded",
				zap.String("ip", ip),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
			)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
			handlers.RespondError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getLimiter возвращает (или создаёт) лимитер для IP и обновляет lastSeen.
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, ok := rl.entries[ip]
	if !ok {
		e = &rateEntry{limiter: rate.NewLimiter(rate.Limit(rl.rps), rl.burst)}
		rl.entries[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// Sweep запускает фоновую очистку простаивающих IP до отмены ctx.
func (rl *RateLimiter) Sweep(ctx context.Context) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rl.cleanup()
		}
	}
}

// cleanup удаляет записи, к которым не обращались дольше entryTTL.
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-entryTTL)
	removed := 0
	for ip, e := range rl.entries {
		if e.lastSeen.Before(cutoff) {
			delete(rl.entries, ip)
			removed++
		}
	}
	if removed > 0 {
		rl.logger.Debug("rate limiter cleanup", zap.Int("removed", removed))
	}
}

// extractIP вырезает IP из RemoteAddr (`ip:port` → `ip`).
func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
