package queue

import "time"

// Backoff returns the capped delay before attempt n.
func Backoff(attempt int, cap time.Duration) time.Duration {
	delay := time.Second << attempt
	if delay > cap {
		return cap
	}
	return delay
}
