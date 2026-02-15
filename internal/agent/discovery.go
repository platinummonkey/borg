package agent

import (
	"strings"
	"sync"
	"time"
)

// AgentCapability describes an agent's advertised capabilities.
type AgentCapability struct {
	Nick        string    `json:"nick"`
	Expertise   []string  `json:"expertise"`
	Channels    []string  `json:"channels"`
	CurrentTask string    `json:"current_task"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Role        string    `json:"role,omitempty"`
	Load        int       `json:"load,omitempty"`
	MaxLoad     int       `json:"max_load,omitempty"`
	ActiveTasks []string  `json:"active_tasks,omitempty"`
}

// DiscoveryStore tracks known agents and their capabilities with TTL-based expiry.
type DiscoveryStore struct {
	mu     sync.RWMutex
	agents map[string]*AgentCapability
	ttl    time.Duration
}

// NewDiscoveryStore creates a DiscoveryStore with the given entry TTL.
func NewDiscoveryStore(ttl time.Duration) *DiscoveryStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &DiscoveryStore{
		agents: make(map[string]*AgentCapability),
		ttl:    ttl,
	}
}

// Update adds or refreshes an agent's capability entry.
func (ds *DiscoveryStore) Update(cap *AgentCapability) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	cp := *cap
	cp.Expertise = append([]string(nil), cap.Expertise...)
	cp.Channels = append([]string(nil), cap.Channels...)
	cp.ActiveTasks = append([]string(nil), cap.ActiveTasks...)
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = time.Now()
	}
	if cp.ExpiresAt.IsZero() {
		cp.ExpiresAt = cp.UpdatedAt.Add(ds.ttl)
	}
	ds.agents[cp.Nick] = &cp
}

// Get returns a copy of the capability for the given nick, or nil if unknown/expired.
func (ds *DiscoveryStore) Get(nick string) *AgentCapability {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	cap := ds.agents[nick]
	if cap == nil || time.Now().After(cap.ExpiresAt) {
		return nil
	}
	cp := *cap
	cp.Expertise = append([]string(nil), cap.Expertise...)
	cp.Channels = append([]string(nil), cap.Channels...)
	return &cp
}

// FindByExpertise returns all non-expired agents with a matching expertise tag.
func (ds *DiscoveryStore) FindByExpertise(tag string) []*AgentCapability {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	now := time.Now()
	var result []*AgentCapability
	tag = strings.ToLower(tag)
	for _, cap := range ds.agents {
		if now.After(cap.ExpiresAt) {
			continue
		}
		for _, e := range cap.Expertise {
			if strings.ToLower(e) == tag {
				cp := *cap
				cp.Expertise = append([]string(nil), cap.Expertise...)
				cp.Channels = append([]string(nil), cap.Channels...)
				result = append(result, &cp)
				break
			}
		}
	}
	return result
}

// ListActive returns all non-expired agent capabilities.
func (ds *DiscoveryStore) ListActive() []*AgentCapability {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	now := time.Now()
	result := make([]*AgentCapability, 0, len(ds.agents))
	for _, cap := range ds.agents {
		if now.After(cap.ExpiresAt) {
			continue
		}
		cp := *cap
		cp.Expertise = append([]string(nil), cap.Expertise...)
		cp.Channels = append([]string(nil), cap.Channels...)
		result = append(result, &cp)
	}
	return result
}

// Prune removes expired entries.
func (ds *DiscoveryStore) Prune() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	now := time.Now()
	for nick, cap := range ds.agents {
		if now.After(cap.ExpiresAt) {
			delete(ds.agents, nick)
		}
	}
}
