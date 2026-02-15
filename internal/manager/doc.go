// Package manager provides the central management plane for observing,
// spawning, and monitoring multiple agent-chat agents.
//
// The [Manager] connects to IRC as an observer, watches all protocol
// messages to populate local stores, polls agent dashboard endpoints
// for health/metrics, and serves a web UI with WebSocket live updates.
package manager
