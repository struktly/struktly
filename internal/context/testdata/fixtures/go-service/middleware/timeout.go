// Package middleware provides HTTP middleware for the service.
package middleware

import (
	"net/http"

	"example.com/go-service/internal/clock"
)

// Timeout wraps h with a fixed request timeout.
func Timeout(h http.Handler) http.Handler {
	return http.TimeoutHandler(h, clock.Grace, "request timed out")
}
