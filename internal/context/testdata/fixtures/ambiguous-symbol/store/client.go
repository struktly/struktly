package store

// Client reads and writes rows.
type Client struct {
	DSN string
}

// Close releases the connection pool.
func (c *Client) Close() error { return nil }
