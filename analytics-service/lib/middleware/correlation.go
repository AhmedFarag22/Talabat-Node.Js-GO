// Package middleware holds chi-compatible HTTP middleware: correlation id,
// request logging, recovery (chi has its own; we add an envelope on top).
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/quickbite/analytics-service/lib/appcontext"
	"github.com/quickbite/analytics-service/lib/logger"
)

const HeaderCorrelationID = "X-Correlation-ID"

// Correlation reads X-Correlation-ID (or generates one), stashes it in the
// request context, echoes it on the response. Mirrors
// lib/correlation/correlation.middleware.ts in the Node services.
func Correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get(HeaderCorrelationID)
		if cid == "" {
			cid = uuid.NewString()
		}
		w.Header().Set(HeaderCorrelationID, cid)
		ctx := appcontext.WithCorrelationID(r.Context(), cid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AccessLog logs one structured line per request after the handler returns.
// Status code is captured via a small ResponseWriter wrapper.
func AccessLog(base *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			logger.FromContext(r.Context(), base).Info("http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Duration("dur", time.Since(start)),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
