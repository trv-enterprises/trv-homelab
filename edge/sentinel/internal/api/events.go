package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/trv-enterprises/trv-homelab/edge/sentinel/internal/model"
)

// Hub fans store events out to SSE clients and to the pusher. It is the
// ingest engine's Sink. Slow clients are dropped rather than allowed to
// block the dispatcher.
type Hub struct {
	mu      sync.Mutex
	clients map[chan model.Event]struct{}
	notify  func(model.Event)
	decor   func(model.Alert) model.Alert
}

func NewHub(notify func(model.Event)) *Hub {
	return &Hub{clients: map[chan model.Event]struct{}{}, notify: notify}
}

// SetDecorator installs the function that attaches signed media links
// before an event is written to a client.
func (h *Hub) SetDecorator(f func(model.Alert) model.Alert) { h.decor = f }

// OnEvent implements ingest.Sink: push first (it is the point of the
// system), then fan out.
func (h *Hub) OnEvent(ev model.Event) {
	if h.notify != nil {
		h.notify(ev)
	}
	h.Broadcast(ev)
}

func (h *Hub) Broadcast(ev model.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- ev:
		default:
			delete(h.clients, c)
			close(c)
		}
	}
}

func (h *Hub) subscribe() chan model.Event {
	c := make(chan model.Event, 16)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) unsubscribe(c chan model.Event) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c)
	}
	h.mu.Unlock()
}

func (h *Hub) Clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// events is GET /v1/events: a text/event-stream of created/updated alerts
// plus a keepalive comment every 25s so proxies do not time it out.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": connected %s\n\n", time.Now().UTC().Format(time.RFC3339))
	fl.Flush()

	c := s.hub.subscribe()
	defer s.hub.unsubscribe(c)
	ka := time.NewTicker(25 * time.Second)
	defer ka.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ka.C:
			fmt.Fprint(w, ": keepalive\n\n")
			fl.Flush()
		case ev, open := <-c:
			if !open {
				return
			}
			ev.Alert = s.decorate(ev.Alert)
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\nid: %s\ndata: %s\n\n", ev.Type, ev.Alert.ID, b)
			fl.Flush()
		}
	}
}
