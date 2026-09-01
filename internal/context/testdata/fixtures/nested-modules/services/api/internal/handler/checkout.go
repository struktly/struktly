package handler

import "example.com/nested-modules/pkg/telemetry"

// Checkout completes a cart and records the outcome.
func Checkout(cart []string) error {
	telemetry.Count("checkout")
	return nil
}
