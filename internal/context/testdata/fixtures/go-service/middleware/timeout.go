// Package middleware provides HTTP middleware for the service.
package middleware

import (
	"net/http"
	"time"
)

// Timeout wraps h with a fixed request timeout.
func Timeout(h http.Handler, d time.Duration) http.Handler {
	return http.TimeoutHandler(h, d, "request timed out")
}
