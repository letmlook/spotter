package registryd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Event is broadcast over the WebSocket hub when the device list
// changes. Clients subscribe with ?since=<ts> to replay missed
// events, but v0.5 only delivers live updates (last-N replay is a
// v0.5.1 add).
type Event struct {
	Type      string    `json:"type"` // "device-added" | "device-updated" | "device-removed" | "heartbeat-lost"
	At        time.Time `json:"at"`
	DeviceID  string    `json:"device_id"`
}

// Hub is a many-publishers, many-subscribers fan-out for the
// Event stream. Subscribe returns a per-connection channel; the
// hub drops slow consumers (best-effort) rather than blocking
// publishers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

func (h *Hub) Publish(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// slow consumer; drop.
		}
	}
}

func (h *Hub) Subscribe(ctx context.Context) <-chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	go func() {
		<-ctx.Done()
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}()
	return ch
}

// Handler binds the REST + WebSocket routes to a Store+Hub.
type Handler struct {
	Store *Store
	Hub   *Hub
}

func NewHandler(s *Store, h *Hub) *Handler { return &Handler{Store: s, Hub: h} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/healthz" && r.Method == http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	case r.URL.Path == "/ws/events" && r.Method == http.MethodGet:
		h.serveWS(w, r)
	case r.URL.Path == "/api/v1/devices" && r.Method == http.MethodGet:
		h.listDevices(w, r)
	case r.URL.Path == "/api/v1/devices" && r.Method == http.MethodPost:
		h.registerDevice(w, r)
	case len(r.URL.Path) > len("/api/v1/devices/") && r.URL.Path[:len("/api/v1/devices/")] == "/api/v1/devices/":
		rest := r.URL.Path[len("/api/v1/devices/"):]
		switch {
		case r.Method == http.MethodGet:
			h.getDevice(w, r, rest)
		case r.Method == http.MethodDelete:
			h.deleteDevice(w, r, rest)
		case r.Method == http.MethodPost && len(rest) > len("heartbeat") && rest[len(rest)-len("heartbeat"):] == "heartbeat":
			h.recordHeartbeat(w, r, rest[:len(rest)-len("heartbeat")-1])
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) registerDevice(w http.ResponseWriter, r *http.Request) {
	var d Device
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if d.DeviceID == "" || d.IP == "" {
		http.Error(w, "device_id and ip required", http.StatusBadRequest)
		return
	}
	if d.Port == 0 {
		d.Port = 9999
	}
	d.LastSource = "agent-registered"
	if err := h.Store.Upsert(d); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Hub.Publish(Event{Type: "device-added", At: time.Now().UTC(), DeviceID: d.DeviceID})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(d)
}

func (h *Handler) recordHeartbeat(w http.ResponseWriter, _ *http.Request, id string) {
	existing, err := h.Store.Get(id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "device not registered", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	existing.Online = true
	existing.LastSeenAt = time.Now().UTC()
	existing.LastSource = "heartbeat"
	if err := h.Store.Upsert(*existing); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listDevices(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.Store.List())
}

func (h *Handler) getDevice(w http.ResponseWriter, _ *http.Request, id string) {
	d, err := h.Store.Get(id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

func (h *Handler) deleteDevice(w http.ResponseWriter, _ *http.Request, id string) {
	if err := h.Store.Delete(id); err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Hub.Publish(Event{Type: "device-removed", At: time.Now().UTC(), DeviceID: id})
	w.WriteHeader(http.StatusNoContent)
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer conn.Close()
	ch := h.Hub.Subscribe(ctx)
	for ev := range ch {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}
}
