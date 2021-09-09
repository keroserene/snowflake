package snowflake_client

// Tongue is an interface for catching Snowflakes. (aka the remote dialer)
type Tongue interface {
	// Catch makes a connection to a new snowflake.
	Catch() (*WebRTCPeer, error)

	// GetMax returns the maximum number of snowflakes a client can have.
	GetMax() int
}

// SnowflakeCollector is an interface for managing a client's collection of snowflakes.
type SnowflakeCollector interface {
	// Collect adds a snowflake to the collection.
	// The implementation of Collect should decide how to connect to and maintain
	// the connection to the WebRTCPeer.
	Collect() (*WebRTCPeer, error)

	// Pop removes and returns the most available snowflake from the collection.
	Pop() *WebRTCPeer

	// Melted returns a channel that will signal when the collector has stopped.
	Melted() <-chan struct{}
}
