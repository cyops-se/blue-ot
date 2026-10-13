// Package plant simulates a combined heat and power plant (kraftvärmeverk)
// with two boilers, a back-pressure steam turbine with generator and a
// district heating network with a heat accumulator.
//
//	[P1 flis]   [P2 bioolja]          boilers
//	    |            |
//	====+============+====  ångsamlare (steam header, pressure controlled)
//	    |                 \
//	[turbin]--(G)--[Gen.CB]--> 20 kV till blue-station (fack G1)
//	    |                   \
//	kondensor  <------------ bypass (reduction valve)
//	    |
//	fjärrvärmenät <--> värmeackumulator
//
// The generator is connected to the 20 kV busbar of blue-station. The grid
// state on the connection point is delivered through Link(); without a link
// the plant assumes a stiff grid (fristående drift).
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

const sqrt3 = 1.7320508075688772

var (
	ErrLockout         = errors.New("blockerad av utlösning – återställ först")
	ErrRange           = errors.New("värde utanför tillåtet intervall")
	ErrNotControllable = errors.New("punkten är inte styrbar")
	ErrSync            = errors.New("synkronisering ej möjlig")
	ErrTurbine         = errors.New("turbinen kan inte startas")
)

// Config holds the process parameters of the plant.
type Config struct {
	BoilerFuelMW   [2]float64 // rated fuel power
	BoilerEff      [2]float64 // fuel → steam efficiency
	FuelLHV        [2]float64 // MWh per tonne
	BoilerMinPct   float64    // minimum stable load
	BoilerRampPct  float64    // load ramp rate, % per second
	BoilerStartS   float64    // ignition to minimum load
	SteamTauS      float64    // fuel → steam time constant
	SteamMWhPerT   float64    // enthalpy rise feedwater → live steam
	HeaderCap      float64    // steam header storage, MW·s per bar
	HeaderTripBar  float64    // boilers trip above this pressure
	HeaderAlarmBar float64
	TurbineMaxMW   float64 // max steam (thermal) through the turbine
	TurbineAlpha   float64 // share of turbine steam heat converted to electricity
	TurbineLossMW  float64 // mechanical and generator losses
	RunUpS         float64 // turbine start to rated speed
	RatedRPM       float64
	BypassMaxMW    float64
	NetKV          float64 // nominal voltage at the connection point
	PumpMaxKgS     float64 // per district heating pump
	ReturnDesignC  float64
	NetCap         float64 // district heating water, MW·s per K
	DPSetBar       float64 // differential pressure at the critical customer
	AccCapMWh      float64
	AccMaxMW       float64
	LinkTimeout    time.Duration
}

func DefaultConfig() Config {
	return Config{
		BoilerFuelMW:   [2]float64{120, 50},
		BoilerEff:      [2]float64{0.88, 0.90},
		FuelLHV:        [2]float64{2.8, 10.5},
		BoilerMinPct:   35,
		BoilerRampPct:  0.5,
		BoilerStartS:   120,
		SteamTauS:      45,
		SteamMWhPerT:   0.78,
		HeaderCap:      20,
		HeaderTripBar:  102,
		HeaderAlarmBar: 97,
		TurbineMaxMW:   115,
		TurbineAlpha:   0.30,
		TurbineLossMW:  0.5,
		RunUpS:         60,
		RatedRPM:       3000,
		BypassMaxMW:    150,
		NetKV:          20.5,
		PumpMaxKgS:     550,
		ReturnDesignC:  47,
		NetCap:         60,
		DPSetBar:       1.5,
		AccCapMWh:      400,
		AccMaxMW:       60,
		LinkTimeout:    5 * time.Second,
	}
}

// Breaker is a circuit breaker; Lockout is set by protection and cleared by reset.
type Breaker struct {
	Closed  bool
	Lockout bool
	Ops     uint32
}

type BoilerState int

const (
	BoilerOff BoilerState = iota
	BoilerStarting
	BoilerRunning
	BoilerStopping
)

var boilerStateText = [...]string{"STOPP", "START", "DRIFT", "STOPPAR"}

type Boiler struct {
	Name, Fuel string
	Run        bool    // operator start/stop command
	Auto       bool    // load follows the plant master controller
	LoadSetPct float64 // manual load setpoint
	FaultSim   bool
	Trip       bool

	State     BoilerState
	LoadPct   float64
	FuelMW    float64
	SteamMW   float64
	SteamC    float64
	O2        float64
	FlueC     float64
	targetPct float64
	startT    float64
	idx       int
}

func (b *Boiler) firing() bool { return b.State != BoilerOff }

type Plant struct {
	mu        sync.RWMutex
	cfg       Config
	rng       *rand.Rand
	clock     time.Time
	timeScale float64

	B [2]*Boiler

	PressBar, PressSetBar float64
	BypassAuto            bool
	BypassManPct          float64
	BypassMW              float64
	SafetyMW              float64

	TurbineRun   bool
	TurbineTrip  bool
	SpeedRPM     float64
	TurbineMW    float64 // steam heat into the turbine
	GenCB        Breaker
	GenTrip      bool
	GenP, GenQ   float64
	GenKV, GenHz float64
	GenI         float64
	GenMWh       float64
	GenQSet      float64

	// connection point (from blue-station)
	linkSeen       time.Time
	LinkOK         bool
	NetLive        bool
	NetKV, NetHz   float64
	linkLive       bool
	linkKV, linkHz float64
	linkBayClosed  bool

	OutdoorC   float64
	HeatScale  float64
	heatNoise  float64
	Demand     float64 // heat demand of the network incl. losses
	Delivered  float64
	Production float64 // heat from condenser and bypass
	SupplyC    float64
	SupplySetC float64
	ReturnC    float64
	FlowKgS    float64
	DPBar      float64
	Pump       [2]bool
	AccMWh     float64
	AccMW      float64 // + charging
	AccSOCSet  float64

	points  []*Point
	byID    map[string]*Point
	vals    []Value
	version uint64
	events  []Event
	evSeq   uint64
	onEvent func(Event)
}

type Event struct {
	Seq  uint64    `json:"seq"`
	Time time.Time `json:"time"`
	Text string    `json:"text"`
}

// New creates a plant in normal winter operation: P1 in automatic load
// control, P2 on standby, turbine synchronised, both pumps running.
func New(cfg Config, timeScale float64) *Plant {
	if timeScale <= 0 {
		timeScale = 1
	}
	s := &Plant{
		cfg:         cfg,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
		clock:       time.Now(),
		timeScale:   timeScale,
		PressSetBar: 90,
		BypassAuto:  true,
		TurbineRun:  true,
		SpeedRPM:    cfg.RatedRPM,
		GenQSet:     4,
		OutdoorC:    2,
		HeatScale:   100,
		Pump:        [2]bool{true, true},
		AccSOCSet:   50,
	}
	s.AccMWh = cfg.AccCapMWh * 0.5
	s.B = [2]*Boiler{
		{Name: "P1", Fuel: "biobränsle (flis)", Run: true, Auto: true, LoadSetPct: 70, State: BoilerRunning, idx: 0},
		{Name: "P2", Fuel: "bioolja", Auto: true, LoadSetPct: 60, idx: 1},
	}
	s.GenCB.Closed = true
	s.PressBar = s.PressSetBar
	s.linkStatus(time.Now())

	// Start in a steady state.
	s.updateDemand(0)
	s.SupplySetC = s.supplyCurve()
	s.SupplyC = s.SupplySetC
	s.master()
	for _, b := range s.B {
		if b.firing() {
			b.LoadPct = b.targetPct
			b.FuelMW = b.LoadPct / 100 * cfg.BoilerFuelMW[b.idx]
			b.SteamMW = b.FuelMW * cfg.BoilerEff[b.idx]
		}
		s.boilerFlue(b, 1e6)
	}
	s.steam(0)
	s.electrical(0)
	s.heat(0)

	s.buildPoints()
	s.vals = make([]Value, len(s.points))
	s.refresh(false)
	s.logEvent("Simulatorn startad")
	return s
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
	s.linkStatus(time.Now())
	s.updateDemand(dt)
	s.master()
	for _, b := range s.B {
		s.boiler(b, dt)
	}
	s.steam(dt)
	s.turbine(dt)
	s.electrical(dt)
	s.heat(dt)
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
	s.electrical(0)
	s.refresh(true)
	return nil
}

// ---- grid connection (link to blue-station) ----

// LinkIn is the state of the connection point as seen from the substation.
type LinkIn struct {
	BusLive   bool    `json:"busLive"`
	BusKV     float64 `json:"busKV"`
	Hz        float64 `json:"hz"`
	BayClosed bool    `json:"bayClosed"`
}

// LinkOut is the generator output fed into the substation.
type LinkOut struct {
	GenOnline bool    `json:"genOnline"`
	P         float64 `json:"p"` // MW, positive = export
	Q         float64 `json:"q"` // Mvar
}

// Link exchanges the connection point state with blue-station.
func (s *Plant) Link(in LinkIn) LinkOut {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.linkSeen = time.Now()
	s.linkLive = in.BusLive && in.BayClosed
	s.linkKV, s.linkHz, s.linkBayClosed = in.BusKV, in.Hz, in.BayClosed
	s.linkStatus(s.linkSeen)
	s.electrical(0) // loss of mains acts immediately
	s.refresh(true)
	return LinkOut{GenOnline: s.GenCB.Closed, P: s.GenP, Q: s.GenQ}
}

func (s *Plant) linkStatus(now time.Time) {
	ok := !s.linkSeen.IsZero() && now.Sub(s.linkSeen) < s.cfg.LinkTimeout
	if ok != s.LinkOK && !s.linkSeen.IsZero() {
		if ok {
			s.logEvent("Länk till blue-station upprättad")
		} else {
			s.logEvent("Länk till blue-station bruten – fristående drift mot stelt nät")
		}
	}
	s.LinkOK = ok
	if ok {
		s.NetLive = s.linkLive
		s.NetKV, s.NetHz = 0, 0
		if s.NetLive {
			s.NetKV, s.NetHz = s.linkKV, s.linkHz
		}
	} else {
		s.NetLive, s.NetKV, s.NetHz = true, s.cfg.NetKV, 50
	}
}

// ---- heat demand and plant master ----

func (s *Plant) updateDemand(dt float64) {
	if dt > 0 {
		s.clock = s.clock.Add(time.Duration(dt * s.timeScale * float64(time.Second)))
		s.heatNoise = clamp(s.heatNoise-s.heatNoise*0.02*dt+s.rng.NormFloat64()*0.004*math.Sqrt(dt), -0.06, 0.06)
	}
	h := float64(s.clock.Hour()) + float64(s.clock.Minute())/60 + float64(s.clock.Second())/3600
	base := 15 + 3.2*math.Max(0, 17-s.OutdoorC) // MW incl. network losses
	profile := 0.92 + 0.12*bump(h, 7, 1.5) + 0.07*bump(h, 19, 2.5) - 0.05*bump(h, 3, 2)
	s.Demand = base * profile * s.HeatScale / 100 * (1 + s.heatNoise)
}

// supplyCurve is the supply temperature setpoint as a function of outdoor temperature.
func (s *Plant) supplyCurve() float64 {
	return clamp(75+(5-s.OutdoorC)*1.25, 75, 110)
}

// master computes the load targets of boilers in automatic mode so that heat
// production follows demand and the accumulator returns to its setpoint.
func (s *Plant) master() {
	cfg := s.cfg
	heat := s.Demand + 1.0*(s.AccSOCSet-s.soc())
	steam := heat
	if s.GenCB.Closed {
		steam = (heat + cfg.TurbineLossMW) / (1 - cfg.TurbineAlpha)
	}
	var autoCap float64
	for _, b := range s.B {
		if !b.Auto {
			b.targetPct = b.LoadSetPct
			if b.State == BoilerRunning {
				steam -= b.SteamMW
			}
			continue
		}
		if b.State == BoilerRunning {
			autoCap += cfg.BoilerFuelMW[b.idx] * cfg.BoilerEff[b.idx]
		}
	}
	for _, b := range s.B {
		if b.Auto {
			pct := 100.0
			if autoCap > 0 {
				pct = steam / autoCap * 100
			}
			b.targetPct = clamp(pct, cfg.BoilerMinPct, 100)
		}
	}
}

func (s *Plant) soc() float64 { return s.AccMWh / s.cfg.AccCapMWh * 100 }

// ---- boilers ----

func (s *Plant) boiler(b *Boiler, dt float64) {
	cfg := s.cfg
	if b.FaultSim && b.firing() {
		reason := "bränslematningsfel"
		if b.idx == 1 {
			reason = "brännarfel"
		}
		s.tripBoiler(b, reason)
	}
	switch b.State {
	case BoilerOff:
		b.LoadPct = 0
		if b.Run && !b.Trip {
			b.State, b.startT = BoilerStarting, 0
			s.logEvent("%s: tändning", b.Name)
		}
	case BoilerStarting:
		b.startT += dt
		b.LoadPct = cfg.BoilerMinPct * math.Min(1, b.startT/cfg.BoilerStartS)
		if !b.Run {
			b.State = BoilerStopping
		} else if b.startT >= cfg.BoilerStartS {
			b.State = BoilerRunning
			s.logEvent("%s: i drift", b.Name)
		}
	case BoilerRunning:
		if !b.Run {
			b.State = BoilerStopping
			break
		}
		b.LoadPct = approach(b.LoadPct, clamp(b.targetPct, cfg.BoilerMinPct, 100), cfg.BoilerRampPct*dt)
	case BoilerStopping:
		b.LoadPct = approach(b.LoadPct, 0, 2*cfg.BoilerRampPct*dt)
		if b.Run && !b.Trip {
			b.State = BoilerStarting
			b.startT = b.LoadPct / cfg.BoilerMinPct * cfg.BoilerStartS
		} else if b.LoadPct == 0 {
			b.State = BoilerOff
			s.logEvent("%s: stoppad", b.Name)
		}
	}
	b.FuelMW = b.LoadPct / 100 * cfg.BoilerFuelMW[b.idx]
	b.SteamMW += (b.FuelMW*cfg.BoilerEff[b.idx] - b.SteamMW) * math.Min(1, dt/cfg.SteamTauS)
	if b.SteamMW < 0.05 {
		b.SteamMW = 0
	}
	s.boilerFlue(b, dt)
}

// boilerFlue updates temperatures and O2 (cosmetic but plausible values).
func (s *Plant) boilerFlue(b *Boiler, dt float64) {
	k := math.Min(1, dt/60)
	load := b.LoadPct / 100
	rated := s.cfg.BoilerFuelMW[b.idx] * s.cfg.BoilerEff[b.idx]
	steamC, o2, flue := 25.0, 20.9, 30.0
	if b.firing() {
		steamC = 250 + 290*math.Min(1, b.SteamMW/(0.3*rated))
		o2 = 3.5 + 4*(1-load)
		flue = 120 + 50*load
	}
	b.SteamC += (steamC - b.SteamC) * k
	b.O2 += (o2 - b.O2) * k
	b.FlueC += (flue - b.FlueC) * k
}

func (s *Plant) tripBoiler(b *Boiler, reason string) {
	if !b.firing() && b.Trip {
		return
	}
	b.State, b.Run, b.Trip = BoilerOff, false, true
	b.LoadPct, b.FuelMW = 0, 0
	s.logEvent("SKYDD: %s utlöst, %s", b.Name, reason)
}

// ---- steam header, turbine and bypass ----

func (s *Plant) steamIn() float64 {
	var in float64
	for _, b := range s.B {
		in += b.SteamMW
	}
	return in
}

func (s *Plant) steam(dt float64) {
	cfg := s.cfg
	in := s.steamIn()
	const kp = 5 // MW per bar
	switch {
	case s.GenCB.Closed:
		// inlet pressure control: the turbine swallows all steam it can
		s.TurbineMW = clamp(in+kp*(s.PressBar-s.PressSetBar), 0, cfg.TurbineMaxMW)
	case s.SpeedRPM > 0:
		s.TurbineMW = math.Min(in, 2*s.SpeedRPM/cfg.RatedRPM) // no-load steam
	default:
		s.TurbineMW = 0
	}
	if s.BypassAuto {
		s.BypassMW = clamp(in-s.TurbineMW+kp*(s.PressBar-(s.PressSetBar+3)), 0, cfg.BypassMaxMW)
	} else {
		s.BypassMW = s.BypassManPct / 100 * cfg.BypassMaxMW
	}
	s.SafetyMW = math.Max(0, (s.PressBar-105)*20)
	loss := 0.01 * s.PressBar
	if dt > 0 {
		s.PressBar = math.Max(0, s.PressBar+(in-s.TurbineMW-s.BypassMW-s.SafetyMW-loss)/cfg.HeaderCap*dt)
	}
	if s.PressBar > cfg.HeaderTripBar {
		for _, b := range s.B {
			if b.firing() {
				s.tripBoiler(b, fmt.Sprintf("högt ångtryck %.1f bar", s.PressBar))
			}
		}
	}
}

func (s *Plant) turbine(dt float64) {
	cfg := s.cfg
	if s.TurbineRun && !s.TurbineTrip {
		if s.SpeedRPM > 0.5*cfg.RatedRPM && s.PressBar < 50 {
			s.tripTurbine(fmt.Sprintf("lågt ångtryck %.1f bar", s.PressBar))
		} else if s.PressBar >= 50 {
			s.SpeedRPM = math.Min(cfg.RatedRPM, s.SpeedRPM+cfg.RatedRPM/cfg.RunUpS*dt)
		}
	} else {
		s.SpeedRPM = math.Max(0, s.SpeedRPM-cfg.RatedRPM/120*dt)
	}
	if s.GenCB.Closed && s.NetHz > 0 {
		s.SpeedRPM = cfg.RatedRPM * s.NetHz / 50 // locked to the grid
	}
}

func (s *Plant) tripTurbine(reason string) {
	s.TurbineTrip, s.TurbineRun = true, false
	s.openGen()
	s.logEvent("SKYDD: turbinen utlöst, %s", reason)
}

func (s *Plant) openGen() {
	if s.GenCB.Closed {
		s.GenCB.Closed = false
		s.GenCB.Ops++
	}
}

// ---- generator ----

func (s *Plant) electrical(dt float64) {
	cfg := s.cfg
	if s.GenCB.Closed && !s.NetLive {
		s.openGen()
		s.GenCB.Lockout, s.GenTrip = true, true
		s.logEvent("SKYDD: generatorbrytaren utlöst, nätbortfall (LOM)")
	}
	if s.GenCB.Closed {
		s.GenP = math.Max(0, cfg.TurbineAlpha*s.TurbineMW-cfg.TurbineLossMW)
		s.GenQ = clamp(s.GenQSet, -0.4*s.GenP-2, 0.6*s.GenP+2)
		s.GenKV, s.GenHz = s.NetKV, s.NetHz
		s.GenI = math.Hypot(s.GenP, s.GenQ) / (sqrt3 * s.GenKV) * 1000
	} else {
		s.GenP, s.GenQ, s.GenI = 0, 0, 0
		s.GenHz = s.SpeedRPM / cfg.RatedRPM * 50
		s.GenKV = cfg.NetKV * math.Min(1, s.SpeedRPM/cfg.RatedRPM)
	}
	s.GenMWh += s.GenP * dt / 3600
}

func (s *Plant) closeGen(on bool) error {
	if !on {
		s.openGen()
		return nil
	}
	if s.GenCB.Closed {
		return nil
	}
	switch {
	case s.GenCB.Lockout || s.TurbineTrip:
		return ErrLockout
	case s.SpeedRPM < 0.98*s.cfg.RatedRPM:
		return fmt.Errorf("%w: turbinen ej på varvtal (%.0f rpm)", ErrSync, s.SpeedRPM)
	case !s.NetLive:
		return fmt.Errorf("%w: ingen spänning på anslutningspunkten", ErrSync)
	}
	s.GenCB.Closed = true
	s.GenCB.Ops++
	return nil
}

// ---- district heating network and accumulator ----

func (s *Plant) heat(dt float64) {
	cfg := s.cfg
	s.SupplySetC = s.supplyCurve()
	condenser := math.Max(0, s.TurbineMW-s.GenP-cfg.TurbineLossMW)
	if !s.GenCB.Closed {
		condenser = s.TurbineMW
	}
	s.Production = condenser + 0.98*s.BypassMW

	need := s.Demand * 1000 / (4.19 * math.Max(10, s.SupplySetC-cfg.ReturnDesignC))
	var pumps float64
	for _, on := range s.Pump {
		if on {
			pumps++
		}
	}
	capacity := pumps * cfg.PumpMaxKgS
	s.FlowKgS = math.Min(need, capacity)
	r := 0.0
	if need > 0 {
		r = s.FlowKgS / need
	}
	s.Delivered = s.Demand * r
	s.DPBar = cfg.DPSetBar * r * r

	// The accumulator absorbs the imbalance and holds the supply temperature.
	acc := s.Production - s.Delivered + 5*(s.SupplyC-s.SupplySetC)
	acc = clamp(acc, -cfg.AccMaxMW, cfg.AccMaxMW)
	if acc > 0 && s.AccMWh >= cfg.AccCapMWh {
		acc = 0
	}
	if acc < 0 && s.AccMWh <= 0 {
		acc = 0
	}
	s.AccMW = acc
	if dt > 0 {
		s.AccMWh = clamp(s.AccMWh+acc*dt/3600, 0, cfg.AccCapMWh)
		s.SupplyC += (s.Production - s.Delivered - acc) / cfg.NetCap * dt
		s.SupplyC = clamp(s.SupplyC, 20, 130)
	}
	if s.FlowKgS > 1 {
		s.ReturnC = math.Max(25, s.SupplyC-s.Delivered*1000/(4.19*s.FlowKgS))
	} else {
		s.ReturnC += (math.Min(s.SupplyC, 40) - s.ReturnC) * math.Min(1, dt/300)
	}
}

// ---- alarms and reset ----

func (s *Plant) highPressure() bool { return s.PressBar > s.cfg.HeaderAlarmBar }
func (s *Plant) lowDP() bool        { return s.DPBar < 0.6 }
func (s *Plant) supplyLow() bool    { return s.SupplyC < s.SupplySetC-8 }
func (s *Plant) supplyHigh() bool   { return s.SupplyC > 120 }

func (s *Plant) commonAlarm() bool {
	for _, b := range s.B {
		if b.Trip {
			return true
		}
	}
	return s.TurbineTrip || s.GenTrip || s.highPressure() || s.lowDP() || s.supplyLow() || s.supplyHigh()
}

func (s *Plant) reset() {
	for _, b := range s.B {
		b.Trip = false
	}
	s.TurbineTrip, s.GenTrip, s.GenCB.Lockout = false, false, false
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

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
