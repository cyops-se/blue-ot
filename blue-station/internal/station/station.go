// Package station simulates a simple transformer substation that is fed from
// the 220 kV transmission grid (stamnät) and supplies four 20 kV distribution
// feeders through two power transformers and a sectionalised busbar.
//
//	  Stamnät 220 kV
//	        |
//	      [Q0]                 line breaker
//	   -----+-----------       220 kV busbar
//	   |               |
//	[T1.HV]         [T2.HV]
//	  (T1)            (T2)     220/20 kV, OLTC + AVR
//	[T1.LV]         [T2.LV]
//	   |               |
//	 ==A==[Coupler]==B==       20 kV busbar, section A and B
//	  |  |           |  |  |
//	 F1  F2         F3  F4 G1  distribution feeders and generator bay
//
// The generator bay G1 connects the CHP plant simulated by blue-chp. Its
// output is delivered through SetGenerator (see package chplink).
package station

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

const sqrt3 = 1.7320508075688772

var (
	ErrLockout         = errors.New("blockerad av skyddsutlösning – återställ först")
	ErrAuto            = errors.New("lindningskopplaren styrs av automatiken")
	ErrLimit           = errors.New("lindningskopplaren är i ändläge")
	ErrRange           = errors.New("värde utanför tillåtet intervall")
	ErrNotControllable = errors.New("punkten är inte styrbar")
)

// Config holds the electrical parameters of the station.
type Config struct {
	HVNominalKV     float64 // transmission side nominal voltage
	LVNominalKV     float64 // distribution side nominal voltage
	LVNoLoadKV      float64 // secondary no-load voltage at nominal tap and nominal HV voltage
	TrafoMVA        float64 // rating per transformer
	TapMin, TapMax  int
	TapNominal      int
	TapStepPct      float64
	R, X            float64 // transformer short-circuit impedance, pu on own rating
	NoLoadLossMW    float64
	LoadLossMW      float64 // copper losses at rated load
	MagnetizingMvar float64
	OilTauS         float64 // top-oil time constant (shortened to make the demo responsive)
	FaultCurrentA   float64 // feeder short-circuit current
	FaultVoltagePU  float64 // remaining busbar voltage during a feeder fault
}

func DefaultConfig() Config {
	return Config{
		HVNominalKV:     220,
		LVNominalKV:     20,
		LVNoLoadKV:      21,
		TrafoMVA:        40,
		TapMin:          1,
		TapMax:          17,
		TapNominal:      9,
		TapStepPct:      1.25,
		R:               0.005,
		X:               0.12,
		NoLoadLossMW:    0.025,
		LoadLossMW:      0.2,
		MagnetizingMvar: 0.15,
		OilTauS:         600,
		FaultCurrentA:   6000,
		FaultVoltagePU:  0.35,
	}
}

type Breaker struct {
	Closed  bool
	Lockout bool // set by protection trip, cleared by reset
	Ops     uint32
}

type Transformer struct {
	Name    string
	HV, LV  Breaker
	Tap     int
	Auto    bool
	OilC    float64
	P, Q    float64 // HV side
	IHV     float64
	ILV     float64
	Loading float64 // percent of rated current

	Overload, OilAlarm, Trip bool

	bus               int
	energized         bool // HV side live
	feeding           bool // energized and LV breaker closed
	avrTimer, olTimer float64
}

type Feeder struct {
	Name, Desc string
	Bus        int
	CB         Breaker
	BaseMW     float64
	PF         float64
	PickupA    float64 // I> pickup; I>> is 3 x pickup
	Fault      bool    // simulated short circuit on the feeder
	Trip       bool

	P, Q, I   float64
	EnergyMWh float64

	profile   func(hour float64) float64
	energized bool
	demand    float64
	noise     float64
	ocTimer   float64
}

// Generator is the bay where the CHP plant (blue-chp) is connected. The
// plant can only run in parallel with the grid: it trips on loss of mains,
// so the busbar is never energised by the generator alone.
type Generator struct {
	Name, Desc string
	Bus        int
	CB         Breaker

	P, Q, I   float64 // export into the busbar
	EnergyMWh float64

	GenOnline  bool // generator breaker in the plant closed
	LinkOK     bool
	genP, genQ float64
	energized  bool
}

type Event struct {
	Seq  uint64    `json:"seq"`
	Time time.Time `json:"time"`
	Text string    `json:"text"`
}

type Station struct {
	mu        sync.RWMutex
	cfg       Config
	rng       *rand.Rand
	clock     time.Time // simulated time of day, drives the load profiles
	timeScale float64

	gridAvail      bool
	gridDev, hzDev float64
	GridKV, GridHz float64
	GridP, GridQ   float64
	LineI          float64
	Line           Breaker
	T              [2]*Transformer
	Coupler        Breaker
	BusKV          [2]float64
	BusLive        [2]bool
	F              [4]*Feeder
	G1             *Generator

	USet, DeadbandPct, DelayS float64
	LoadScale, AmbientC       float64

	points  []*Point
	byID    map[string]*Point
	vals    []Value
	version uint64
	events  []Event
	evSeq   uint64
	onEvent func(Event)
}

// New creates a station in normal operation: all breakers closed except the
// bus coupler. timeScale speeds up the clock that drives the daily load profiles.
func New(cfg Config, timeScale float64) *Station {
	if timeScale <= 0 {
		timeScale = 1
	}
	s := &Station{
		cfg:         cfg,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
		clock:       time.Now(),
		timeScale:   timeScale,
		gridAvail:   true,
		USet:        20.5,
		DeadbandPct: 1.5,
		DelayS:      10,
		LoadScale:   100,
		AmbientC:    15,
	}
	s.Line.Closed = true
	for i := range s.T {
		t := &Transformer{Name: fmt.Sprintf("T%d", i+1), Tap: cfg.TapNominal, Auto: true, bus: i}
		t.HV.Closed, t.LV.Closed = true, true
		s.T[i] = t
	}
	s.F = [4]*Feeder{
		{Name: "F1", Desc: "Bostadsområde Norr", Bus: 0, BaseMW: 9, PF: 0.97, PickupA: 600, profile: residential},
		{Name: "F2", Desc: "Industriområde Hamnen", Bus: 0, BaseMW: 11, PF: 0.92, PickupA: 600, profile: industrial},
		{Name: "F3", Desc: "Centrum", Bus: 1, BaseMW: 8, PF: 0.95, PickupA: 600, profile: commercial},
		{Name: "F4", Desc: "Landsbygd Väst", Bus: 1, BaseMW: 5, PF: 0.96, PickupA: 400, profile: rural},
	}
	for _, f := range s.F {
		f.CB.Closed = true
	}
	s.G1 = &Generator{Name: "G1", Desc: "Kraftvärmeverk (blue-chp)", Bus: 1}
	s.G1.CB.Closed = true
	s.updateDemand(0)
	s.solve()
	for _, t := range s.T {
		t.OilC = s.oilTarget(t)
	}
	s.buildPoints()
	s.vals = make([]Value, len(s.points))
	s.refresh(false)
	s.logEvent("Simulatorn startad")
	return s
}

// SetEventHook registers a callback invoked (with the station locked) for every event.
func (s *Station) SetEventHook(fn func(Event)) {
	s.mu.Lock()
	s.onEvent = fn
	s.mu.Unlock()
}

func (s *Station) Run(ctx context.Context, tick time.Duration) {
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
func (s *Station) Tick(dt float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateDemand(dt)
	s.solve()
	s.dynamics(dt)
	s.solve()
	s.refresh(true)
}

// Points returns the static point list. Index i corresponds to Snapshot()[i].
func (s *Station) Points() []*Point { return s.points }

func (s *Station) PointByID(id string) *Point { return s.byID[id] }

// Snapshot returns a copy of all point values and a version that changes
// whenever any value changes.
func (s *Station) Snapshot() ([]Value, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Value(nil), s.vals...), s.version
}

// Events returns the event log, oldest first.
func (s *Station) Events() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Event(nil), s.events...)
}

// GridSide returns the state of the generator bay as seen by the plant.
func (s *Station) GridSide() (busLive bool, busKV, hz float64, bayClosed bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.G1
	return s.BusLive[g.Bus], s.BusKV[g.Bus], s.GridHz, g.CB.Closed
}

// SetGenerator stores the latest output reported by the plant. linkOK is
// false when the plant could not be reached; the generator then contributes
// nothing. The values are used from the next simulation step.
func (s *Station) SetGenerator(linkOK, online bool, p, q float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.G1
	g.LinkOK = linkOK
	g.GenOnline = linkOK && online
	g.genP, g.genQ = 0, 0
	if g.GenOnline {
		g.genP, g.genQ = p, q
	}
}

// Control executes a command on a point. src identifies the origin in the event log.
func (s *Station) Control(p *Point, c Ctrl, src string) error {
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
	s.updateDemand(0)
	s.solve()
	s.refresh(true)
	return nil
}

func (s *Station) updateDemand(dt float64) {
	if dt > 0 {
		s.clock = s.clock.Add(time.Duration(dt * s.timeScale * float64(time.Second)))
		sq := math.Sqrt(dt)
		s.gridDev = clamp(s.gridDev-s.gridDev*0.01*dt+s.rng.NormFloat64()*0.002*sq, -0.05, 0.05)
		s.hzDev = clamp(s.hzDev-s.hzDev*0.1*dt+s.rng.NormFloat64()*0.005*sq, -0.2, 0.2)
		for _, f := range s.F {
			f.noise = clamp(f.noise-f.noise*0.02*dt+s.rng.NormFloat64()*0.008*sq, -0.15, 0.15)
		}
	}
	h := float64(s.clock.Hour()) + float64(s.clock.Minute())/60 + float64(s.clock.Second())/3600
	for _, f := range s.F {
		f.demand = f.BaseMW * f.profile(h) * s.LoadScale / 100 * (1 + f.noise)
	}
}

// solve computes topology, voltages and power flows for the current state.
func (s *Station) solve() {
	cfg := s.cfg
	if s.gridAvail {
		s.GridKV = cfg.HVNominalKV * (1 + s.gridDev)
		s.GridHz = 50 + s.hzDev
	} else {
		s.GridKV, s.GridHz = 0, 0
	}
	hvLive := s.gridAvail && s.Line.Closed
	for _, t := range s.T {
		t.energized = hvLive && t.HV.Closed
		t.feeding = t.energized && t.LV.Closed
	}

	groups := [][]int{{0}, {1}}
	if s.Coupler.Closed {
		groups = [][]int{{0, 1}}
	}
	ratedLV := cfg.TrafoMVA / (sqrt3 * cfg.LVNominalKV) * 1000
	s.GridP, s.GridQ, s.LineI = 0, 0, 0

	for _, g := range groups {
		inGroup := func(bus int) bool { return bus == g[0] || bus == g[len(g)-1] }
		var src []*Transformer
		for _, b := range g {
			if s.T[b].feeding {
				src = append(src, s.T[b])
			}
		}
		live := len(src) > 0
		n := float64(len(src))

		var p, q float64
		fault := false
		for _, f := range s.F {
			if !inGroup(f.Bus) {
				continue
			}
			f.energized = live && f.CB.Closed
			if f.energized {
				f.P = f.demand
				f.Q = f.P * math.Tan(math.Acos(f.PF))
				fault = fault || f.Fault
			} else {
				f.P, f.Q = 0, 0
			}
			p += f.P
			q += f.Q
		}
		gen := s.G1
		if inGroup(gen.Bus) {
			gen.energized = live && gen.CB.Closed
			gen.P, gen.Q = 0, 0
			if gen.energized && gen.GenOnline {
				gen.P, gen.Q = gen.genP, gen.genQ
			}
			p -= gen.P // the generator covers part of the load (or exports)
			q -= gen.Q
		}

		var u, ratio float64
		if live {
			for _, t := range src {
				u0 := s.GridKV * cfg.LVNoLoadKV / cfg.HVNominalKV * (1 + float64(t.Tap-cfg.TapNominal)*cfg.TapStepPct/100)
				drop := (cfg.R*p/n + cfg.X*q/n) / cfg.TrafoMVA
				u += u0 * (1 - drop)
				ratio += u0 / s.GridKV
			}
			u /= n
			ratio /= n
		}

		var isum float64
		for _, f := range s.F {
			if !inGroup(f.Bus) {
				continue
			}
			switch {
			case !f.energized:
				f.I = 0
			case f.Fault:
				f.I = cfg.FaultCurrentA
			default:
				f.I = math.Hypot(f.P, f.Q) / (sqrt3 * u) * 1000
			}
			isum += f.I
		}
		if inGroup(s.G1.Bus) {
			s.G1.I = 0
			if s.G1.energized && u > 0 {
				s.G1.I = math.Hypot(s.G1.P, s.G1.Q) / (sqrt3 * u) * 1000
			}
		}
		if fault {
			u *= cfg.FaultVoltagePU
		}
		for _, b := range g {
			s.BusKV[b] = u
			s.BusLive[b] = live
		}

		for _, b := range g {
			t := s.T[b]
			if !t.energized {
				t.P, t.Q, t.IHV, t.ILV, t.Loading = 0, 0, 0, 0, 0
				continue
			}
			var pl, ql, il float64
			if t.feeding {
				pl, ql = p/n, q/n
				il = isum / n
				if !fault && u > 0 {
					// net flow, which may be reversed when the generator exports
					il = math.Hypot(pl, ql) / (sqrt3 * u) * 1000
				}
			}
			t.ILV = il
			t.Loading = il / ratedLV * 100
			k := t.Loading / 100
			t.P = pl + cfg.NoLoadLossMW + cfg.LoadLossMW*k*k
			t.Q = ql + cfg.MagnetizingMvar + cfg.X*cfg.TrafoMVA*k*k
			t.IHV = math.Max(math.Hypot(t.P, t.Q)/(sqrt3*s.GridKV)*1000, il*ratio)
			s.GridP += t.P
			s.GridQ += t.Q
			s.LineI += t.IHV
		}
	}
}

// dynamics advances timers, thermal model, protection and voltage regulation.
func (s *Station) dynamics(dt float64) {
	s.G1.EnergyMWh += s.G1.P * dt / 3600
	for _, f := range s.F {
		f.EnergyMWh += f.P * dt / 3600
		if !f.energized || f.I <= f.PickupA {
			f.ocTimer = 0
			continue
		}
		f.ocTimer += dt
		switch {
		case f.I >= 3*f.PickupA:
			s.tripFeeder(f, fmt.Sprintf("I>> %.0f A", f.I))
		case f.ocTimer >= 2:
			s.tripFeeder(f, fmt.Sprintf("I> %.0f A", f.I))
		}
	}
	for _, t := range s.T {
		t.OilC += (s.oilTarget(t) - t.OilC) * math.Min(1, dt/s.cfg.OilTauS)
		t.Overload = hyst(t.Overload, t.Loading, 100, 97)
		t.OilAlarm = hyst(t.OilAlarm, t.OilC, 90, 87)
		if !t.energized {
			t.olTimer, t.avrTimer = 0, 0
			continue
		}
		if t.OilC > 105 {
			s.tripTrafo(t, fmt.Sprintf("oljetemperatur %.1f °C", t.OilC))
			continue
		}
		if t.Loading > 150 {
			t.olTimer += dt
			if t.olTimer >= 5 {
				s.tripTrafo(t, fmt.Sprintf("överström %.0f %%", t.Loading))
				continue
			}
		} else {
			t.olTimer = 0
		}
		s.avr(t, dt)
	}
}

func (s *Station) avr(t *Transformer, dt float64) {
	u := s.BusKV[t.bus]
	if !t.Auto || !t.feeding || u < 0.7*s.cfg.LVNominalKV {
		t.avrTimer = 0
		return
	}
	dev := (u - s.USet) / s.USet * 100
	if math.Abs(dev) <= s.DeadbandPct {
		t.avrTimer = 0
		return
	}
	t.avrTimer += dt
	if t.avrTimer < s.DelayS {
		return
	}
	t.avrTimer = 0
	if dev > 0 {
		_ = s.stepTap(t, -1)
	} else {
		_ = s.stepTap(t, +1)
	}
}

func (s *Station) oilTarget(t *Transformer) float64 {
	if !t.energized {
		return s.AmbientC
	}
	// IEC 60076-7 style steady-state top-oil rise, 55 K at rated load, loss ratio 6.
	k := t.Loading / 100
	return s.AmbientC + 55*math.Pow((k*k*6+1)/7, 0.8)
}

func (s *Station) stepTap(t *Transformer, d int) error {
	n := t.Tap + d
	if n < s.cfg.TapMin || n > s.cfg.TapMax {
		return ErrLimit
	}
	t.Tap = n
	return nil
}

func (s *Station) manualTap(t *Transformer, d int) error {
	if t.Auto {
		return ErrAuto
	}
	return s.stepTap(t, d)
}

func (s *Station) operate(b *Breaker, close bool) error {
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

func (s *Station) tripFeeder(f *Feeder, reason string) {
	f.CB.Closed, f.CB.Lockout, f.Trip = false, true, true
	f.CB.Ops++
	f.ocTimer = 0
	s.logEvent("SKYDD: %s (%s) utlöst, %s", f.Name, f.Desc, reason)
}

func (s *Station) tripTrafo(t *Transformer, reason string) {
	for _, b := range []*Breaker{&t.HV, &t.LV} {
		if b.Closed {
			b.Closed = false
			b.Ops++
		}
		b.Lockout = true
	}
	t.Trip = true
	t.olTimer = 0
	s.logEvent("SKYDD: %s utlöst, %s", t.Name, reason)
}

func (s *Station) reset() {
	s.Line.Lockout, s.Coupler.Lockout, s.G1.CB.Lockout = false, false, false
	for _, t := range s.T {
		t.HV.Lockout, t.LV.Lockout, t.Trip = false, false, false
	}
	for _, f := range s.F {
		f.CB.Lockout, f.Trip = false, false
	}
}

func (s *Station) commonAlarm() bool {
	if !s.gridAvail {
		return true
	}
	for _, t := range s.T {
		if t.Trip || t.Overload || t.OilAlarm {
			return true
		}
	}
	for _, f := range s.F {
		if f.Trip {
			return true
		}
	}
	return false
}

// refresh re-evaluates all points, timestamps changes and logs binary changes.
func (s *Station) refresh(logChanges bool) {
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

func (s *Station) logEvent(format string, args ...any) {
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

// Daily load profiles, roughly normalised to 1.0 at peak.

func residential(h float64) float64 { return 0.45 + 0.30*bump(h, 7.5, 1.2) + 0.55*bump(h, 18.5, 2.2) }
func industrial(h float64) float64  { return 0.45 + 0.55*window(h, 6, 22, 2) }
func commercial(h float64) float64  { return 0.30 + 0.70*window(h, 8, 19, 1.5) }
func rural(h float64) float64       { return 0.50 + 0.20*bump(h, 7, 1.5) + 0.35*bump(h, 19, 2.5) }

// bump is a gaussian around hour c that wraps around midnight.
func bump(h, c, w float64) float64 {
	d := math.Mod(h-c+36, 24) - 12
	return math.Exp(-d * d / (2 * w * w))
}

func window(h, from, to, k float64) float64 {
	return sigmoid((h-from)*k) * sigmoid((to-h)*k)
}

func sigmoid(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func hyst(state bool, v, on, off float64) bool {
	if state {
		return v > off
	}
	return v > on
}
