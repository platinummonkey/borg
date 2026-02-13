package ircclient

import (
	"context"
	"time"
)

// HandlerID identifies a registered event handler for later removal.
type HandlerID int

// MessageEvent represents a PRIVMSG or NOTICE received from IRC.
type MessageEvent struct {
	Channel   string
	Nick      string
	User      string
	Host      string
	Message   string
	IsNotice  bool
	Timestamp time.Time
}

// JoinEvent represents a user joining a channel.
type JoinEvent struct {
	Channel   string
	Nick      string
	User      string
	Host      string
	Timestamp time.Time
}

// PartEvent represents a user leaving a channel.
type PartEvent struct {
	Channel   string
	Nick      string
	User      string
	Host      string
	Message   string
	Timestamp time.Time
}

// KickEvent represents a user being kicked from a channel.
type KickEvent struct {
	Channel   string
	Nick      string
	KickedBy  string
	Message   string
	Timestamp time.Time
}

// ErrorEvent represents an IRC error.
type ErrorEvent struct {
	Message   string
	Timestamp time.Time
}

// ConnectEvent is emitted when the client successfully connects and is registered (001).
type ConnectEvent struct {
	Server    string
	Nick      string
	Timestamp time.Time
}

// DisconnectEvent is emitted when the client disconnects from the server.
type DisconnectEvent struct {
	Server    string
	Timestamp time.Time
}

// MessageHandler handles incoming messages.
type MessageHandler func(MessageEvent)

// JoinHandler handles join events.
type JoinHandler func(JoinEvent)

// PartHandler handles part events.
type PartHandler func(PartEvent)

// KickHandler handles kick events.
type KickHandler func(KickEvent)

// ErrorHandler handles error events.
type ErrorHandler func(ErrorEvent)

// ConnectHandler handles connect events.
type ConnectHandler func(ConnectEvent)

// DisconnectHandler handles disconnect events.
type DisconnectHandler func(DisconnectEvent)

// Client is the interface for an IRC client connection.
type Client interface {
	// Connect establishes a connection to the IRC server.
	Connect(ctx context.Context) error

	// Disconnect cleanly disconnects from the IRC server.
	Disconnect()

	// Connected returns true if currently connected to the server.
	Connected() bool

	// Healthy returns true if the connection is healthy and fully registered.
	Healthy() bool

	// Join joins an IRC channel.
	Join(channel string)

	// Part leaves an IRC channel.
	Part(channel string)

	// JoinedChannels returns the list of channels currently joined.
	JoinedChannels() []string

	// SendMessage sends a PRIVMSG to a target (channel or user).
	SendMessage(target, message string)

	// SendNotice sends a NOTICE to a target (channel or user).
	SendNotice(target, message string)

	// SendRaw sends a raw IRC command.
	SendRaw(message string)

	// Nick returns the current nickname.
	Nick() string

	// SetNick changes the client's nickname.
	SetNick(nick string)

	// OnMessage registers a handler for PRIVMSG events. Returns a HandlerID for removal.
	OnMessage(handler MessageHandler) HandlerID

	// OnJoin registers a handler for JOIN events.
	OnJoin(handler JoinHandler) HandlerID

	// OnPart registers a handler for PART events.
	OnPart(handler PartHandler) HandlerID

	// OnKick registers a handler for KICK events.
	OnKick(handler KickHandler) HandlerID

	// OnError registers a handler for ERROR events.
	OnError(handler ErrorHandler) HandlerID

	// OnConnect registers a handler for successful connection (001).
	OnConnect(handler ConnectHandler) HandlerID

	// OnDisconnect registers a handler for disconnection.
	OnDisconnect(handler DisconnectHandler) HandlerID

	// RemoveHandler removes a previously registered handler.
	RemoveHandler(id HandlerID)

	// Wait blocks until the connection is closed.
	Wait() error
}
