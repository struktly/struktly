// Package clock supplies the wall-clock source middleware depends on.
package clock

import "time"

// Grace is the deadline applied when none is configured.
const Grace = 30 * time.Second

// Wall returns the current instant.
func Wall() time.Time { return time.Now() }
