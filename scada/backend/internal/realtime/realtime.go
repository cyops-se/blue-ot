// Package realtime holds the current process image, a short in-memory
// history per tag, the event list and fans out changes to subscribers
// (WebSocket clients).
package realtime

import (
	"encoding/json"
	"sync"
	"time"

	"blueot/scada/internal/model"
)

const (
	historySize = 3600 // samples per tag (1 h at 1 s polling)
	eventSize   = 1000
)

// Message is pushed to subscribers. It is serialised as-is on the websocket.
type Message struct {
	Type    string               `json:"type"` // values, device, event
	Values  []model.Value        `json:"values,omitempty"`
	Device  *model.DeviceStatus  `json:"device,omitempty"`
	Event   *model.Event         `json:"event,omitempty"`
}

type ring struct {
	buf  []model.Sample
	next int
	full bool
}

func (r *ring) add(s model.Sample) {
	if r.buf == nil {
		r.buf = make([]model.Sample, historySize)
	}
	r.buf[r.next] = s
	r.next = (r.next + 1) % len(r.buf)
	if r.next == 0 {
		r.full = true
	}
}

func (r *ring) since(t int64) []model.Sample {
	out := []model.Sample{}
	if r.buf == nil {
		return out
	}
	start, n := 0, r.next
	if r.full {
		start, n = r.next, len(r.buf)
	}
	for i := 0; i < n; i++ {
		s := r.buf[(start+i)%len(r.buf)]
		if s.TS >= t {
			out = append(out, s)
		}
	}
	return out
}

// Hub is the realtime database.
type Hub struct {
	mu      sync.RWMutex
	values  map[string]model.Value
	history map[string]*ring
	devices map[string]model.DeviceStatus
	events  []model.Event
	eventID int64
	subs    map[chan []byte]struct{}
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{
		values:  map[string]model.Value{},
		history: map[string]*ring{},
		devices: map[string]model.DeviceStatus{},
		subs:    map[chan []byte]struct{}{},
	}
}

// Subscribe registers a subscriber. The channel receives encoded messages;
// slow subscribers drop messages rather than blocking the pollers.
func (h *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes and closes a subscriber channel.
func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// broadcast must be called with h.mu held (read or write).
func (h *Hub) broadcast(m Message) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}

// Update stores a batch of values coming from one poll cycle and broadcasts
// the ones that changed. It returns the changed values together with their
// previous value (for event generation).
func (h *Hub) Update(vals []model.Value) (changed []model.Value, previous []model.Value) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, v := range vals {
		old, existed := h.values[v.Tag]
		if v.Quality == model.QualityGood {
			r := h.history[v.Tag]
			if r == nil {
				r = &ring{}
				h.history[v.Tag] = r
			}
			r.add(model.Sample{V: v.V, TS: v.TS.UnixMilli()})
		}
		if existed && old.V == v.V && old.Quality == v.Quality {
			old.TS = v.TS
			h.values[v.Tag] = old
			continue
		}
		h.values[v.Tag] = v
		changed = append(changed, v)
		if existed {
			previous = append(previous, old)
		} else {
			previous = append(previous, model.Value{Tag: v.Tag, Quality: ""})
		}
	}
	if len(changed) > 0 {
		h.broadcast(Message{Type: "values", Values: changed})
	}
	return changed, previous
}

// Forget removes tags that no longer exist in the configuration.
func (h *Hub) Forget(keep map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := range h.values {
		if !keep[id] {
			delete(h.values, id)
			delete(h.history, id)
		}
	}
}

// Snapshot returns all current values.
func (h *Hub) Snapshot() []model.Value {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]model.Value, 0, len(h.values))
	for _, v := range h.values {
		out = append(out, v)
	}
	return out
}

// Value returns the current value of a tag.
func (h *Hub) Value(tag string) (model.Value, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	v, ok := h.values[tag]
	return v, ok
}

// History returns samples newer than since.
func (h *Hub) History(tag string, since time.Time) []model.Sample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r := h.history[tag]
	if r == nil {
		return []model.Sample{}
	}
	return r.since(since.UnixMilli())
}

// SetDevice updates the communication status of a device.
func (h *Hub) SetDevice(s model.DeviceStatus) (changed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	old, ok := h.devices[s.Device]
	h.devices[s.Device] = s
	changed = !ok || old.Connected != s.Connected || old.Error != s.Error
	if changed {
		h.broadcast(Message{Type: "device", Device: &s})
	}
	return changed
}

// RemoveDevice drops the status of a device that was deleted/disabled.
func (h *Hub) RemoveDevice(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.devices, id)
}

// Devices returns the status of all devices.
func (h *Hub) Devices() []model.DeviceStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]model.DeviceStatus, 0, len(h.devices))
	for _, d := range h.devices {
		out = append(out, d)
	}
	return out
}

// AddEvent appends to the event list and broadcasts it.
func (h *Hub) AddEvent(kind, severity, source, msg, user string) model.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.eventID++
	e := model.Event{ID: h.eventID, TS: time.Now(), Kind: kind, Severity: severity, Source: source, Message: msg, User: user}
	h.events = append(h.events, e)
	if len(h.events) > eventSize {
		h.events = h.events[len(h.events)-eventSize:]
	}
	h.broadcast(Message{Type: "event", Event: &e})
	return e
}

// Events returns the event list, newest first.
func (h *Hub) Events() []model.Event {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]model.Event, len(h.events))
	for i, e := range h.events {
		out[len(h.events)-1-i] = e
	}
	return out
}
