package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/platinummonkey/borg/internal/config"
	"github.com/platinummonkey/borg/pkg/ircclient"
)

// FederationLink represents a connection to a remote IRC server.
type FederationLink struct {
	Name     string
	Client   ircclient.Client
	Channels []string
}

// FederationManager relays messages between the local IRC server and remote servers.
// Loop prevention is achieved via a [fed:<origin>] prefix on relayed messages.
type FederationManager struct {
	mu          sync.RWMutex
	links       map[string]*FederationLink
	mappings    []config.ChannelMapping
	localClient ircclient.Client
	localNick   func() string
	handlerIDs  []ircclient.HandlerID
}

// NewFederationManager creates a FederationManager.
func NewFederationManager(localClient ircclient.Client, localNick func() string) *FederationManager {
	return &FederationManager{
		links:       make(map[string]*FederationLink),
		localClient: localClient,
		localNick:   localNick,
	}
}

// AddLink registers a remote server link.
func (fm *FederationManager) AddLink(name string, client ircclient.Client, channels []string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.links[name] = &FederationLink{
		Name:     name,
		Client:   client,
		Channels: channels,
	}
}

// AddMapping adds a channel mapping for relay.
func (fm *FederationManager) AddMapping(m config.ChannelMapping) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.mappings = append(fm.mappings, m)
}

// Start connects all remote links and registers message relay handlers.
func (fm *FederationManager) Start(ctx context.Context) error {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	// Connect remote links.
	for name, link := range fm.links {
		if err := link.Client.Connect(ctx); err != nil {
			return fmt.Errorf("connect federation link %q: %w", name, err)
		}
		slog.Info("federation link connected", "name", name)
	}

	// Register local→remote relay.
	localID := fm.localClient.OnMessage(func(ev ircclient.MessageEvent) {
		if IsFederatedMessage(ev.Message) {
			return
		}
		if ev.Nick == fm.localNick() {
			return
		}
		fm.relayLocalToRemote(ev)
	})
	fm.handlerIDs = append(fm.handlerIDs, localID)

	// Register remote→local relay for each link.
	for linkName, link := range fm.links {
		linkName := linkName
		link := link
		id := link.Client.OnMessage(func(ev ircclient.MessageEvent) {
			if IsFederatedMessage(ev.Message) {
				return
			}
			fm.relayRemoteToLocal(linkName, ev)
		})
		fm.handlerIDs = append(fm.handlerIDs, id)
	}

	return nil
}

// Shutdown disconnects all remote links.
func (fm *FederationManager) Shutdown() {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	for name, link := range fm.links {
		link.Client.Disconnect()
		slog.Info("federation link disconnected", "name", name)
	}
}

// relayLocalToRemote sends a local message to all matching remote channels.
func (fm *FederationManager) relayLocalToRemote(ev ircclient.MessageEvent) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	for _, m := range fm.mappings {
		if m.LocalChannel != ev.Channel {
			continue
		}
		link, ok := fm.links[m.LinkName]
		if !ok {
			continue
		}
		relayed := fmt.Sprintf("[fed:local] <%s> %s", ev.Nick, ev.Message)
		link.Client.SendMessage(m.RemoteChannel, relayed)
		slog.Debug("federated local→remote", "link", m.LinkName, "channel", m.RemoteChannel)
	}
}

// relayRemoteToLocal sends a remote message to all matching local channels.
func (fm *FederationManager) relayRemoteToLocal(linkName string, ev ircclient.MessageEvent) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	for _, m := range fm.mappings {
		if m.LinkName != linkName || m.RemoteChannel != ev.Channel {
			continue
		}
		relayed := fmt.Sprintf("[fed:%s] <%s> %s", linkName, ev.Nick, ev.Message)
		fm.localClient.SendMessage(m.LocalChannel, relayed)
		slog.Debug("federated remote→local", "link", linkName, "channel", m.LocalChannel)
	}
}

// IsFederatedMessage returns true if the message has a [fed:...] prefix.
func IsFederatedMessage(msg string) bool {
	return strings.HasPrefix(msg, "[fed:")
}
