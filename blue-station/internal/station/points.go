package station

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// Kind decides how a point is exposed over Modbus TCP and IEC 104.
//
//	Kind         Modbus                    IEC 104 monitor   IEC 104 control
//	Breaker      coil + discrete input     M_DP (1xxx)       C_DC (5xxx)
//	Switch       coil + discrete input     M_SP (21xx)       C_SC (61xx)
//	Pulse        coil (write 1)            -                 C_SC (60xx)
//	Status       discrete input            M_SP (20xx)       -
//	Measurement  input register, float32   M_ME_NC (3xxx)    -
//	Step         input register, float32   M_ST (40xx)       C_RC (7xxx)
//	Setpoint     holding register, float32 M_ME_NC (41xx)    C_SE_NC (81xx)
type Kind int

const (
	KindBreaker Kind = iota
	KindSwitch
	KindPulse
	KindStatus
	KindMeasurement
	KindStep
	KindSetpoint
)

var kindNames = [...]string{"breaker", "switch", "pulse", "status", "measurement", "step", "setpoint"}

func (k Kind) String() string { return kindNames[k] }

type Value struct {
	B  bool      // single point state; for breakers: closed
	DP uint8     // double point: 1 = open, 2 = closed
	F  float64   // analog value or tap position
	T  time.Time // time of last change
}

type Ctrl struct {
	On bool    // close / on / raise / execute
	F  float64 // setpoint value
}

type Point struct {
	Index           int
	ID              string
	Group           string
	Desc            string
	Unit            string
	Kind            Kind
	Deadband        float64 // IEC 104 spontaneous transmission threshold
	OnText, OffText string
	Modbus          int    // address within the Modbus table implied by Kind
	IOA             uint32 // IEC 104 monitoring address, 0 = none
	CtrlIOA         uint32 // IEC 104 control address, 0 = none

	get  func() Value
	ctrl func(Ctrl) error
}

func (p *Point) Controllable() bool { return p.ctrl != nil }

func (p *Point) StateText(v Value) string {
	switch p.Kind {
	case KindBreaker:
		switch v.DP {
		case 2:
			return "TILL"
		case 1:
			return "FRÅN"
		}
		return "MELLANLÄGE"
	case KindSwitch, KindStatus, KindPulse:
		if v.B {
			return p.OnText
		}
		return p.OffText
	case KindStep:
		return fmt.Sprintf("%.0f", v.F)
	}
	return strings.TrimSpace(fmt.Sprintf("%.2f %s", v.F, p.Unit))
}

func (p *Point) ctrlText(c Ctrl) string {
	switch p.Kind {
	case KindBreaker:
		if c.On {
			return "TILL"
		}
		return "FRÅN"
	case KindSwitch:
		if c.On {
			return p.OnText
		}
		return p.OffText
	case KindPulse:
		if c.On {
			return "UTFÖR"
		}
		return "(ingen åtgärd)"
	case KindStep:
		if c.On {
			return "HÖJ"
		}
		return "SÄNK"
	}
	return fmt.Sprintf("= %g %s", c.F, p.Unit)
}

func (p *Point) ModbusRef() string {
	switch p.Kind {
	case KindBreaker, KindSwitch:
		return fmt.Sprintf("Coil %d, DI %d", p.Modbus, p.Modbus)
	case KindPulse:
		return fmt.Sprintf("Coil %d", p.Modbus)
	case KindStatus:
		return fmt.Sprintf("DI %d", p.Modbus)
	case KindMeasurement, KindStep:
		return fmt.Sprintf("IR %d–%d", p.Modbus, p.Modbus+1)
	case KindSetpoint:
		return fmt.Sprintf("HR %d–%d", p.Modbus, p.Modbus+1)
	}
	return "–"
}

func (p *Point) IECMonitorRef() string {
	switch p.Kind {
	case KindBreaker:
		return fmt.Sprintf("M_DP %d", p.IOA)
	case KindSwitch, KindStatus:
		return fmt.Sprintf("M_SP %d", p.IOA)
	case KindStep:
		return fmt.Sprintf("M_ST %d", p.IOA)
	case KindMeasurement, KindSetpoint:
		return fmt.Sprintf("M_ME_NC %d", p.IOA)
	}
	return "–"
}

func (p *Point) IECControlRef() string {
	switch p.Kind {
	case KindBreaker:
		return fmt.Sprintf("C_DC %d", p.CtrlIOA)
	case KindSwitch, KindPulse:
		return fmt.Sprintf("C_SC %d", p.CtrlIOA)
	case KindStep:
		return fmt.Sprintf("C_RC %d", p.CtrlIOA)
	case KindSetpoint:
		return fmt.Sprintf("C_SE_NC %d", p.CtrlIOA)
	}
	return "–"
}

// WritePointJSON writes the signal list as JSON, for generating SCADA configuration.
func WritePointJSON(w io.Writer, pts []*Point) error {
	type jp struct {
		ID      string `json:"id"`
		Group   string `json:"group"`
		Desc    string `json:"desc"`
		Unit    string `json:"unit"`
		Kind    string `json:"kind"`
		OnText  string `json:"onText"`
		OffText string `json:"offText"`
		Modbus  int    `json:"modbus"`
		IOA     uint32 `json:"ioa,omitempty"`
		CtrlIOA uint32 `json:"ctrlIoa,omitempty"`
	}
	out := make([]jp, 0, len(pts))
	for _, p := range pts {
		out = append(out, jp{p.ID, p.Group, p.Desc, p.Unit, p.Kind.String(), p.OnText, p.OffText, p.Modbus, p.IOA, p.CtrlIOA})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WritePointTable writes the signal list as a Markdown table.
func WritePointTable(w io.Writer, pts []*Point) {
	fmt.Fprintln(w, "| ID | Beskrivning | Enhet | Modbus TCP | IEC 104 övervakning | IEC 104 styrning |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, p := range pts {
		unit := p.Unit
		if unit == "" {
			unit = "–"
		}
		fmt.Fprintf(w, "| `%s` | %s | %s | %s | %s | %s |\n", p.ID, p.Desc, unit, p.ModbusRef(), p.IECMonitorRef(), p.IECControlRef())
	}
}

type builder struct {
	s     *Station
	group string
	n     map[Kind]int
}

func (b *builder) add(p Point) {
	n := b.n[p.Kind]
	b.n[p.Kind]++
	switch p.Kind {
	case KindBreaker:
		p.Modbus, p.IOA, p.CtrlIOA = n, uint32(1001+n), uint32(5001+n)
	case KindSwitch:
		p.Modbus, p.IOA, p.CtrlIOA = 20+n, uint32(2101+n), uint32(6101+n)
	case KindPulse:
		p.Modbus, p.CtrlIOA = 40+n, uint32(6001+n)
	case KindStatus:
		p.Modbus, p.IOA = 100+n, uint32(2001+n)
	case KindMeasurement:
		p.Modbus, p.IOA = 2*n, uint32(3001+n)
	case KindStep:
		p.Modbus, p.IOA, p.CtrlIOA = 200+2*n, uint32(4001+n), uint32(7001+n)
	case KindSetpoint:
		p.Modbus, p.IOA, p.CtrlIOA = 2*n, uint32(4101+n), uint32(8101+n)
	}
	if p.OnText == "" {
		p.OnText, p.OffText = "PÅ", "AV"
	}
	if p.get == nil {
		p.get = func() Value { return Value{} }
	}
	p.Group = b.group
	p.Index = len(b.s.points)
	b.s.points = append(b.s.points, &p)
	b.s.byID[p.ID] = &p
}

func dp(br *Breaker) func() Value {
	return func() Value {
		if br.Closed {
			return Value{B: true, DP: 2}
		}
		return Value{DP: 1}
	}
}

func sp(f func() bool) func() Value { return func() Value { return Value{B: f()} } }

func an(f func() float64) func() Value { return func() Value { return Value{F: f()} } }

func setBool(p *bool) func(Ctrl) error {
	return func(c Ctrl) error { *p = c.On; return nil }
}

func setFloat(p *float64, lo, hi float64) func(Ctrl) error {
	return func(c Ctrl) error {
		if math.IsNaN(c.F) || c.F < lo || c.F > hi {
			return fmt.Errorf("%w: %g (tillåtet %g–%g)", ErrRange, c.F, lo, hi)
		}
		*p = c.F
		return nil
	}
}

func (s *Station) breakerCtrl(br *Breaker) func(Ctrl) error {
	return func(c Ctrl) error { return s.operate(br, c.On) }
}

func (s *Station) buildPoints() {
	cfg := s.cfg
	s.byID = map[string]*Point{}
	b := &builder{s: s, n: map[Kind]int{}}

	b.group = "Stamnät 220 kV"
	b.add(Point{ID: "Grid.Available", Desc: "Simulering: matning från stamnätet", Kind: KindSwitch,
		OnText: "TILLGÄNGLIG", OffText: "BORTFALL", get: sp(func() bool { return s.gridAvail }), ctrl: setBool(&s.gridAvail)})
	b.add(Point{ID: "Grid.VoltagePresent", Desc: "Spänning på inkommande ledning", Kind: KindStatus,
		OnText: "SPÄNNING", OffText: "SPÄNNINGSLÖS", get: sp(func() bool { return s.GridKV > 0 })})
	b.add(Point{ID: "Grid.U", Desc: "Spänning stamnät", Unit: "kV", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return s.GridKV })})
	b.add(Point{ID: "Grid.f", Desc: "Frekvens", Unit: "Hz", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.GridHz })})
	b.add(Point{ID: "Grid.P", Desc: "Aktiv effekt från stamnät", Unit: "MW", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.GridP })})
	b.add(Point{ID: "Grid.Q", Desc: "Reaktiv effekt från stamnät", Unit: "Mvar", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.GridQ })})
	b.add(Point{ID: "Q0.CB", Desc: "Linjebrytare Q0 mot stamnät", Kind: KindBreaker, get: dp(&s.Line), ctrl: s.breakerCtrl(&s.Line)})
	b.add(Point{ID: "Q0.I", Desc: "Ström genom Q0", Unit: "A", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.LineI })})

	for i := range s.T {
		t := s.T[i]
		n := t.Name
		b.group = fmt.Sprintf("Transformator %s %.0f/%.0f kV, %.0f MVA", n, cfg.HVNominalKV, cfg.LVNominalKV, cfg.TrafoMVA)
		b.add(Point{ID: n + ".HV.CB", Desc: fmt.Sprintf("%s brytare uppsida %.0f kV", n, cfg.HVNominalKV), Kind: KindBreaker, get: dp(&t.HV), ctrl: s.breakerCtrl(&t.HV)})
		b.add(Point{ID: n + ".LV.CB", Desc: fmt.Sprintf("%s brytare nedsida %.0f kV", n, cfg.LVNominalKV), Kind: KindBreaker, get: dp(&t.LV), ctrl: s.breakerCtrl(&t.LV)})
		b.add(Point{ID: n + ".Tap", Desc: fmt.Sprintf("%s lindningskopplarläge (%d–%d)", n, cfg.TapMin, cfg.TapMax), Kind: KindStep,
			get: an(func() float64 { return float64(t.Tap) }),
			ctrl: func(c Ctrl) error {
				if c.On {
					return s.manualTap(t, +1)
				}
				return s.manualTap(t, -1)
			}})
		b.add(Point{ID: n + ".AVR.Auto", Desc: n + " spänningsreglering", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
			get: sp(func() bool { return t.Auto }), ctrl: setBool(&t.Auto)})
		b.add(Point{ID: n + ".Tap.Raise", Desc: n + " höj lindningskopplare (manuell)", Kind: KindPulse,
			ctrl: func(c Ctrl) error {
				if !c.On {
					return nil
				}
				return s.manualTap(t, +1)
			}})
		b.add(Point{ID: n + ".Tap.Lower", Desc: n + " sänk lindningskopplare (manuell)", Kind: KindPulse,
			ctrl: func(c Ctrl) error {
				if !c.On {
					return nil
				}
				return s.manualTap(t, -1)
			}})
		b.add(Point{ID: n + ".P", Desc: n + " aktiv effekt", Unit: "MW", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return t.P })})
		b.add(Point{ID: n + ".Q", Desc: n + " reaktiv effekt", Unit: "Mvar", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return t.Q })})
		b.add(Point{ID: n + ".I.HV", Desc: n + " ström uppsida", Unit: "A", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return t.IHV })})
		b.add(Point{ID: n + ".I.LV", Desc: n + " ström nedsida", Unit: "A", Kind: KindMeasurement, Deadband: 5, get: an(func() float64 { return t.ILV })})
		b.add(Point{ID: n + ".Loading", Desc: n + " belastning", Unit: "%", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return t.Loading })})
		b.add(Point{ID: n + ".OilTemp", Desc: n + " oljetemperatur", Unit: "°C", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return t.OilC })})
		b.add(Point{ID: n + ".Overload", Desc: n + " överlast (>100 %)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(func() bool { return t.Overload })})
		b.add(Point{ID: n + ".OilAlarm", Desc: n + " hög oljetemperatur (>90 °C)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(func() bool { return t.OilAlarm })})
		b.add(Point{ID: n + ".Trip", Desc: n + " skyddsutlösning", Kind: KindStatus, OnText: "UTLÖST", OffText: "NORMAL", get: sp(func() bool { return t.Trip })})
	}

	b.group = fmt.Sprintf("Samlingsskena %.0f kV", cfg.LVNominalKV)
	b.add(Point{ID: "BUS.Coupler.CB", Desc: "Sektioneringsbrytare A–B", Kind: KindBreaker, get: dp(&s.Coupler), ctrl: s.breakerCtrl(&s.Coupler)})
	for i, sec := range []string{"A", "B"} {
		i := i
		b.add(Point{ID: "BUS." + sec + ".Live", Desc: "Spänning sektion " + sec, Kind: KindStatus, OnText: "SPÄNNING", OffText: "SPÄNNINGSLÖS",
			get: sp(func() bool { return s.BusLive[i] })})
		b.add(Point{ID: "BUS." + sec + ".U", Desc: "Spänning sektion " + sec, Unit: "kV", Kind: KindMeasurement, Deadband: 0.05,
			get: an(func() float64 { return s.BusKV[i] })})
	}

	for i := range s.F {
		f := s.F[i]
		n := f.Name
		b.group = fmt.Sprintf("%s – %s (sektion %s)", n, f.Desc, []string{"A", "B"}[f.Bus])
		b.add(Point{ID: n + ".CB", Desc: n + " utgående brytare", Kind: KindBreaker, get: dp(&f.CB), ctrl: s.breakerCtrl(&f.CB)})
		b.add(Point{ID: n + ".P", Desc: n + " aktiv effekt", Unit: "MW", Kind: KindMeasurement, Deadband: 0.05, get: an(func() float64 { return f.P })})
		b.add(Point{ID: n + ".Q", Desc: n + " reaktiv effekt", Unit: "Mvar", Kind: KindMeasurement, Deadband: 0.05, get: an(func() float64 { return f.Q })})
		b.add(Point{ID: n + ".I", Desc: n + " ström", Unit: "A", Kind: KindMeasurement, Deadband: 2, get: an(func() float64 { return f.I })})
		b.add(Point{ID: n + ".Energy", Desc: n + " levererad energi", Unit: "MWh", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return f.EnergyMWh })})
		b.add(Point{ID: n + ".Trip", Desc: n + " överströmsskydd utlöst", Kind: KindStatus, OnText: "UTLÖST", OffText: "NORMAL", get: sp(func() bool { return f.Trip })})
		b.add(Point{ID: n + ".FaultSim", Desc: "Simulering: kortslutning på " + n, Kind: KindSwitch, OnText: "FEL", OffText: "INGET FEL",
			get: sp(func() bool { return f.Fault }), ctrl: setBool(&f.Fault)})
		b.add(Point{ID: n + ".OC.Pickup", Desc: n + " inställning I> (I>> = 3×I>)", Unit: "A", Kind: KindSetpoint,
			get: an(func() float64 { return f.PickupA }), ctrl: setFloat(&f.PickupA, 50, 5000)})
	}

	b.group = "Reglering och simulering"
	b.add(Point{ID: "AVR.USet", Desc: "Börvärde spänning 20 kV", Unit: "kV", Kind: KindSetpoint, get: an(func() float64 { return s.USet }), ctrl: setFloat(&s.USet, 18, 23)})
	b.add(Point{ID: "AVR.Deadband", Desc: "Dödband spänningsreglering", Unit: "%", Kind: KindSetpoint, get: an(func() float64 { return s.DeadbandPct }), ctrl: setFloat(&s.DeadbandPct, 0.5, 5)})
	b.add(Point{ID: "AVR.Delay", Desc: "Fördröjning spänningsreglering", Unit: "s", Kind: KindSetpoint, get: an(func() float64 { return s.DelayS }), ctrl: setFloat(&s.DelayS, 1, 120)})
	b.add(Point{ID: "Load.Scale", Desc: "Simulering: lastskalning alla avgångar", Unit: "%", Kind: KindSetpoint, get: an(func() float64 { return s.LoadScale }), ctrl: setFloat(&s.LoadScale, 0, 300)})
	b.add(Point{ID: "Env.Ambient", Desc: "Simulering: omgivningstemperatur", Unit: "°C", Kind: KindSetpoint, get: an(func() float64 { return s.AmbientC }), ctrl: setFloat(&s.AmbientC, -40, 50)})

	b.group = "Station"
	b.add(Point{ID: "Station.CommonAlarm", Desc: "Summalarm", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.commonAlarm)})
	b.add(Point{ID: "Station.Reset", Desc: "Återställ skyddsutlösningar och blockeringar", Kind: KindPulse,
		ctrl: func(c Ctrl) error {
			if c.On {
				s.reset()
			}
			return nil
		}})

	// G1 is appended last so that the addresses of all earlier points are unchanged.
	g := s.G1
	b.group = fmt.Sprintf("%s – %s (sektion %s)", g.Name, g.Desc, []string{"A", "B"}[g.Bus])
	b.add(Point{ID: "G1.CB", Desc: "G1 brytare kraftvärmeverk", Kind: KindBreaker, get: dp(&g.CB), ctrl: s.breakerCtrl(&g.CB)})
	b.add(Point{ID: "G1.P", Desc: "G1 aktiv effekt (inmatning)", Unit: "MW", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return g.P })})
	b.add(Point{ID: "G1.Q", Desc: "G1 reaktiv effekt (inmatning)", Unit: "Mvar", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return g.Q })})
	b.add(Point{ID: "G1.I", Desc: "G1 ström", Unit: "A", Kind: KindMeasurement, Deadband: 2, get: an(func() float64 { return g.I })})
	b.add(Point{ID: "G1.Energy", Desc: "G1 inmatad energi", Unit: "MWh", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return g.EnergyMWh })})
	b.add(Point{ID: "G1.GenOnline", Desc: "G1 generator i kraftvärmeverket inkopplad", Kind: KindStatus, OnText: "I DRIFT", OffText: "UR DRIFT",
		get: sp(func() bool { return g.GenOnline })})
	b.add(Point{ID: "G1.Link", Desc: "G1 förbindelse med kraftvärmeverket", Kind: KindStatus, OnText: "LÄNK", OffText: "AVBROTT",
		get: sp(func() bool { return g.LinkOK })})
}
