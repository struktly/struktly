package http

// Client issues outbound requests.
type Client struct {
	BaseURL string
}

// Close releases idle connections.
func (c *Client) Close() error { return nil }
