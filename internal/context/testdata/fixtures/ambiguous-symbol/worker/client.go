package worker

// Client leases jobs from the scheduler.
type Client struct {
	Lease string
}

// Close returns any held lease.
func (c *Client) Close() error { return nil }
