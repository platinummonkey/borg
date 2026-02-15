package agent

import (
	"context"
	"testing"
	"time"

	"github.com/platinummonkey/agent-chat/internal/config"
	"github.com/platinummonkey/agent-chat/pkg/ircclient"
)

// federationStubClient extends stubClient with message handler support for federation tests.
type federationStubClient struct {
	stubClient
	handlers map[ircclient.HandlerID]ircclient.MessageHandler
	nextID   ircclient.HandlerID
}

func newFederationStub(nick string) *federationStubClient {
	return &federationStubClient{
		stubClient: stubClient{nick: nick},
		handlers:   make(map[ircclient.HandlerID]ircclient.MessageHandler),
	}
}

func (c *federationStubClient) OnMessage(h ircclient.MessageHandler) ircclient.HandlerID {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	c.handlers[id] = h
	return id
}

func (c *federationStubClient) simulateMessage(ev ircclient.MessageEvent) {
	c.mu.Lock()
	handlers := make([]ircclient.MessageHandler, 0, len(c.handlers))
	for _, h := range c.handlers {
		handlers = append(handlers, h)
	}
	c.mu.Unlock()
	for _, h := range handlers {
		h(ev)
	}
}

func TestFederation_RelayLocalToRemote(t *testing.T) {
	local := newFederationStub("local-agent")
	remote := newFederationStub("remote-agent")

	fm := NewFederationManager(local, local.Nick)
	fm.AddLink("east", remote, []string{"#project"})
	fm.AddMapping(config.ChannelMapping{
		LocalChannel:  "#project",
		RemoteChannel: "#project",
		LinkName:      "east",
	})

	if err := fm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer fm.Shutdown()

	// Simulate a local message.
	local.simulateMessage(ircclient.MessageEvent{
		Channel: "#project",
		Nick:    "human-user",
		Message: "hello from local",
	})

	time.Sleep(50 * time.Millisecond)

	msgs := remote.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 relayed message, got %d", len(msgs))
	}
	if msgs[0].target != "#project" {
		t.Errorf("target = %q, want #project", msgs[0].target)
	}
	if !IsFederatedMessage(msgs[0].message) {
		t.Errorf("message not prefixed: %q", msgs[0].message)
	}
}

func TestFederation_RelayRemoteToLocal(t *testing.T) {
	local := newFederationStub("local-agent")
	remote := newFederationStub("remote-agent")

	fm := NewFederationManager(local, local.Nick)
	fm.AddLink("east", remote, []string{"#project"})
	fm.AddMapping(config.ChannelMapping{
		LocalChannel:  "#project",
		RemoteChannel: "#project",
		LinkName:      "east",
	})

	if err := fm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer fm.Shutdown()

	// Simulate a remote message.
	remote.simulateMessage(ircclient.MessageEvent{
		Channel: "#project",
		Nick:    "remote-human",
		Message: "hello from remote",
	})

	time.Sleep(50 * time.Millisecond)

	msgs := local.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 relayed message, got %d", len(msgs))
	}
	if msgs[0].target != "#project" {
		t.Errorf("target = %q, want #project", msgs[0].target)
	}
	if !IsFederatedMessage(msgs[0].message) {
		t.Errorf("message not prefixed: %q", msgs[0].message)
	}
}

func TestFederation_LoopPrevention(t *testing.T) {
	local := newFederationStub("local-agent")
	remote := newFederationStub("remote-agent")

	fm := NewFederationManager(local, local.Nick)
	fm.AddLink("east", remote, []string{"#project"})
	fm.AddMapping(config.ChannelMapping{
		LocalChannel:  "#project",
		RemoteChannel: "#project",
		LinkName:      "east",
	})

	if err := fm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer fm.Shutdown()

	// Simulate a federated message (already relayed) on local side.
	local.simulateMessage(ircclient.MessageEvent{
		Channel: "#project",
		Nick:    "some-user",
		Message: "[fed:east] <remote-human> hello",
	})

	time.Sleep(50 * time.Millisecond)

	// Remote should NOT receive the already-federated message.
	msgs := remote.sentMessages()
	if len(msgs) != 0 {
		t.Errorf("expected 0 relayed messages (loop prevention), got %d", len(msgs))
	}
}

func TestFederation_MultipleLinks(t *testing.T) {
	local := newFederationStub("local-agent")
	east := newFederationStub("east-agent")
	west := newFederationStub("west-agent")

	fm := NewFederationManager(local, local.Nick)
	fm.AddLink("east", east, []string{"#project"})
	fm.AddLink("west", west, []string{"#ops"})
	fm.AddMapping(config.ChannelMapping{
		LocalChannel:  "#project",
		RemoteChannel: "#project",
		LinkName:      "east",
	})
	fm.AddMapping(config.ChannelMapping{
		LocalChannel:  "#ops",
		RemoteChannel: "#ops",
		LinkName:      "west",
	})

	if err := fm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer fm.Shutdown()

	// Message on #project should only go to east.
	local.simulateMessage(ircclient.MessageEvent{
		Channel: "#project",
		Nick:    "user",
		Message: "project msg",
	})

	time.Sleep(50 * time.Millisecond)

	eastMsgs := east.sentMessages()
	westMsgs := west.sentMessages()

	if len(eastMsgs) != 1 {
		t.Errorf("east: expected 1 message, got %d", len(eastMsgs))
	}
	if len(westMsgs) != 0 {
		t.Errorf("west: expected 0 messages, got %d", len(westMsgs))
	}
}

func TestIsFederatedMessage(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"[fed:local] <nick> hello", true},
		{"[fed:east] <nick> hello", true},
		{"hello world", false},
		{"STARTED task=x", false},
	}
	for _, tt := range tests {
		if got := IsFederatedMessage(tt.msg); got != tt.want {
			t.Errorf("IsFederatedMessage(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}
