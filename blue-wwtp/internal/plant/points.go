package plant

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

type builder struct {
	s     *Plant
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
