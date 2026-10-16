package plant

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	ErrLockout         = errors.New("blockerad av utlösning – återställ först")
	ErrRange           = errors.New("värde utanför tillåtet intervall")
	ErrNotControllable = errors.New("punkten är inte styrbar")
	ErrNoPower         = errors.New("ingen elmatning")
)

// Breaker is a circuit breaker; Lockout is set by protection and cleared by reset.
type Breaker struct {
	Closed  bool
	Lockout bool
	Ops     uint32
}

func (b *Breaker) operate(close bool) error {
	if close == b.Closed {
		return nil
	}
	if close && b.Lockout {
		return ErrLockout
	}
	b.Closed = close
	b.Ops++
	return nil
}

type Event struct {
	Seq  uint64    `json:"seq"`
	Time time.Time `json:"time"`
	Text string    `json:"text"`
}

// core holds what every plant model needs: the simulated clock, the point
// list with its current values and the event log.
type core struct {
	mu        sync.RWMutex
	rng       *rand.Rand
	clock     time.Time
	timeScale float64

	points  []*Point
	byID    map[string]*Point
	vals    []Value
	version uint64
	events  []Event
	evSeq   uint64
	onEvent func(Event)
}

func newCore(timeScale float64) core {
	if timeScale <= 0 {
		timeScale = 1
	}
	return core{rng: rand.New(rand.NewSource(time.Now().UnixNano())), clock: time.Now(), timeScale: timeScale}
}

// hour returns the simulated time of day in hours.
func (c *core) hour() float64 {
	return float64(c.clock.Hour()) + float64(c.clock.Minute())/60 + float64(c.clock.Second())/3600
}

// advance moves the simulated clock forward.
func (c *core) advance(dt float64) {
	c.clock = c.clock.Add(time.Duration(dt * c.timeScale * float64(time.Second)))
}

// SetEventHook registers a callback invoked (with the plant locked) for every event.
func (s *Plant) SetEventHook(fn func(Event)) {
	s.mu.Lock()
	s.onEvent = fn
	s.mu.Unlock()
}

func (s *Plant) Run(ctx context.Context, tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.Tick(now.Sub(last).Seconds())
			last = now
		}
	}
}

// Tick advances the simulation dt seconds.
func (s *Plant) Tick(dt float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.step(dt)
	s.refresh(true)
}

func (s *Plant) Points() []*Point { return s.points }

func (s *Plant) PointByID(id string) *Point { return s.byID[id] }

// Snapshot returns a copy of all point values and a version that changes
// whenever any value changes.
func (s *Plant) Snapshot() ([]Value, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Value(nil), s.vals...), s.version
}

// Events returns the event log, oldest first.
func (s *Plant) Events() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Event(nil), s.events...)
}

// Control executes a command on a point. src identifies the origin in the event log.
func (s *Plant) Control(p *Point, c Ctrl, src string) error {
	if p == nil || p.ctrl == nil {
		return ErrNotControllable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := p.ctrl(c); err != nil {
		s.logEvent("[%s] %s %s AVVISAT: %v", src, p.ID, p.ctrlText(c), err)
		return err
	}
	s.logEvent("[%s] %s %s", src, p.ID, p.ctrlText(c))
	s.step(0)
	s.refresh(true)
	return nil
}

// refresh re-evaluates all points, timestamps changes and logs binary changes.
func (s *Plant) refresh(logChanges bool) {
	now := time.Now()
	changed := false
	for i, p := range s.points {
		v, old := p.get(), s.vals[i]
		if v.B == old.B && v.DP == old.DP && v.F == old.F && !old.T.IsZero() {
			v.T = old.T
		} else {
			v.T = now
			changed = true
			if logChanges && !old.T.IsZero() && p.Kind != KindMeasurement && p.Kind != KindSetpoint && p.Kind != KindPulse {
				s.logEvent("%s → %s", p.ID, p.StateText(v))
			}
		}
		s.vals[i] = v
	}
	if changed {
		s.version++
	}
}

func (s *Plant) logEvent(format string, args ...any) {
	s.evSeq++
	e := Event{Seq: s.evSeq, Time: time.Now(), Text: fmt.Sprintf(format, args...)}
	s.events = append(s.events, e)
	if len(s.events) > 200 {
		s.events = s.events[len(s.events)-200:]
	}
	if s.onEvent != nil {
		s.onEvent(e)
	}
}

// finish builds the point list and takes the first snapshot; called last in New.
func (s *Plant) finish() {
	s.buildPoints()
	s.vals = make([]Value, len(s.points))
	s.refresh(false)
	s.logEvent("Simulatorn startad")
}

// bump is a gaussian around hour c that wraps around midnight.
func bump(h, c, w float64) float64 {
	d := math.Mod(h-c+36, 24) - 12
	return math.Exp(-d * d / (2 * w * w))
}

func approach(v, target, step float64) float64 {
	if v < target {
		return math.Min(target, v+step)
	}
	return math.Max(target, v-step)
}

// lag moves v towards target with time constant tau (seconds).
func lag(v, target, dt, tau float64) float64 {
	if dt <= 0 || tau <= 0 {
		return v
	}
	return v + (target-v)*math.Min(1, dt/tau)
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func hyst(state bool, v, on, off float64) bool {
	if state {
		return v > off
	}
	return v > on
}
