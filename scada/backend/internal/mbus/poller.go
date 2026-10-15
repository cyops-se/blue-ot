// Package mbus polls a Modbus TCP device and executes commands (writes). The
// Worker implements the per-device polling; the comm package owns one Worker
// per device and dispatches to it.
package mbus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/simonvetter/modbus"

	"blueot/scada/internal/model"
	"blueot/scada/internal/realtime"
)

const (
	maxRegsPerRead = 120
	maxBitsPerRead = 1968
	maxGap         = 16 // merge reads across gaps up to this many addresses
)

// ---- worker ----

type block struct {
	area  string
	start uint16
	count uint16
	tags  []*model.Tag
}

// Worker polls one Modbus device.
type Worker struct {
	dev     model.Device
	tags    []model.Tag
	blocks  []block
	byID    map[string]*model.Tag
	hub     *realtime.Hub
	trigger chan struct{}
	cancel  context.CancelFunc
	done    chan struct{}

	mu     sync.Mutex // guards client
	client *modbus.ModbusClient
}

// NewWorker builds a Modbus worker for a device and its tags.
func NewWorker(d model.Device, tags []model.Tag, hub *realtime.Hub) *Worker {
	w := &Worker{dev: d, tags: tags, hub: hub, trigger: make(chan struct{}, 1), done: make(chan struct{}), byID: map[string]*model.Tag{}}
	for i := range w.tags {
		w.byID[w.tags[i].ID] = &w.tags[i]
	}
	w.blocks = buildBlocks(w.tags)
	return w
}

// buildBlocks groups tags into as few read requests as possible.
func buildBlocks(tags []model.Tag) []block {
	byArea := map[string][]*model.Tag{}
	for i := range tags {
		t := &tags[i]
		byArea[t.Area] = append(byArea[t.Area], t)
	}
	var out []block
	for _, area := range []string{model.AreaCoil, model.AreaDiscrete, model.AreaInput, model.AreaHolding} {
		list := byArea[area]
		sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
		limit := maxRegsPerRead
		if area == model.AreaCoil || area == model.AreaDiscrete {
			limit = maxBitsPerRead
		}
		var cur *block
		for _, t := range list {
			size := 1
			if !t.IsBitArea() {
				size = t.RegisterCount()
			}
			end := t.Address + size
			if cur != nil && t.Address <= int(cur.start)+int(cur.count)+maxGap && end-int(cur.start) <= limit {
				if c := end - int(cur.start); c > int(cur.count) {
					cur.count = uint16(c)
				}
				cur.tags = append(cur.tags, t)
				continue
			}
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &block{area: area, start: uint16(t.Address), count: uint16(size), tags: []*model.Tag{t}}
		}
		if cur != nil {
			out = append(out, *cur)
		}
	}
	return out
}

// Start begins polling in the background.
func (w *Worker) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.run(ctx)
}

// Stop stops polling and waits for the worker to finish.
func (w *Worker) Stop() {
	w.cancel()
	<-w.done
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	defer w.disconnect()

	period := time.Duration(w.dev.PollMs) * time.Millisecond
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	w.poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.trigger:
		}
		w.poll()
	}
}

func (w *Worker) connectLocked() error {
	if w.client != nil {
		return nil
	}
	c, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:     fmt.Sprintf("tcp://%s:%d", w.dev.Host, w.dev.Port),
		Timeout: time.Duration(w.dev.TimeoutMs) * time.Millisecond,
	})
	if err != nil {
		return err
	}
	if err := c.SetUnitId(uint8(w.dev.UnitID)); err != nil {
		return err
	}
	if err := c.Open(); err != nil {
		return err
	}
	w.client = c
	return nil
}

func (w *Worker) disconnect() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.disconnectLocked()
}

func (w *Worker) disconnectLocked() {
	if w.client != nil {
		_ = w.client.Close()
		w.client = nil
	}
}

// isException reports whether err is a Modbus exception response, i.e. the
// device answered but rejected the request. The connection is still fine.
func isException(err error) bool {
	var me modbus.Error
	if !errors.As(err, &me) {
		return false
	}
	switch me {
	case modbus.ErrIllegalFunction, modbus.ErrIllegalDataAddress, modbus.ErrIllegalDataValue,
		modbus.ErrServerDeviceFailure, modbus.ErrAcknowledge, modbus.ErrServerDeviceBusy,
		modbus.ErrMemoryParityError, modbus.ErrGWPathUnavailable, modbus.ErrGWTargetFailedToRespond:
		return true
	}
	return false
}

func (w *Worker) poll() {
	now := time.Now()
	vals := make([]model.Value, 0, len(w.tags))

	w.mu.Lock()
	connErr := w.connectLocked()
	var blockErr error
	if connErr == nil {
		for _, b := range w.blocks {
			v, err := w.readBlock(b, now)
			if err != nil {
				if !isException(err) {
					connErr = err
					w.disconnectLocked()
					break
				}
				blockErr = fmt.Errorf("%s %d-%d: %w", b.area, b.start, int(b.start)+int(b.count)-1, err)
				for _, t := range b.tags {
					vals = append(vals, model.Value{Tag: t.ID, Quality: model.QualityBad, TS: now})
				}
				continue
			}
			vals = append(vals, v...)
		}
	}
	w.mu.Unlock()

	status := model.DeviceStatus{Device: w.dev.ID, Connected: connErr == nil, LastPoll: now}
	if connErr != nil {
		status.Error = connErr.Error()
		vals = vals[:0]
		for _, t := range w.tags {
			vals = append(vals, model.Value{Tag: t.ID, Quality: model.QualityBad, TS: now})
		}
	} else if blockErr != nil {
		status.Error = blockErr.Error()
	}
	if w.hub.SetDevice(status) {
		if status.Connected {
			w.hub.AddEvent("comm", "info", w.dev.Name, "Kommunikation OK", "")
		} else {
			w.hub.AddEvent("comm", "alarm", w.dev.Name, "Kommunikationsfel: "+status.Error, "")
		}
	}

	changed, prev := w.hub.Update(vals)
	for i, v := range changed {
		t := w.byID[v.Tag]
		if t == nil || !t.LogEvents || v.Quality != model.QualityGood || prev[i].Quality != model.QualityGood {
			continue
		}
		w.hub.AddEvent("state", "info", t.Name, fmt.Sprintf("%s → %s", stateText(t, prev[i].V), stateText(t, v.V)), "")
	}
}

func stateText(t *model.Tag, v float64) string {
	key := strconv.FormatFloat(v, 'f', -1, 64)
	if s, ok := t.StateText[key]; ok {
		return s
	}
	s := strconv.FormatFloat(v, 'f', t.Decimals, 64)
	if t.Unit != "" {
		s += " " + t.Unit
	}
	return s
}

func (w *Worker) readBlock(b block, now time.Time) ([]model.Value, error) {
	out := make([]model.Value, 0, len(b.tags))
	switch b.area {
	case model.AreaCoil, model.AreaDiscrete:
		var bits []bool
		var err error
		if b.area == model.AreaCoil {
			bits, err = w.client.ReadCoils(b.start, b.count)
		} else {
			bits, err = w.client.ReadDiscreteInputs(b.start, b.count)
		}
		if err != nil {
			return nil, err
		}
		for _, t := range b.tags {
			v := 0.0
			if bits[t.Address-int(b.start)] {
				v = 1
			}
			out = append(out, model.Value{Tag: t.ID, V: v, Quality: model.QualityGood, TS: now})
		}
	default:
		rt := modbus.HOLDING_REGISTER
		if b.area == model.AreaInput {
			rt = modbus.INPUT_REGISTER
		}
		regs, err := w.client.ReadRegisters(b.start, b.count, rt)
		if err != nil {
			return nil, err
		}
		for _, t := range b.tags {
			off := t.Address - int(b.start)
			out = append(out, model.Value{Tag: t.ID, V: decode(t, regs[off:off+t.RegisterCount()]), Quality: model.QualityGood, TS: now})
		}
	}
	return out, nil
}

func (w *Worker) Write(t *model.Tag, v float64) error {
	w.mu.Lock()
	err := w.writeLocked(t, v)
	if err != nil && !isException(err) {
		w.disconnectLocked()
	}
	w.mu.Unlock()
	// Read back immediately so the HMI reflects the result quickly.
	select {
	case w.trigger <- struct{}{}:
	default:
	}
	return err
}

func (w *Worker) writeLocked(t *model.Tag, v float64) error {
	if err := w.connectLocked(); err != nil {
		return fmt.Errorf("ingen förbindelse med %s: %w", w.dev.Name, err)
	}
	switch t.Area {
	case model.AreaCoil:
		return w.client.WriteCoil(uint16(t.Address), v != 0)
	case model.AreaHolding:
		regs, err := encode(t, v)
		if err != nil {
			return err
		}
		if len(regs) == 1 {
			return w.client.WriteRegister(uint16(t.Address), regs[0])
		}
		return w.client.WriteRegisters(uint16(t.Address), regs)
	default:
		return fmt.Errorf("området %q är skrivskyddat", t.Area)
	}
}
