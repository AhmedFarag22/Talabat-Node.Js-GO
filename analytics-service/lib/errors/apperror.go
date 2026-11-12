// Package errors defines AppError, the typed user-facing error this service
// throws. Services and controllers return AppError{...}; the error middleware
// renders it as a JSON envelope. Equivalent to lib/error/AppError.ts in the
// Node services.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type AppError struct {
	Code    string
	Status  int
	Message string
	cause   error
}

func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.cause }

// WithCause wraps a low-level error, preserving the AppError's user-facing
// shape. Use when bubbling up an unexpected failure (e.g. mongo timeout).
func (e *AppError) WithCause(cause error) *AppError {
	cp := *e
	cp.cause = cause
	return &cp
}

// New builds a fresh AppError. Prefer declaring module-level vars in
// app/<module>/errors.go rather than constructing inline at call sites.
func New(code string, status int, message string) *AppError {
	return &AppError{Code: code, Status: status, Message: message}
}

// As resolves an error chain to an *AppError, or returns the canonical 500
// fallback if the error isn't one of ours.
func As(err error) *AppError {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return &AppError{
		Code:    "INTERNAL_ERROR",
		Status:  http.StatusInternalServerError,
		Message: "internal server error",
		cause:   err,
	}
}
