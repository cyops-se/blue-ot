// Package plant simulates a drinking water treatment plant (vattenverk).
//
//	sjö ─[RP1/RP2]─► fällning ─► sedimentering ─► [F1/F2 sandfilter] ─► klor
//	                                                                    │
//	                     nät ◄─[utgående ventil]◄─[HP1/HP2]◄─ rentvattenreservoar
//
// Raw water is pumped from a lake, treated with a coagulant, settled and
// filtered, disinfected with chlorine and stored in a clear water reservoir.
// High-lift pumps keep the network pressure while consumption follows a
// daily profile. Everything is powered through one incoming breaker.
package plant

import (
	"fmt"
	"math"
)

// Config holds the process parameters of the plant.
type Config struct {
	RawPumpLps    float64 // capacity per raw water pump
	RawHeadBar    float64 // lift from the lake through the treatment
	HPPumpLps     float64 // capacity per high-lift pump
	PumpEff       float64 // wire-to-water efficiency
	FilterMaxLps  float64 // hydraulic capacity per filter
	BackwashS     float64 // backwash duration
	BackwashLps   float64 // wash water taken from the reservoir
	ReservoirM3   float64
	ReservoirM    float64 // water depth at full volume
	BaseDemandLps float64 // mean consumption
	HeadlossClean float64 // headloss of a freshly washed filter
	HeadlossAlarm float64 // filter breakthrough begins above this
	ClogRate      float64 // headloss growth, m per s per (l/s)·(NTU+0.5)
	BasePowerKW   float64 // lighting, dosing, ventilation …
}

func DefaultConfig() Config {
	return Config{
		RawPumpLps:    130,
		RawHeadBar:    2,
		HPPumpLps:     160,
		PumpEff:       0.7,
		FilterMaxLps:  150,
		BackwashS:     300,
		BackwashLps:   60,
		ReservoirM3:   8000,
		ReservoirM:    6,
		BaseDemandLps: 120,
		HeadlossClean: 0.3,
		HeadlossAlarm: 2.2,
		ClogRate:      6e-6,
		BasePowerKW:   15,
	}
}

// Pump is a speed-controlled pump with motor protection.
type Pump struct {
	Name, Desc string
	Run        bool // operator start/stop
	FaultSim   bool
	Trip       bool

	running  bool
	SpeedPct float64
	FlowLps  float64
	PowerKW  float64
}

// Filter is a rapid sand filter.
type Filter struct {
	Name      string
	HeadlossM float64
	FlowLps   float64
	washLeft  float64 // seconds of backwash remaining
}

func (f *Filter) inService() bool { return f.washLeft <= 0 }

type Plant struct {
	core
	cfg Config

	Supply     Breaker
	PowerAvail bool

	RP [2]*Pump
	HP [2]*Pump

	RawAuto   bool
	LevelSetM float64
	RawFlow   float64
	StormNTU  float64

	TurbRaw, TurbSettled, TurbTreated float64

	CoagAuto bool
	CoagSet  float64
	CoagDose float64

	F           [2]*Filter
	TreatedFlow float64
	WashFlow    float64

	ClAuto     bool
	ClSet      float64
	ClTarget   float64
	ClDose     float64
	ClResidual float64
	PH         float64

	ReservoirM3 float64

	OutletOpen            bool
	Demand, Delivered     float64
	PressBar, PressSetBar float64
	DemandScale           float64

	PowerKW   float64
	EnergyMWh float64

	noise, turbNoise float64
}

// New creates a plant in normal operation.
func New(cfg Config, timeScale float64) *Plant {
	s := &Plant{
		core: newCore(timeScale), cfg: cfg,
		PowerAvail: true, RawAuto: true, LevelSetM: 4,
		CoagAuto: true, CoagSet: 15,
		ClAuto: true, ClSet: 0.6, ClTarget: 0.3,
		OutletOpen: true, PressSetBar: 4.5, DemandScale: 100,
	}
	s.Supply.Closed = true
	s.RP = [2]*Pump{{Name: "RP1", Desc: "Råvattenpump 1", Run: true}, {Name: "RP2", Desc: "Råvattenpump 2", Run: true}}
	s.HP = [2]*Pump{{Name: "HP1", Desc: "Högtryckspump 1", Run: true}, {Name: "HP2", Desc: "Högtryckspump 2", Run: true}}
	s.F = [2]*Filter{{Name: "F1", HeadlossM: 0.8}, {Name: "F2", HeadlossM: 1.3}}
	s.ReservoirM3 = cfg.ReservoirM3 * s.LevelSetM / cfg.ReservoirM
	s.PressBar = s.PressSetBar
	s.step(0)
	// start in a settled state
	s.CoagDose = s.coagTarget()
	s.TurbSettled = s.settledTarget()
	s.TurbTreated = s.treatedTarget()
	s.ClDose = s.clTarget()
	s.ClResidual = math.Max(0, s.ClDose-s.clDemand())
	s.PH = 7.8 - 0.02*s.CoagDose
	s.finish()
	return s
}

func (s *Plant) powered() bool { return s.PowerAvail && s.Supply.Closed }

func (s *Plant) level() float64 { return s.ReservoirM3 / s.cfg.ReservoirM3 * s.cfg.ReservoirM }

// step advances the process dt seconds (dt = 0 recomputes without dynamics).
func (s *Plant) step(dt float64) {
	cfg := s.cfg
	if dt > 0 {
		s.advance(dt)
		sq := math.Sqrt(dt)
		s.noise = clamp(s.noise-s.noise*0.02*dt+s.rng.NormFloat64()*0.01*sq, -0.15, 0.15)
		s.turbNoise = clamp(s.turbNoise-s.turbNoise*0.01*dt+s.rng.NormFloat64()*0.05*sq, -0.8, 0.8)
	}
	h := s.hour()
	profile := 0.55 + 0.75*bump(h, 7.5, 1.5) + 0.25*bump(h, 12.5, 2) + 0.5*bump(h, 19, 2)
	s.Demand = cfg.BaseDemandLps * profile * s.DemandScale / 100 * (1 + s.noise)

	powered := s.powered()
	level := s.level()
	for _, p := range s.RP {
		s.pumpState(p, powered, false)
	}
	for _, p := range s.HP {
		s.pumpState(p, powered, level < 0.3)
	}

	s.distribution(dt, level)
	s.treatment(dt)

	if dt > 0 {
		s.ReservoirM3 = clamp(s.ReservoirM3+(s.TreatedFlow-s.Delivered-s.WashFlow)*dt/1000, 0, cfg.ReservoirM3)
	}

	s.PowerKW = 0
	if powered {
		s.PowerKW = cfg.BasePowerKW
		for _, p := range append(s.RP[:], s.HP[:]...) {
			s.PowerKW += p.PowerKW
		}
	}
	s.EnergyMWh += s.PowerKW * dt / 3600 / 1000
}

func (s *Plant) pumpState(p *Pump, powered, dry bool) {
	if p.Run && !p.Trip && powered {
		switch {
		case p.FaultSim:
			s.tripPump(p, "motorskydd")
		case dry:
			s.tripPump(p, "torrkörningsskydd, låg nivå i reservoaren")
		}
	}
	p.running = p.Run && !p.Trip && powered
}

func (s *Plant) tripPump(p *Pump, reason string) {
	p.Trip = true
	s.logEvent("SKYDD: %s (%s) utlöst, %s", p.Name, p.Desc, reason)
}

// distribution: high-lift pumps share the delivered flow and hold the network pressure.
func (s *Plant) distribution(dt, level float64) {
	cfg := s.cfg
	n := 0
	for _, p := range s.HP {
		if p.running {
			n++
		}
	}
	capacity := float64(n) * cfg.HPPumpLps
	need := 0.0
	if s.OutletOpen {
		need = s.Demand
	}
	deliver := 0.0
	if n > 0 && level > 0.2 {
		deliver = math.Min(need, capacity)
	}
	s.Delivered = deliver

	target := 0.6 // static pressure from the water tower when the plant does not feed the network
	if n > 0 && level > 0.2 && s.OutletOpen {
		r := 1.0
		if need > 0 {
			r = math.Min(1, capacity/need)
		}
		target = math.Max(0.6, s.PressSetBar*r*r)
	}
	if dt > 0 {
		s.PressBar = lag(s.PressBar, target, dt, 5)
	}

	for _, p := range s.HP {
		p.FlowLps, p.SpeedPct, p.PowerKW = 0, 0, 0
		if !p.running {
			continue
		}
		p.FlowLps = deliver / float64(n)
		p.SpeedPct = clamp(p.FlowLps/cfg.HPPumpLps*100, 35, 100)
		p.PowerKW = pumpPower(p.FlowLps, s.PressBar, cfg.PumpEff)
	}
}

// pumpPower is the electrical power of a pump delivering lps against bar.
func pumpPower(lps, bar, eff float64) float64 {
	return 2 + lps/1000*bar*1e5/eff/1000
}

func (s *Plant) coagTarget() float64 {
	if s.CoagAuto {
		return 8 + 3*s.TurbRaw
	}
	return s.CoagSet
}

func (s *Plant) settledTarget() float64 {
	dopt := 8 + 3*s.TurbRaw
	eff := 0.92 * math.Min(1, s.CoagDose/dopt)
	return s.TurbRaw*(1-eff) + 0.2
}

func filterEff(h, alarm float64) float64 {
	if h <= alarm {
		return 0.96
	}
	return clamp(0.96-0.45*(h-alarm), 0.4, 0.96)
}

func (s *Plant) treatedTarget() float64 {
	var sum, n float64
	for _, f := range s.F {
		if f.inService() {
			sum += s.TurbSettled * (1 - filterEff(f.HeadlossM, s.cfg.HeadlossAlarm))
			n++
		}
	}
	if n == 0 {
		return s.TurbTreated
	}
	return sum/n + 0.03
}

func (s *Plant) clDemand() float64 { return 0.25 + 0.3*s.TurbTreated }

func (s *Plant) clTarget() float64 {
	if s.ClAuto {
		return s.ClTarget + s.clDemand()
	}
	return s.ClSet
}

// treatment: raw water intake, coagulation, settling, filtration and disinfection.
func (s *Plant) treatment(dt float64) {
	cfg := s.cfg
	nRP := 0
	for _, p := range s.RP {
		if p.running {
			nRP++
		}
	}
	nF := 0
	for _, f := range s.F {
		if f.inService() {
			nF++
		}
	}
	limit := math.Min(float64(nRP)*cfg.RawPumpLps, float64(nF)*cfg.FilterMaxLps)
	target := limit
	if s.RawAuto {
		target = s.Delivered + 60*(s.LevelSetM-s.level())
	}
	raw := clamp(target, 0, limit)
	s.RawFlow = raw
	s.TreatedFlow = raw
	for _, p := range s.RP {
		p.FlowLps, p.SpeedPct, p.PowerKW = 0, 0, 0
		if p.running && raw > 0 {
			p.FlowLps = raw / float64(nRP)
			p.SpeedPct = clamp(p.FlowLps/cfg.RawPumpLps*100, 30, 100)
			p.PowerKW = pumpPower(p.FlowLps, cfg.RawHeadBar, cfg.PumpEff)
		}
	}

	s.TurbRaw = math.Max(0.5, 3+s.StormNTU+s.turbNoise)
	coag := s.coagTarget()
	if raw <= 0 {
		coag = 0
	}
	s.CoagDose = lag(s.CoagDose, coag, dt, 30)
	s.TurbSettled = lag(s.TurbSettled, s.settledTarget(), dt, 60)

	// filters
	s.WashFlow = 0
	per := 0.0
	if nF > 0 {
		per = raw / float64(nF)
	}
	for _, f := range s.F {
		f.FlowLps = 0
		if !f.inService() {
			s.WashFlow += cfg.BackwashLps
			f.washLeft -= dt
			if f.washLeft <= 0 {
				f.washLeft = 0
				f.HeadlossM = cfg.HeadlossClean
				s.logEvent("%s: backspolning klar", f.Name)
			}
			continue
		}
		f.FlowLps = per
		f.HeadlossM += cfg.ClogRate * per * (s.TurbSettled + 0.5) * dt
	}
	if raw > 0 {
		s.TurbTreated = lag(s.TurbTreated, s.treatedTarget(), dt, 60)
	}

	// disinfection: the residual is measured at the reservoir outlet
	cl := s.clTarget()
	if raw <= 0 {
		cl = 0
	}
	s.ClDose = lag(s.ClDose, cl, dt, 20)
	if raw > 0 {
		s.ClResidual = lag(s.ClResidual, math.Max(0, s.ClDose-s.clDemand()), dt, 120)
	} else {
		s.ClResidual = lag(s.ClResidual, 0, dt, 1800)
	}
	s.PH = lag(s.PH, 7.8-0.02*s.CoagDose+0.3*s.noise, dt, 60)
}

func (s *Plant) backwash(f, other *Filter) error {
	switch {
	case !f.inService():
		return fmt.Errorf("%s backspolas redan", f.Name)
	case !other.inService():
		return fmt.Errorf("%s backspolas, bara ett filter i taget", other.Name)
	case !s.powered():
		return ErrNoPower
	}
	f.washLeft = s.cfg.BackwashS
	s.logEvent("%s: backspolning startad", f.Name)
	return nil
}

// ---- alarms and reset ----

func (s *Plant) resLow() bool           { return s.level() < 1.0 }
func (s *Plant) resHigh() bool          { return s.level() > 0.95*s.cfg.ReservoirM }
func (s *Plant) turbHigh() bool         { return s.TurbTreated > 0.5 }
func (s *Plant) clLow() bool            { return s.ClResidual < 0.1 }
func (s *Plant) clHigh() bool           { return s.ClResidual > 1.0 }
func (s *Plant) pressLow() bool         { return s.PressBar < 2.5 }
func (s *Plant) clogged(f *Filter) bool { return f.HeadlossM > s.cfg.HeadlossAlarm }

func (s *Plant) commonAlarm() bool {
	for _, p := range append(s.RP[:], s.HP[:]...) {
		if p.Trip {
			return true
		}
	}
	for _, f := range s.F {
		if s.clogged(f) {
			return true
		}
	}
	return !s.powered() || s.resLow() || s.resHigh() || s.turbHigh() || s.clLow() || s.clHigh() || s.pressLow()
}

func (s *Plant) reset() {
	for _, p := range append(s.RP[:], s.HP[:]...) {
		p.Trip = false
	}
	s.Supply.Lockout = false
}
