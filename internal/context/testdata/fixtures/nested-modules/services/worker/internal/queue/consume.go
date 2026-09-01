package queue

// Consume drains one batch of work.
func Consume(batch []string) int {
	return len(batch)
}
