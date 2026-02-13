package agent

import (
	"context"
	"sync"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
)

// stubClient implements ircclient.Client for unit tests.
// It reuses the sentMessage type from protocol_test.go.
type stubClient struct {
	mu       sync.Mutex
	messages []sentMessage
	nick     string
}

func (c *stubClient) SendMessage(target, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, sentMessage{target, message})
}

func (c *stubClient) Nick() string { return c.nick }

func (c *stubClient) sentMessages() []sentMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]sentMessage, len(c.messages))
	copy(result, c.messages)
	return result
}

// Unused interface methods.
func (c *stubClient) Connect(_ context.Context) error   { return nil }
func (c *stubClient) Disconnect()                       {}
func (c *stubClient) Connected() bool                   { return true }
func (c *stubClient) Healthy() bool                     { return true }
func (c *stubClient) Join(channel string)               {}
func (c *stubClient) Part(channel string)               {}
func (c *stubClient) JoinedChannels() []string          { return nil }
func (c *stubClient) SendNotice(target, message string) {}
func (c *stubClient) SendRaw(message string)            {}
func (c *stubClient) SetNick(nick string)               {}
func (c *stubClient) OnMessage(h ircclient.MessageHandler) ircclient.HandlerID {
	return 0
}
func (c *stubClient) OnJoin(h ircclient.JoinHandler) ircclient.HandlerID       { return 0 }
func (c *stubClient) OnPart(h ircclient.PartHandler) ircclient.HandlerID       { return 0 }
func (c *stubClient) OnKick(h ircclient.KickHandler) ircclient.HandlerID       { return 0 }
func (c *stubClient) OnError(h ircclient.ErrorHandler) ircclient.HandlerID     { return 0 }
func (c *stubClient) OnConnect(h ircclient.ConnectHandler) ircclient.HandlerID { return 0 }
func (c *stubClient) OnDisconnect(h ircclient.DisconnectHandler) ircclient.HandlerID {
	return 0
}
func (c *stubClient) RemoveHandler(id ircclient.HandlerID) {}
func (c *stubClient) Wait() error                          { return nil }
