// Package ircclient provides an IRC client for agent-to-agent communication.
//
// The client wraps go-ircevo with mandatory TLS and SASL authentication,
// a token-bucket rate limiter for outgoing messages, and exponential
// backoff with jitter for automatic reconnection.
//
// Create a client with [NewClient], connect with [Client.Connect], and
// register event handlers (OnMessage, OnJoin, etc.) to react to IRC events.
package ircclient
