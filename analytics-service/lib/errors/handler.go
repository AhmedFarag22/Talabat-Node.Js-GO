package errors

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Handler is a chi-compatible signature: handler returns error, the wrapper
// turns it into an envelope. The Node services do this via Express
// middleware; in Go we wrap each handler explicitly so the type is honest.
type Handler func(w http.ResponseWriter, r *http.Request) error

// Wrap converts a Handler into the http.HandlerFunc chi expects, rendering
// any returned error through Render.
func Wrap(logger *slog.Logger, h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic in handler",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				Render(w, &AppError{
					Code:    "INTERNAL_ERROR",
					Status:  http.StatusInternalServerError,
					Message: "internal server error",
				})
			}
		}()
		if err := h(w, r); err != nil {
			ae := As(err)
			if ae.Status >= 500 {
				logger.Error("request failed", slog.String("code", ae.Code), slog.String("error", ae.Error()))
			}
			Render(w, ae)
		}
	}
}

// Render writes the AppError as the standard JSON envelope.
func Render(w http.ResponseWriter, ae *AppError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(ae.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error": map[string]any{
			"code":    ae.Code,
			"message": ae.Message,
		},
	})
}
