package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("rate_limit:%s:%s", r.URL.Path, clientIP(r))
		count, err := s.redis.Incr(r.Context(), key).Result()
		if err != nil {
			s.logger.ErrorContext(r.Context(), "redis rate limit failed", slog.String("error", err.Error()))
			next.ServeHTTP(w, r)
			return
		}
		if count == 1 {
			_ = s.redis.Expire(r.Context(), key, s.cfg.RateLimit.Window).Err()
		}
		if count > int64(s.cfg.RateLimit.Limit) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.logger.DebugContext(r.Context(), "request completed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", time.Since(started)),
		)
	})
}
