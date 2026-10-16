package plant

import (
	"strconv"
	"strings"
)

// sv formats a number with a Swedish decimal comma.
func sv(v float64) string { return strings.ReplaceAll(strconv.FormatFloat(v, 'f', -1, 64), ".", ",") }

func (s *Plant) machinePoints(b *builder, m *Machine, flowUnit, flowDesc string) {
	n := m.Name
	b.add(Point{ID: n + ".Run", Desc: m.Desc + " driftval (MANUELL: kör, AUTO: tillgänglig)", Kind: KindSwitch, OnText: "START", OffText: "STOPP",
		get: sp(func() bool { return m.Run }),
		ctrl: func(c Ctrl) error {
			if c.On && m.Trip {
				return ErrLockout
			}
			m.Run = c.On
			return nil
		}})
	b.add(Point{ID: n + ".Running", Desc: m.Desc + " i drift", Kind: KindStatus, OnText: "I DRIFT", OffText: "STOPPAD",
		get: sp(func() bool { return m.running })})
	b.add(Point{ID: n + ".Trip", Desc: m.Desc + " motorskydd utlöst", Kind: KindStatus, OnText: "UTLÖST", OffText: "NORMAL",
		get: sp(func() bool { return m.Trip })})
	b.add(Point{ID: n + "." + flowDesc, Desc: m.Desc + " flöde", Unit: flowUnit, Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return m.Flow })})
	b.add(Point{ID: n + ".Power", Desc: m.Desc + " effekt", Unit: "kW", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return m.PowerKW })})
	b.add(Point{ID: n + ".FaultSim", Desc: "Simulering: motorfel " + n, Kind: KindSwitch, OnText: "FEL", OffText: "INGET FEL",
		get: sp(func() bool { return m.FaultSim }), ctrl: setBool(&m.FaultSim)})
}

func (s *Plant) buildPoints() {
	cfg := s.cfg
	s.byID = map[string]*Point{}
	b := &builder{s: s, n: map[Kind]int{}}

	b.group = "Elmatning"
	b.add(Point{ID: "Supply.CB", Desc: "Inkommande brytare (elmatning)", Kind: KindBreaker, get: dp(&s.Supply),
		ctrl: func(c Ctrl) error { return s.Supply.operate(c.On) }})
	b.add(Point{ID: "Power.Available", Desc: "Simulering: extern elmatning", Kind: KindSwitch, OnText: "TILLGÄNGLIG", OffText: "BORTFALL",
		get: sp(func() bool { return s.PowerAvail }), ctrl: setBool(&s.PowerAvail)})
	b.add(Point{ID: "Power.Lost", Desc: "Verket spänningslöst", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL",
		get: sp(func() bool { return !s.powered() })})
	b.add(Point{ID: "Power.Total", Desc: "Total eleffekt", Unit: "kW", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.PowerKW })})
	b.add(Point{ID: "Power.Energy", Desc: "Förbrukad elenergi", Unit: "MWh", Kind: KindMeasurement, Deadband: 0.001, get: an(func() float64 { return s.EnergyMWh })})

	b.group = "Inloppspumpstation"
	b.add(Point{ID: "Inflow.Flow", Desc: "Inkommande flöde från ledningsnätet", Unit: "l/s", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.Inflow })})
	b.add(Point{ID: "Well.Level", Desc: "Nivå pumpsump", Unit: "m", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.WellLevel })})
	b.add(Point{ID: "Well.High", Desc: "Hög nivå pumpsump (>3,3 m)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.wellHigh)})
	b.add(Point{ID: "Inlet.Auto", Desc: "Nivåstyrning inloppspumpar", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
		get: sp(func() bool { return s.InletAuto }), ctrl: setBool(&s.InletAuto)})
	for _, m := range s.IP {
		s.machinePoints(b, m, "l/s", "Flow")
	}
	b.add(Point{ID: "Pumped.Flow", Desc: "Pumpat flöde till reningen", Unit: "l/s", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.Pumped })})
	b.add(Point{ID: "Overflow.Active", Desc: "Bräddning pågår", Kind: KindStatus, OnText: "BRÄDDNING", OffText: "NORMAL", get: sp(s.overflowing)})
	b.add(Point{ID: "Overflow.Flow", Desc: "Bräddat flöde", Unit: "l/s", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.Overflow })})
	b.add(Point{ID: "Overflow.Volume", Desc: "Bräddad volym (ackumulerad)", Unit: "m³", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return s.OverflowM3 })})

	b.group = "Galler"
	b.add(Point{ID: "Screen.Running", Desc: "Galler i drift", Kind: KindStatus, OnText: "I DRIFT", OffText: "STOPPAT",
		get: sp(func() bool { return s.Pumped > 0 && !s.ScreenFault })})
	b.add(Point{ID: "Screen.Blocked", Desc: "Galler igensatt", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL",
		get: sp(func() bool { return s.ScreenFault })})
	b.add(Point{ID: "Screen.FaultSim", Desc: "Simulering: igensatt galler", Kind: KindSwitch, OnText: "FEL", OffText: "INGET FEL",
		get: sp(func() bool { return s.ScreenFault }), ctrl: setBool(&s.ScreenFault)})

	b.group = "Biologisk rening (aktivslam)"
	b.add(Point{ID: "Blower.Auto", Desc: "Syrereglering blåsmaskiner", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
		get: sp(func() bool { return s.BlowerAuto }), ctrl: setBool(&s.BlowerAuto)})
	for _, m := range s.B {
		s.machinePoints(b, m, "Nm³/h", "Air")
	}
	b.add(Point{ID: "Air.Flow", Desc: "Total luftmängd", Unit: "Nm³/h", Kind: KindMeasurement, Deadband: 10, get: an(func() float64 { return s.Air })})
	b.add(Point{ID: "Bio.Flow", Desc: "Flöde genom bassängerna", Unit: "l/s", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.BioFlow })})
	b.add(Point{ID: "Bio.DO", Desc: "Löst syre i luftningsbassäng", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.02, get: an(func() float64 { return s.DO })})
	b.add(Point{ID: "Bio.DOSet", Desc: "Börvärde löst syre", Unit: "mg/l", Kind: KindSetpoint,
		get: an(func() float64 { return s.DOSet }), ctrl: setFloat(&s.DOSet, 0.5, 4)})
	b.add(Point{ID: "Bio.DOLow", Desc: "Lågt syre (<1,0 mg/l)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.doLow)})

	b.group = "Kemisk fällning"
	b.add(Point{ID: "Prec.Auto", Desc: "Dosering fällningskemikalie", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
		get: sp(func() bool { return s.PrecAuto }), ctrl: setBool(&s.PrecAuto)})
	b.add(Point{ID: "Prec.Dose", Desc: "Dos fällningskemikalie", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.PrecDose })})
	b.add(Point{ID: "Prec.DoseSet", Desc: "Dos fällningskemikalie vid manuell", Unit: "mg/l", Kind: KindSetpoint,
		get: an(func() float64 { return s.PrecSet }), ctrl: setFloat(&s.PrecSet, 0, 50)})

	b.group = "In- och utgående vatten"
	b.add(Point{ID: "In.NH4", Desc: "Ammoniumkväve inkommande", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.NH4In })})
	b.add(Point{ID: "In.Ptot", Desc: "Totalfosfor inkommande", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.05, get: an(func() float64 { return s.PIn })})
	b.add(Point{ID: "Eff.Flow", Desc: "Utgående flöde till recipient", Unit: "l/s", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.BioFlow })})
	b.add(Point{ID: "Eff.NH4", Desc: "Ammoniumkväve utgående", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.05, get: an(func() float64 { return s.NH4Out })})
	b.add(Point{ID: "Eff.Ptot", Desc: "Totalfosfor utgående", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.PtotOut })})
	b.add(Point{ID: "Eff.BOD7", Desc: "BOD7 utgående", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.BOD7 })})
	b.add(Point{ID: "Eff.NH4High", Desc: "Ammonium över gränsvärde (>" + sv(cfg.NH4LimitMgl) + " mg/l)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.nh4High)})
	b.add(Point{ID: "Eff.PtotHigh", Desc: "Fosfor över gränsvärde (>" + sv(cfg.PtotLimitMgl) + " mg/l)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.ptotHigh)})

	b.group = "Rötning och biogas"
	b.add(Point{ID: "Digester.Temp", Desc: "Temperatur rötkammare", Unit: "°C", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.DigesterTemp })})
	b.add(Point{ID: "Digester.TempLow", Desc: "Låg temperatur rötkammare (<33 °C)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.digesterLow)})
	b.add(Point{ID: "Boiler.Run", Desc: "Gaspanna (värmer rötkammaren)", Kind: KindSwitch, OnText: "START", OffText: "STOPP",
		get: sp(func() bool { return s.BoilerRun }), ctrl: setBool(&s.BoilerRun)})
	b.add(Point{ID: "Biogas.Flow", Desc: "Biogasproduktion", Unit: "Nm³/h", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.Biogas })})
	b.add(Point{ID: "Gasholder.Level", Desc: "Nivå gasklocka", Unit: "%", Kind: KindMeasurement, Deadband: 0.2, get: an(s.gasholderPct)})
	b.add(Point{ID: "Gas.Flaring", Desc: "Fackla brinner (överskottsgas)", Kind: KindStatus, OnText: "FACKLA", OffText: "AV",
		get: sp(func() bool { return s.Flaring })})

	b.group = "Simulering"
	b.add(Point{ID: "Rain.Intensity", Desc: "Simulering: regnintensitet", Unit: "mm/h", Kind: KindSetpoint,
		get: an(func() float64 { return s.Rain }), ctrl: setFloat(&s.Rain, 0, 60)})
	b.add(Point{ID: "Load.Scale", Desc: "Simulering: skalning spillvattenbelastning", Unit: "%", Kind: KindSetpoint,
		get: an(func() float64 { return s.LoadScale }), ctrl: setFloat(&s.LoadScale, 0, 250)})
	b.add(Point{ID: "Water.Temp", Desc: "Simulering: vattentemperatur", Unit: "°C", Kind: KindSetpoint,
		get: an(func() float64 { return s.WaterTemp }), ctrl: setFloat(&s.WaterTemp, 2, 25)})

	b.group = "Verk"
	b.add(Point{ID: "Plant.CommonAlarm", Desc: "Summalarm", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.commonAlarm)})
	b.add(Point{ID: "Plant.Reset", Desc: "Återställ utlösningar", Kind: KindPulse,
		ctrl: func(c Ctrl) error {
			if c.On {
				s.reset()
			}
			return nil
		}})
}
