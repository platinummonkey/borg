package ircclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"time"

	irc "github.com/kofany/go-ircevo"
)

// ircClient implements Client using go-ircevo.
type ircClient struct {
	cfg  Config
	conn *irc.Connection

	mu       sync.RWMutex
	channels map[string]bool

	limiter *RateLimiter
	backoff *Backoff

	handlerMu    sync.RWMutex
	nextID       HandlerID
	msgHandlers  map[HandlerID]MessageHandler
	joinHandlers map[HandlerID]JoinHandler
	partHandlers map[HandlerID]PartHandler
	kickHandlers map[HandlerID]KickHandler
	errHandlers  map[HandlerID]ErrorHandler
	connHandlers map[HandlerID]ConnectHandler
	discHandlers map[HandlerID]DisconnectHandler
	// Track which "category" each HandlerID belongs to for RemoveHandler.
	handlerKinds map[HandlerID]string

	done chan error
}

// NewClient creates a new IRC client from the given config.
func NewClient(cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	realName := cfg.RealName
	if realName == "" {
		realName = cfg.Nick
	}

	conn := irc.IRC(cfg.Nick, cfg.Username)
	if conn == nil {
		return nil, fmt.Errorf("failed to create IRC connection (invalid nick or user)")
	}

	conn.RealName = realName
	conn.UseTLS = true
	conn.TLSConfig = &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}
	conn.UseSASL = true
	conn.SASLLogin = cfg.Username
	conn.SASLPassword = cfg.Password
	conn.SASLMech = cfg.SASLMech
	if conn.SASLMech == "" {
		conn.SASLMech = "PLAIN"
	}

	conn.QuitMessage = cfg.QuitMessage
	conn.Debug = cfg.Debug
	conn.HandleErrorAsDisconnect = true
	conn.SmartErrorHandling = true

	// When we manage our own reconnect with backoff, disable go-ircevo's
	// internal reconnect to avoid conflicting retry logic.
	if cfg.ReconnectBackoff > 0 && cfg.MaxReconnectBackoff > 0 {
		conn.MaxRecoverableReconnects = 0
	} else if cfg.MaxReconnectAttempts > 0 {
		conn.MaxRecoverableReconnects = cfg.MaxReconnectAttempts
	}
	if cfg.PingFrequency > 0 {
		conn.PingFreq = cfg.PingFrequency
	}
	if cfg.Timeout > 0 {
		conn.Timeout = cfg.Timeout
	}

	// Silence go-ircevo's internal logger unless in debug mode.
	if !cfg.Debug {
		conn.Log = log.New(log.Writer(), "", 0)
	}

	c := &ircClient{
		cfg:          cfg,
		conn:         conn,
		channels:     make(map[string]bool),
		msgHandlers:  make(map[HandlerID]MessageHandler),
		joinHandlers: make(map[HandlerID]JoinHandler),
		partHandlers: make(map[HandlerID]PartHandler),
		kickHandlers: make(map[HandlerID]KickHandler),
		errHandlers:  make(map[HandlerID]ErrorHandler),
		connHandlers: make(map[HandlerID]ConnectHandler),
		discHandlers: make(map[HandlerID]DisconnectHandler),
		handlerKinds: make(map[HandlerID]string),
		done:         make(chan error, 1),
	}

	if cfg.RateLimit > 0 {
		c.limiter = NewRateLimiter(cfg.RateLimit, cfg.RateLimitBurst)
	}
	if cfg.ReconnectBackoff > 0 && cfg.MaxReconnectBackoff > 0 {
		c.backoff = NewBackoff(cfg.ReconnectBackoff, cfg.MaxReconnectBackoff)
	}

	c.registerInternalCallbacks()

	return c, nil
}

// registerInternalCallbacks sets up go-ircevo callbacks that dispatch to our typed handlers.
func (c *ircClient) registerInternalCallbacks() {
	c.conn.AddCallback("PRIVMSG", func(e *irc.Event) {
		ev := MessageEvent{
			Channel:   e.Arguments[0],
			Nick:      e.Nick,
			User:      e.User,
			Host:      e.Host,
			Message:   e.Message(),
			IsNotice:  false,
			Timestamp: time.Now(),
		}
		c.handlerMu.RLock()
		defer c.handlerMu.RUnlock()
		for _, h := range c.msgHandlers {
			h(ev)
		}
	})

	c.conn.AddCallback("NOTICE", func(e *irc.Event) {
		// Skip server notices during registration (no nick).
		if e.Nick == "" {
			return
		}
		ev := MessageEvent{
			Channel:   e.Arguments[0],
			Nick:      e.Nick,
			User:      e.User,
			Host:      e.Host,
			Message:   e.Message(),
			IsNotice:  true,
			Timestamp: time.Now(),
		}
		c.handlerMu.RLock()
		defer c.handlerMu.RUnlock()
		for _, h := range c.msgHandlers {
			h(ev)
		}
	})

	c.conn.AddCallback("JOIN", func(e *irc.Event) {
		channel := e.Arguments[0]
		if e.Nick == c.conn.GetNick() {
			c.mu.Lock()
			c.channels[channel] = true
			c.mu.Unlock()
			slog.Info("joined channel", "channel", channel)
		}
		ev := JoinEvent{
			Channel:   channel,
			Nick:      e.Nick,
			User:      e.User,
			Host:      e.Host,
			Timestamp: time.Now(),
		}
		c.handlerMu.RLock()
		defer c.handlerMu.RUnlock()
		for _, h := range c.joinHandlers {
			h(ev)
		}
	})

	c.conn.AddCallback("PART", func(e *irc.Event) {
		channel := e.Arguments[0]
		if e.Nick == c.conn.GetNick() {
			c.mu.Lock()
			delete(c.channels, channel)
			c.mu.Unlock()
		}
		ev := PartEvent{
			Channel:   channel,
			Nick:      e.Nick,
			User:      e.User,
			Host:      e.Host,
			Message:   e.Message(),
			Timestamp: time.Now(),
		}
		c.handlerMu.RLock()
		defer c.handlerMu.RUnlock()
		for _, h := range c.partHandlers {
			h(ev)
		}
	})

	c.conn.AddCallback("KICK", func(e *irc.Event) {
		channel := e.Arguments[0]
		kicked := ""
		if len(e.Arguments) > 1 {
			kicked = e.Arguments[1]
		}
		msg := e.Message()

		if kicked == c.conn.GetNick() {
			c.mu.Lock()
			delete(c.channels, channel)
			c.mu.Unlock()
			slog.Warn("kicked from channel", "channel", channel, "by", e.Nick, "reason", msg)
			if c.cfg.AutoRejoinOnKick {
				slog.Info("auto-rejoining channel", "channel", channel)
				c.conn.Join(channel)
			}
		}

		ev := KickEvent{
			Channel:   channel,
			Nick:      kicked,
			KickedBy:  e.Nick,
			Message:   msg,
			Timestamp: time.Now(),
		}
		c.handlerMu.RLock()
		defer c.handlerMu.RUnlock()
		for _, h := range c.kickHandlers {
			h(ev)
		}
	})

	c.conn.AddCallback("ERROR", func(e *irc.Event) {
		slog.Warn("IRC ERROR", "message", e.Message())
		ev := ErrorEvent{Message: e.Message(), Timestamp: time.Now()}
		c.handlerMu.RLock()
		defer c.handlerMu.RUnlock()
		for _, h := range c.errHandlers {
			h(ev)
		}
	})
}

func (c *ircClient) Connect(ctx context.Context) error {
	slog.Info("connecting to IRC server", "server", c.cfg.Server, "nick", c.cfg.Nick)

	ready := make(chan struct{}, 1)

	// Register a callback for 001 (RPL_WELCOME) to signal readiness and handle reconnects.
	var welcomeOnce sync.Once
	c.conn.AddCallback("001", func(e *irc.Event) {
		// Reset backoff on successful (re)connection.
		if c.backoff != nil {
			c.backoff.Reset()
		}

		ev := ConnectEvent{
			Server:    c.cfg.Server,
			Nick:      c.conn.GetNick(),
			Timestamp: time.Now(),
		}
		c.handlerMu.RLock()
		for _, h := range c.connHandlers {
			h(ev)
		}
		c.handlerMu.RUnlock()

		slog.Info("connected and registered", "server", c.cfg.Server, "nick", c.conn.GetNick())

		// Join configured channels.
		for _, ch := range c.cfg.Channels {
			c.Join(ch)
		}

		welcomeOnce.Do(func() {
			select {
			case ready <- struct{}{}:
			default:
			}
		})
	})

	if err := c.conn.Connect(c.cfg.Server); err != nil {
		return fmt.Errorf("connect to %s: %w", c.cfg.Server, err)
	}

	// Start the event loop in a goroutine with reconnect support.
	go func() {
		for {
			c.conn.Loop()

			ev := DisconnectEvent{Server: c.cfg.Server, Timestamp: time.Now()}
			c.handlerMu.RLock()
			for _, h := range c.discHandlers {
				h(ev)
			}
			c.handlerMu.RUnlock()

			if !c.cfg.Reconnect || c.backoff == nil {
				c.done <- nil
				return
			}

			if c.cfg.MaxReconnectAttempts > 0 && c.backoff.Attempt() >= c.cfg.MaxReconnectAttempts {
				slog.Warn("max reconnect attempts reached", "attempts", c.backoff.Attempt())
				c.done <- nil
				return
			}

			delay := c.backoff.Next()
			slog.Info("reconnecting", "attempt", c.backoff.Attempt(), "delay", delay)
			time.Sleep(delay)

			if err := c.conn.Reconnect(); err != nil {
				slog.Error("reconnect failed", "error", err)
				continue
			}
		}
	}()

	// Wait for 001 or context cancellation.
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		c.conn.Quit()
		return ctx.Err()
	}
}

func (c *ircClient) Disconnect() {
	slog.Info("disconnecting from IRC server")
	c.conn.Quit()
}

func (c *ircClient) Connected() bool {
	return c.conn.Connected()
}

func (c *ircClient) Healthy() bool {
	return c.conn.ValidateConnectionState()
}

func (c *ircClient) Join(channel string) {
	slog.Debug("joining channel", "channel", channel)
	c.conn.Join(channel)
}

func (c *ircClient) Part(channel string) {
	slog.Debug("parting channel", "channel", channel)
	c.conn.Part(channel)
	c.mu.Lock()
	delete(c.channels, channel)
	c.mu.Unlock()
}

func (c *ircClient) JoinedChannels() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	chs := make([]string, 0, len(c.channels))
	for ch := range c.channels {
		chs = append(chs, ch)
	}
	return chs
}

func (c *ircClient) waitRateLimit() {
	if c.limiter != nil {
		_ = c.limiter.Wait(context.Background())
	}
}

func (c *ircClient) SendMessage(target, message string) {
	c.waitRateLimit()
	c.conn.Privmsg(target, message)
}

func (c *ircClient) SendNotice(target, message string) {
	c.waitRateLimit()
	c.conn.Notice(target, message)
}

func (c *ircClient) SendRaw(message string) {
	c.waitRateLimit()
	c.conn.SendRaw(message)
}

func (c *ircClient) Nick() string {
	return c.conn.GetNick()
}

func (c *ircClient) SetNick(nick string) {
	c.conn.Nick(nick)
}

func (c *ircClient) Wait() error {
	return <-c.done
}

// --- Handler registration ---

func (c *ircClient) OnMessage(handler MessageHandler) HandlerID {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	id := c.nextID
	c.nextID++
	c.msgHandlers[id] = handler
	c.handlerKinds[id] = "msg"
	return id
}

func (c *ircClient) OnJoin(handler JoinHandler) HandlerID {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	id := c.nextID
	c.nextID++
	c.joinHandlers[id] = handler
	c.handlerKinds[id] = "join"
	return id
}

func (c *ircClient) OnPart(handler PartHandler) HandlerID {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	id := c.nextID
	c.nextID++
	c.partHandlers[id] = handler
	c.handlerKinds[id] = "part"
	return id
}

func (c *ircClient) OnKick(handler KickHandler) HandlerID {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	id := c.nextID
	c.nextID++
	c.kickHandlers[id] = handler
	c.handlerKinds[id] = "kick"
	return id
}

func (c *ircClient) OnError(handler ErrorHandler) HandlerID {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	id := c.nextID
	c.nextID++
	c.errHandlers[id] = handler
	c.handlerKinds[id] = "error"
	return id
}

func (c *ircClient) OnConnect(handler ConnectHandler) HandlerID {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	id := c.nextID
	c.nextID++
	c.connHandlers[id] = handler
	c.handlerKinds[id] = "connect"
	return id
}

func (c *ircClient) OnDisconnect(handler DisconnectHandler) HandlerID {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	id := c.nextID
	c.nextID++
	c.discHandlers[id] = handler
	c.handlerKinds[id] = "disconnect"
	return id
}

func (c *ircClient) RemoveHandler(id HandlerID) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	kind, ok := c.handlerKinds[id]
	if !ok {
		return
	}
	delete(c.handlerKinds, id)
	switch kind {
	case "msg":
		delete(c.msgHandlers, id)
	case "join":
		delete(c.joinHandlers, id)
	case "part":
		delete(c.partHandlers, id)
	case "kick":
		delete(c.kickHandlers, id)
	case "error":
		delete(c.errHandlers, id)
	case "connect":
		delete(c.connHandlers, id)
	case "disconnect":
		delete(c.discHandlers, id)
	}
}
