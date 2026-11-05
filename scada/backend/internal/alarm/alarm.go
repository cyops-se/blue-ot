// Package alarm supervises tag values and device communication and keeps
// the alarm list of the realtime hub up to date.
package alarm

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"blueot/scada/internal/model"
	"blueot/scada/internal/realtime"
	"blueot/scada/internal/store"
)

// commandWindow is how long after an HMI command a state change counts as
// caused by it.
const commandWindow = 30 * time.Second

type command struct {
	value float64
	ts    time.Time
}

// Engine raises and clears alarms from value and device status changes.
type Engine struct {
	store *store.Store
	hub   *realtime.Hub

	mu       sync.Mutex
	last     map[string]float64 // last good value per tag
	commands map[string]command // latest command per command tag
}

// New creates an engine and registers it with the hub.
func New(s *store.Store, h *realtime.Hub) *Engine {
	e := &Engine{store: s, hub: h, last: map[string]float64{}, commands: map[string]command{}}
	h.Watch(e)
	return e
}

// Commanded records a command from the HMI, so that the state change it
// causes is not reported as uncommanded.
func (e *Engine) Commanded(tag string, value float64) {
	e.mu.Lock()
	e.commands[tag] = command{value: value, ts: time.Now()}
	e.mu.Unlock()
}

// Cancel forgets a command that could not be sent.
func (e *Engine) Cancel(tag string) {
	e.mu.Lock()
	delete(e.commands, tag)
	e.mu.Unlock()
}

// ValuesChanged implements realtime.Watcher.
func (e *Engine) ValuesChanged(vals []model.Value) {
	for _, v := range vals {
		if v.Quality != model.QualityGood {
			continue
		}
		e.mu.Lock()
		prev, seen := e.last[v.Tag]
		e.last[v.Tag] = v.V
		e.mu.Unlock()

		t, ok := e.store.Tag(v.Tag)
		if !ok || t.Alarm == nil {
			continue
		}
		switch {
		case !slices.Contains(t.Alarm.Values, v.V):
			e.hub.ClearAlarm(t.ID)
		case !t.Alarm.Uncommanded:
			e.raise(t.ID, "state", t.Name, alarmText(&t, t.Text(v.V)))
		case seen && prev != v.V && !e.consume(&t, v.V):
			e.raise(t.ID, "state", t.Name, alarmText(&t, fmt.Sprintf("Ej beordrad lägesändring %s → %s", t.Text(prev), t.Text(v.V))))
		}
	}
}

// DeviceChanged implements realtime.Watcher.
func (e *Engine) DeviceChanged(s model.DeviceStatus) {
	id := model.CommAlarmID(s.Device)
	if s.Connected {
		e.hub.ClearAlarm(id)
		return
	}
	name := s.Device
	for _, d := range e.store.Config().Devices {
		if d.ID == s.Device {
			name = d.Name
		}
	}
	msg := "Kommunikationsfel"
	if s.Error != "" {
		msg += ": " + s.Error
	}
	e.raise(id, "comm", name, msg)
}

// raise looks up the station and a matching view for the alarm object and
// activates the alarm on the hub.
func (e *Engine) raise(id, kind, source, msg string) {
	station, view := e.store.AlarmLocation(id)
	e.hub.RaiseAlarm(id, kind, source, msg, station, view)
}

// consume reports whether a recent command wrote v to the command tag of t,
// and forgets that command.
func (e *Engine) consume(t *model.Tag, v float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, id := range cmdTags(t) {
		c, ok := e.commands[id]
		if ok && c.value == v && time.Since(c.ts) < commandWindow {
			delete(e.commands, id)
			return true
		}
	}
	return false
}

func cmdTags(t *model.Tag) []string {
	if t.Alarm.CmdTag != "" {
		return []string{t.Alarm.CmdTag}
	}
	ids := []string{t.ID, t.ID + ".Cmd", t.ID + "_CMD"}
	if s, ok := strings.CutSuffix(t.ID, "_POS"); ok {
		ids = append(ids, s+"_CMD")
	}
	return ids
}

func alarmText(t *model.Tag, def string) string {
	if t.Alarm.Text != "" {
		return t.Alarm.Text
	}
	return def
}

// Keep returns the ids of all alarms the configuration can raise, for
// realtime.Hub.PruneAlarms.
func Keep(cfg model.Config) map[string]bool {
	keep := map[string]bool{}
	for _, t := range cfg.Tags {
		if t.Alarm != nil {
			keep[t.ID] = true
		}
	}
	for _, d := range cfg.Devices {
		if d.Enabled {
			keep[model.CommAlarmID(d.ID)] = true
		}
	}
	return keep
}
