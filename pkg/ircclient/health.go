package ircclient

// Health-related methods are implemented directly on ircClient in connection.go:
// - Connected() bool
// - Healthy() bool (wraps go-ircevo's ValidateConnectionState)
//
// This file exists as a placeholder for future health monitoring features
// such as latency tracking, reconnection metrics, and health check endpoints.
