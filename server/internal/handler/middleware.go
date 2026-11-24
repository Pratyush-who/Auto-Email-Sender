package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// StructuredLoggerMiddleware logs each incoming HTTP request using slog.
func StructuredLoggerMiddleware(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				duration := time.Since(start)
				status := ww.Status()
				if status == 0 {
					status = http.StatusOK
				}

				attrs := []slog.Attr{
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("remote_addr", r.RemoteAddr),
					slog.Int("status", status),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Duration("duration", duration),
					slog.String("request_id", middleware.GetReqID(r.Context())),
				}

				if status >= 500 {
					logger.LogAttrs(r.Context(), slog.LevelError, "http request completed with error", attrs...)
				} else if status >= 400 {
					logger.LogAttrs(r.Context(), slog.LevelWarn, "http request completed with client error", attrs...)
				} else {
					logger.LogAttrs(r.Context(), slog.LevelInfo, "http request completed", attrs...)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
