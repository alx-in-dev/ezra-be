// Package realtime is an in-memory pub/sub hub that pushes server events to
// connected clients over SSE (Server-Sent Events). It powers the defense race
// (B6): "beacon under attack" / "network severed" reach the owner instantly
// instead of waiting for the next poll. Single-instance scope; a Redis-backed
// fan-out can replace the in-memory map when the server scales horizontally.
package realtime

import (
	"context"
	"sync"
)

// Event is a single server→client message. Type is the discriminator the
// client switches on; Data carries an arbitrary payload.
type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

// Hub fans events out to per-player subscriber channels.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan Event]struct{})}
}

// Subscribe registers a new buffered channel for playerID. Caller must call
// Unsubscribe (typically via defer) when the connection ends.
func (h *Hub) Subscribe(playerID string) chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[playerID] == nil {
		h.subs[playerID] = make(map[chan Event]struct{})
	}
	h.subs[playerID][ch] = struct{}{}
	return ch
}

func (h *Hub) Unsubscribe(playerID string, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[playerID]; ok {
		if _, ok := set[ch]; ok {
			delete(set, ch)
			close(ch)
		}
		if len(set) == 0 {
			delete(h.subs, playerID)
		}
	}
}

// Publish delivers an event to every channel subscribed for playerID.
// Non-blocking: a full subscriber buffer drops the event rather than stalling
// the publisher (defense alerts are idempotent — the next poll/tick recovers).
func (h *Hub) Publish(playerID string, ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[playerID] {
		select {
		case ch <- ev:
		default:
		}
	}
}

// PublishEvent is a convenience wrapper around Publish for callers that don't
// want to import the Event type — they pass a type string and a data map.
// Implements the small publisher interface other packages depend on (R4-6).
func (h *Hub) PublishEvent(playerID, eventType string, data map[string]any) {
	h.Publish(playerID, Event{Type: eventType, Data: data})
}

// HasSubscribers reports whether anyone is currently listening for playerID.
func (h *Hub) HasSubscribers(playerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[playerID]) > 0
}

// PushFanout adapts the Hub to the tower service's push interface, mirroring
// each Firebase notification onto the realtime channel. The embedded Inner is
// the original push service (Firebase) so existing behaviour is preserved.
type PushFanout struct {
	Inner interface {
		NotifyTowerPlaced(ctx context.Context, playerID string) error
		NotifyTowerUnderAttack(ctx context.Context, defenderID, attackerName string) error
	}
	Hub *Hub
}

func (f *PushFanout) NotifyTowerPlaced(ctx context.Context, playerID string) error {
	if f.Inner != nil {
		_ = f.Inner.NotifyTowerPlaced(ctx, playerID)
	}
	f.Hub.Publish(playerID, Event{Type: "tower_placed"})
	return nil
}

func (f *PushFanout) NotifyTowerUnderAttack(ctx context.Context, defenderID, attackerName string) error {
	if f.Inner != nil {
		_ = f.Inner.NotifyTowerUnderAttack(ctx, defenderID, attackerName)
	}
	f.Hub.Publish(defenderID, Event{Type: "tower_under_attack", Data: map[string]any{"attacker": attackerName}})
	return nil
}
