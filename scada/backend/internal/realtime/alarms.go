package realtime

import (
	"sort"
	"time"

	"blueot/scada/internal/model"
)

// RaiseAlarm activates an alarm. An alarm that is already active is left as
// it is; one that has returned to normal but is still unacknowledged is
// raised anew.
func (h *Hub) RaiseAlarm(id, kind, source, msg, station, view string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if a := h.alarms[id]; a != nil && a.Active {
		return
	}
	h.alarms[id] = &model.Alarm{ID: id, Kind: kind, Source: source, Station: station, View: view, Message: msg, Active: true, Raised: time.Now()}
	h.addEvent("alarm", "alarm", source, "Larm: "+msg, "")
	h.broadcastAlarms()
}

// ClearAlarm marks an alarm as returned to normal. It leaves the alarm list
// when it is also acknowledged.
func (h *Hub) ClearAlarm(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	a := h.alarms[id]
	if a == nil || !a.Active {
		return
	}
	now := time.Now()
	a.Active, a.Cleared = false, &now
	if a.Acked {
		delete(h.alarms, id)
	}
	h.addEvent("alarm", "info", a.Source, "Larm återgått: "+a.Message, "")
	h.broadcastAlarms()
}

// AckAlarms acknowledges the given alarms, or all alarms when ids is empty,
// and returns how many were acknowledged.
func (h *Hub) AckAlarms(ids []string, user string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	now := time.Now()
	n := 0
	for id, a := range h.alarms {
		if a.Acked || (len(ids) > 0 && !want[id]) {
			continue
		}
		a.Acked, a.AckedAt, a.AckedBy = true, &now, user
		if !a.Active {
			delete(h.alarms, id)
		}
		h.addEvent("alarm", "info", a.Source, "Larm kvitterat: "+a.Message, user)
		n++
	}
	if n > 0 {
		h.broadcastAlarms()
	}
	return n
}

// PruneAlarms silently removes alarms whose id is not in keep, e.g. after
// a tag or device was removed from the configuration.
func (h *Hub) PruneAlarms(keep map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := len(h.alarms)
	for id := range h.alarms {
		if !keep[id] {
			delete(h.alarms, id)
		}
	}
	if len(h.alarms) != n {
		h.broadcastAlarms()
	}
}

// Alarms returns the alarm list, newest first.
func (h *Hub) Alarms() []model.Alarm {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.alarmList()
}

func (h *Hub) alarmList() []model.Alarm {
	out := make([]model.Alarm, 0, len(h.alarms))
	for _, a := range h.alarms {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Raised.After(out[j].Raised) })
	return out
}

// broadcastAlarms sends the whole alarm list; it must be called with h.mu held.
func (h *Hub) broadcastAlarms() {
	h.broadcast(Message{Type: "alarms", Alarms: h.alarmList()})
}
