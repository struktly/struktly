package clock

import "time"

// Nap is called by nothing the request selects, so it should stay out.
func Nap(d time.Duration) { time.Sleep(d) }
