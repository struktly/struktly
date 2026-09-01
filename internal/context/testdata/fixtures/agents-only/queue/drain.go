package queue

// Drain removes every message the handler accepts.
func Drain(messages []string, handle func(string) bool) []string {
	var remaining []string
	for _, message := range messages {
		if !handle(message) {
			remaining = append(remaining, message)
		}
	}
	return remaining
}
