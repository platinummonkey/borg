package manager

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// WSMessage is a typed message sent to WebSocket clients.
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Hub manages WebSocket client connections and broadcasts messages.
type Hub struct {
	mu      sync.RWMutex
	clients map[*wsConn]struct{}
}

// wsConn wraps a WebSocket connection for the hub.
type wsConn struct {
	send chan []byte
	done chan struct{}
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*wsConn]struct{}),
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(c *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(c *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		close(c.send)
		delete(h.clients, c)
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("hub: marshal error", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			// Client buffer full — skip.
			slog.Debug("hub: client buffer full, skipping")
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
