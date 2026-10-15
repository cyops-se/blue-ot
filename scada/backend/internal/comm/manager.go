// Package comm owns one polling worker per enabled device and dispatches
// commands to it. Each device is polled over Modbus TCP or IEC 60870-5-104
// depending on its configured protocol.
package comm

import (
	"context"
	"fmt"
	"log"
	"sync"

	"blueot/scada/internal/iec"
	"blueot/scada/internal/mbus"
	"blueot/scada/internal/model"
	"blueot/scada/internal/realtime"
	"blueot/scada/internal/store"
)

// Worker is a per-device poller (Modbus or IEC 104).
type Worker interface {
	Start(ctx context.Context)
	Stop()
	Write(t *model.Tag, value float64) error
}

// Manager owns the workers.
type Manager struct {
	store *store.Store
	hub   *realtime.Hub

	mu      sync.Mutex
	ctx     context.Context
	workers map[string]Worker
}

// NewManager creates a manager; call Start to begin polling.
func NewManager(s *store.Store, h *realtime.Hub) *Manager {
	return &Manager{store: s, hub: h, workers: map[string]Worker{}}
}

// Start begins polling all enabled devices.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	m.Reload()
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, w := range m.workers {
			w.Stop()
		}
	}()
}

// Reload restarts the workers after a configuration change.
func (m *Manager) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.workers {
		w.Stop()
		m.hub.RemoveDevice(id)
	}
	m.workers = map[string]Worker{}

	cfg := m.store.Config()
	keep := map[string]bool{}
	for _, t := range cfg.Tags {
		keep[t.ID] = true
	}
	m.hub.Forget(keep)

	var modbus, ieccount int
	for _, d := range cfg.Devices {
		if !d.Enabled {
			continue
		}
		var tags []model.Tag
		for _, t := range cfg.Tags {
			if t.Device == d.ID {
				tags = append(tags, t)
			}
		}
		var w Worker
		if d.IsIEC() {
			w = iec.NewWorker(d, tags, m.hub)
			ieccount++
		} else {
			w = mbus.NewWorker(d, tags, m.hub)
			modbus++
		}
		m.workers[d.ID] = w
		w.Start(m.ctx)
	}
	log.Printf("komm: %d Modbus-enheter, %d IEC 104-enheter, %d taggar", modbus, ieccount, len(cfg.Tags))
}

// Write executes a command by writing value to a writable tag.
func (m *Manager) Write(req model.CommandRequest) error {
	t, ok := m.store.Tag(req.Tag)
	if !ok {
		return fmt.Errorf("okänd tagg %q", req.Tag)
	}
	if !t.Writable {
		return fmt.Errorf("taggen %q är inte skrivbar", t.ID)
	}
	m.mu.Lock()
	w := m.workers[t.Device]
	m.mu.Unlock()
	if w == nil {
		return fmt.Errorf("enheten %q är inte aktiv", t.Device)
	}
	return w.Write(&t, req.Value)
}
