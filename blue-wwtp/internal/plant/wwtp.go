// Package plant simulates a municipal wastewater treatment plant (avloppsverk).
//
//	ledningsnät ─► inloppspumpstation ─► galler ─► aktivslam (luftning) ─► fällning ─► recipient
//	                [IP1–IP3]   │                       [B1/B2]
//	                        bräddning                     slam ─► rötkammare ─► biogas ─► gaspanna / fackla
//
// Sewage flows in with a daily profile plus rain. Three level-controlled inlet
// pumps lift it to the treatment; when they cannot keep up the well
// overflows. Blowers aerate the activated sludge to a dissolved oxygen
// setpoint, which drives nitrification. Phosphorus is removed with a
// precipitant. Sludge is digested and the biogas heats the digester; excess
// gas is flared. Everything is powered through one incoming breaker.
package plant

import (
	"fmt"
	"math"
)

// Config holds the process parameters of the plant.
type Config struct {
	InletPumpLps  float64    // capacity per inlet pump (fixed speed)
	InletHeadBar  float64    // lift to the treatment
	PumpEff       float64    // wire-to-water efficiency
	WellAreaM2    float64    // inlet well surface area
	WellOverflowM float64    // overflow level
	StartLevels   [3]float64 // level control: start of pump 1, 2, 3
	StopLevels    [3]float64 // stop of pump 1, 2, 3
	BaseInflowLps float64    // mean dry weather inflow
	RainLpsPerMm  float64    // extra inflow per mm/h of rain
	BlowerNm3h    float64    // air capacity per blower
	BlowerKW      float64    // power at full capacity
	NH4InMgl      float64    // dry weather ammonium
	PInMgl        float64    // dry weather phosphorus
	BiogasPerLps  float64    // Nm³/h biogas per l/s dry weather flow
	GasholderNm3  float64
	BoilerNm3h    float64 // gas boiler heating the digester
	DigesterC     float64 // digester temperature setpoint (mesophilic)
	BasePowerKW   float64
	NH4LimitMgl   float64 // permit limits
	PtotLimitMgl  float64
}

func DefaultConfig() Config {
	return Config{
		InletPumpLps:  220,
		InletHeadBar:  1.2,
		PumpEff:       0.65,
		WellAreaM2:    60,
		WellOverflowM: 3.8,
		StartLevels:   [3]float64{1.6, 2.3, 3.0},
		StopLevels:    [3]float64{0.8, 1.5, 2.2},
		BaseInflowLps: 180,
		RainLpsPerMm:  25,
		BlowerNm3h:    3000,
		BlowerKW:      90,
		NH4InMgl:      35,
		PInMgl:        6,
		BiogasPerLps:  0.8,
		GasholderNm3:  1500,
		BoilerNm3h:    80,
		DigesterC:     37,
		BasePowerKW:   40,
		NH4LimitMgl:   5,
		PtotLimitMgl:  0.3,
	}
}

// Machine is a pump or blower. Run is the operator's selection: in MANUAL the
// machine runs, in AUTO it is available to the controller.
type Machine struct {
	Name, Desc string
	Run        bool
	FaultSim   bool
	Trip       bool

	running bool
	Flow    float64 // l/s for pumps, Nm³/h for blowers
	PowerKW float64
}

func (m *Machine) available(powered bool) bool { return m.Run && !m.Trip && powered }

type Plant struct {
	core
	cfg Config

	Supply     Breaker
	PowerAvail bool

	IP          [3]*Machine
	InletAuto   bool
	WellLevel   float64
	want        int
	Inflow      float64
	Pumped      float64
	BioFlow     float64 // flow through the basins, equalized by their volume
	Overflow    float64
	OverflowM3  float64
	ScreenFault bool

	B          [2]*Machine
	BlowerAuto bool
	DOSet      float64
	DO         float64
	Air        float64
	integ      float64

	NH4In, PIn            float64
	NH4Out, PtotOut, BOD7 float64

	PrecAuto bool
	PrecSet  float64
	PrecDose float64

	DigesterTemp float64
	BoilerRun    bool
	Biogas       float64
	GasholderNm3 float64
	Flaring      bool

	Rain      float64
	LoadScale float64
	WaterTemp float64

	PowerKW   float64
	EnergyMWh float64

	noise float64
}

// New creates a plant in normal dry weather operation.
func New(cfg Config, timeScale float64) *Plant {
	s := &Plant{
		core: newCore(timeScale), cfg: cfg,
		PowerAvail: true, InletAuto: true, WellLevel: 1.8,
		BlowerAuto: true, DOSet: 2, DO: 2,
		PrecAuto: true, PrecSet: 10,
		DigesterTemp: cfg.DigesterC, BoilerRun: true,
		LoadScale: 100, WaterTemp: 14,
	}
	s.Supply.Closed = true
	s.GasholderNm3 = cfg.GasholderNm3 / 2
	for i := range s.IP {
		s.IP[i] = &Machine{Name: fmt.Sprintf("IP%d", i+1), Desc: fmt.Sprintf("Inloppspump %d", i+1), Run: true}
	}
	for i := range s.B {
		s.B[i] = &Machine{Name: fmt.Sprintf("B%d", i+1), Desc: fmt.Sprintf("Blåsmaskin %d", i+1), Run: true}
	}
	s.want = 1
	s.BioFlow = s.dryFlow()
	s.step(0)
	// start in a settled state
	s.PrecDose = s.precTarget()
	s.NH4Out = s.nh4Target()
	s.PtotOut = s.ptotTarget()
	s.BOD7 = s.bodTarget()
	s.Biogas = s.biogasTarget()
	s.finish()
	return s
}

func (s *Plant) powered() bool { return s.PowerAvail && s.Supply.Closed }

// dryFlow is the sewage part of the inflow (without rain).
func (s *Plant) dryFlow() float64 {
	h := s.hour()
	profile := 0.6 + 0.8*bump(h, 9.5, 2) + 0.5*bump(h, 20, 2.5)
	return s.cfg.BaseInflowLps * profile * s.LoadScale / 100 * (1 + s.noise)
}

// step advances the process dt seconds (dt = 0 recomputes without dynamics).
func (s *Plant) step(dt float64) {
	cfg := s.cfg
	if dt > 0 {
		s.advance(dt)
		s.noise = clamp(s.noise-s.noise*0.02*dt+s.rng.NormFloat64()*0.01*math.Sqrt(dt), -0.15, 0.15)
	}
	powered := s.powered()
	dry := s.dryFlow()
	s.Inflow = dry + s.Rain*cfg.RainLpsPerMm
	s.NH4In, s.PIn = 0, 0
	if s.Inflow > 0 {
		s.NH4In = cfg.NH4InMgl * dry / s.Inflow
		s.PIn = cfg.PInMgl * dry / s.Inflow
	}

	s.inlet(dt, powered)
	s.BioFlow = lag(s.BioFlow, s.Pumped, dt, 1200)
	s.aeration(dt, powered)
	s.quality(dt)
	s.digestion(dt, powered, dry)

	s.PowerKW = 0
	if powered {
		s.PowerKW = cfg.BasePowerKW
		for _, m := range s.IP {
			s.PowerKW += m.PowerKW
		}
		for _, m := range s.B {
			s.PowerKW += m.PowerKW
		}
	}
	s.EnergyMWh += s.PowerKW * dt / 3600 / 1000
}

func (s *Plant) protect(m *Machine, powered bool) {
	if m.Run && !m.Trip && powered && m.FaultSim {
		m.Trip = true
		s.logEvent("SKYDD: %s (%s) utlöst, motorskydd", m.Name, m.Desc)
	}
}

// inlet: level-controlled pumping station with overflow.
func (s *Plant) inlet(dt float64, powered bool) {
	cfg := s.cfg
	for _, m := range s.IP {
		s.protect(m, powered)
		m.running = false
	}
	if s.InletAuto {
		if s.want < 3 && s.WellLevel > cfg.StartLevels[s.want] {
			s.want++
		}
		if s.want > 0 && s.WellLevel < cfg.StopLevels[s.want-1] {
			s.want--
		}
		n := 0
		for _, m := range s.IP {
			if n < s.want && m.available(powered) {
				m.running = true
				n++
			}
		}
	} else {
		for _, m := range s.IP {
			m.running = m.available(powered)
		}
	}

	capacity := 0.0
	for _, m := range s.IP {
		if m.running {
			capacity += cfg.InletPumpLps
		}
	}
	if s.ScreenFault {
		capacity *= 0.5 // a blocked screen throttles the flow to the pumps
	}
	pumped := capacity
	if s.WellLevel < 0.5 {
		pumped = math.Min(capacity, s.Inflow) // do not run the well dry
	}
	s.Pumped = pumped
	s.Overflow = 0
	if s.WellLevel >= cfg.WellOverflowM && s.Inflow > pumped {
		s.Overflow = s.Inflow - pumped
	}
	if dt > 0 {
		s.WellLevel += (s.Inflow - pumped - s.Overflow) / 1000 * dt / cfg.WellAreaM2
		s.WellLevel = clamp(s.WellLevel, 0, cfg.WellOverflowM)
		s.OverflowM3 += s.Overflow * dt / 1000
	}

	n := 0
	for _, m := range s.IP {
		if m.running {
			n++
		}
	}
	for _, m := range s.IP {
		m.Flow, m.PowerKW = 0, 0
		if m.running {
			m.Flow = pumped / float64(n)
			m.PowerKW = 1.5 + m.Flow/1000*cfg.InletHeadBar*1e5/cfg.PumpEff/1000
		}
	}
}

// load is the nitrogen load relative to the design load.
func (s *Plant) load() float64 {
	return s.BioFlow * s.NH4In / (s.cfg.BaseInflowLps * s.cfg.NH4InMgl)
}

func (s *Plant) tempFactor() float64 { return math.Exp(0.06 * (s.WaterTemp - 12)) }

// aeration: blowers controlled to the dissolved oxygen setpoint.
func (s *Plant) aeration(dt float64, powered bool) {
	cfg := s.cfg
	avail := 0
	for _, m := range s.B {
		s.protect(m, powered)
		m.running = false
		if m.available(powered) {
			avail++
		}
	}
	our := 0.012 * s.load() * s.tempFactor() // oxygen uptake, mg/l/s

	var air float64
	if s.BlowerAuto {
		e := s.DOSet - s.DO
		if dt > 0 {
			s.integ = clamp(s.integ+e*dt, -3000, 3000)
		}
		air = clamp(1000*s.load()*s.tempFactor()+800*e+s.integ, 0, float64(avail)*cfg.BlowerNm3h)
		need := 0
		if air > 0 {
			need = 1
		}
		if air > 0.9*cfg.BlowerNm3h {
			need = 2
		}
		n := 0
		for _, m := range s.B {
			if n < need && m.available(powered) {
				m.running = true
				n++
			}
		}
		if n > 0 {
			air = math.Min(air, float64(n)*cfg.BlowerNm3h)
		} else {
			air = 0
		}
	} else {
		for _, m := range s.B {
			if m.available(powered) {
				m.running = true
				air += 0.8 * cfg.BlowerNm3h
			}
		}
	}
	s.Air = air

	n := 0
	for _, m := range s.B {
		if m.running {
			n++
		}
	}
	for _, m := range s.B {
		m.Flow, m.PowerKW = 0, 0
		if m.running {
			m.Flow = air / float64(n)
			m.PowerKW = 8 + cfg.BlowerKW*m.Flow/cfg.BlowerNm3h
		}
	}

	if dt > 0 {
		kla := air * 2e-6
		s.DO = clamp(s.DO+(kla*(8-s.DO)-our)*dt, 0, 8)
	}
}

func (s *Plant) nh4Target() float64 {
	eff := 0.985 * s.DO / (s.DO + 0.1) * math.Min(1, math.Exp(0.1*(s.WaterTemp-12)))
	return s.NH4In*(1-eff) + 0.2
}

func (s *Plant) bodTarget() float64 { return 3 + 12*(1-s.DO/(s.DO+0.5)) }

func (s *Plant) precTarget() float64 {
	if s.PrecAuto {
		return 2.5 * s.PIn
	}
	return s.PrecSet
}

func (s *Plant) ptotTarget() float64 {
	if s.PIn <= 0 {
		return 0.05
	}
	eff := 0.97 * math.Min(1, s.PrecDose/(2.5*s.PIn))
	return s.PIn*(1-eff) + 0.05
}

// quality: effluent concentrations follow the process with time lags.
func (s *Plant) quality(dt float64) {
	dose := s.precTarget()
	if s.BioFlow <= 1 {
		dose = 0
	}
	s.PrecDose = lag(s.PrecDose, dose, dt, 60)
	s.NH4Out = lag(s.NH4Out, s.nh4Target(), dt, 900)
	s.BOD7 = lag(s.BOD7, s.bodTarget(), dt, 900)
	s.PtotOut = lag(s.PtotOut, s.ptotTarget(), dt, 600)
}

func (s *Plant) biogasTarget() float64 {
	warm := clamp((s.DigesterTemp-20)/(s.cfg.DigesterC-20), 0, 1)
	return s.cfg.BiogasPerLps * s.cfg.BaseInflowLps * s.LoadScale / 100 * warm
}

// digestion: digester temperature, biogas, gas holder and flare.
func (s *Plant) digestion(dt float64, powered bool, dry float64) {
	cfg := s.cfg
	boiler := s.BoilerRun && powered && s.GasholderNm3 > 1
	target := 15.0
	if boiler {
		target = cfg.DigesterC
	}
	s.DigesterTemp = lag(s.DigesterTemp, target, dt, 3600)
	s.Biogas = lag(s.Biogas, s.biogasTarget(), dt, 1800)

	use := 0.0
	if boiler {
		use = cfg.BoilerNm3h
	}
	if dt > 0 {
		s.GasholderNm3 += (s.Biogas - use) * dt / 3600
	}
	s.Flaring = hyst(s.Flaring, s.GasholderNm3/cfg.GasholderNm3, 0.95, 0.85)
	if s.Flaring && s.GasholderNm3 > 0.85*cfg.GasholderNm3 {
		s.GasholderNm3 = math.Min(s.GasholderNm3, 0.95*cfg.GasholderNm3)
	}
	s.GasholderNm3 = clamp(s.GasholderNm3, 0, cfg.GasholderNm3)
}

// ---- alarms and reset ----

func (s *Plant) wellHigh() bool        { return s.WellLevel > 3.3 }
func (s *Plant) overflowing() bool     { return s.Overflow > 0 }
func (s *Plant) doLow() bool           { return s.DO < 1.0 }
func (s *Plant) nh4High() bool         { return s.NH4Out > s.cfg.NH4LimitMgl }
func (s *Plant) ptotHigh() bool        { return s.PtotOut > s.cfg.PtotLimitMgl }
func (s *Plant) digesterLow() bool     { return s.DigesterTemp < 33 }
func (s *Plant) gasholderPct() float64 { return s.GasholderNm3 / s.cfg.GasholderNm3 * 100 }

func (s *Plant) commonAlarm() bool {
	for _, m := range s.IP {
		if m.Trip {
			return true
		}
	}
	for _, m := range s.B {
		if m.Trip {
			return true
		}
	}
	return !s.powered() || s.overflowing() || s.ScreenFault || s.doLow() || s.nh4High() || s.ptotHigh() || s.digesterLow()
}

func (s *Plant) reset() {
	for _, m := range s.IP {
		m.Trip = false
	}
	for _, m := range s.B {
		m.Trip = false
	}
	s.Supply.Lockout = false
}
