package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ezra-game/server/pkg/httputil"
)

// Handler serves the SSE stream.
type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

// Stream handles GET /events — a long-lived Server-Sent Events connection that
// pushes realtime events (defense alerts, etc.) to the authenticated player.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	if playerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)

	// http.ResponseController reaches the Flusher through the logging
	// middleware's wrapper (which implements Unwrap). Flush now to commit the
	// SSE headers and confirm the writer supports streaming.
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := h.hub.Subscribe(playerID)
	defer h.hub.Unsubscribe(playerID, ch)

	// Greet so the client can confirm the stream is live.
	writeEvent(w, rc, Event{Type: "connected"})

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			writeEvent(w, rc, ev)
		case <-heartbeat.C:
			// Comment line keeps the connection (and intermediaries) warm.
			fmt.Fprint(w, ": ping\n\n")
			_ = rc.Flush()
		}
	}
}

// SelfTest handles POST /events/test — publishes a sample "tower_under_attack"
// event to the caller's own stream. Dev/QA aid for verifying the SSE pipeline
// end-to-end without waiting for the hourly pressure tick.
func (h *Handler) SelfTest(w http.ResponseWriter, r *http.Request) {
	playerID := httputil.GetPlayerID(r.Context())
	if playerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.hub.Publish(playerID, Event{Type: "tower_under_attack", Data: map[string]any{"attacker": "Заражение"}})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"published":true,"subscribers":%v}`, h.hub.HasSubscribers(playerID))
}

func writeEvent(w http.ResponseWriter, rc *http.ResponseController, ev Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", payload)
	_ = rc.Flush()
}
