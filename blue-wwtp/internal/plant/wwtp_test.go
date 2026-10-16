package plant

import (
	"errors"
	"math"
	"testing"
)

func newTest(t *testing.T) *Plant {
	t.Helper()
	return New(DefaultConfig(), 1)
}

func ctrl(t *testing.T, s *Plant, id string, c Ctrl) error {
	t.Helper()
	p := s.PointByID(id)
	if p == nil {
		t.Fatalf("okänd punkt %s", id)
	}
	return s.Control(p, c, "test")
}

func run(s *Plant, seconds int) {
	for i := 0; i < seconds; i++ {
		s.Tick(1)
	}
}

func TestNoAlarmAtStart(t *testing.T) {
	s := newTest(t)
	if s.commonAlarm() {
		t.Error("simulatorn ska starta utan larm")
	}
	for _, p := range s.Points() {
		if p.Kind == KindStatus && p.OffText == "NORMAL" && s.vals[p.Index].B {
			t.Errorf("%s i larm vid start", p.ID)
		}
	}
}

func TestSteadyState(t *testing.T) {
	s := newTest(t)
	run(s, 3600)
	if math.Abs(s.DO-s.DOSet) > 0.5 {
		t.Errorf("syrehalt %.2f mg/l, börvärde %.2f", s.DO, s.DOSet)
	}
	if s.nh4High() || s.ptotHigh() {
		t.Errorf("utgående NH4 %.2f / Ptot %.2f mg/l över gränsvärde", s.NH4Out, s.PtotOut)
	}
	if s.OverflowM3 > 0 || s.WellLevel < 0.3 || s.WellLevel > 3.0 {
		t.Errorf("pumpsump %.2f m, bräddat %.1f m³", s.WellLevel, s.OverflowM3)
	}
	if s.commonAlarm() {
		t.Error("inget summalarm förväntas i normaldrift")
	}
	t.Logf("inflöde %.0f l/s, sump %.2f m, DO %.2f mg/l, luft %.0f Nm³/h, NH4 %.2f, Ptot %.2f, BOD7 %.1f mg/l, biogas %.0f Nm³/h, effekt %.0f kW",
		s.Inflow, s.WellLevel, s.DO, s.Air, s.NH4Out, s.PtotOut, s.BOD7, s.Biogas, s.PowerKW)
}

func TestHeavyRainOverflows(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Rain.Intensity", Ctrl{F: 30})
	run(s, 1800)
	running := 0
	for _, m := range s.IP {
		if m.running {
			running++
		}
	}
	if running != 3 {
		t.Errorf("alla tre pumpar ska gå vid kraftigt regn, %d går", running)
	}
	if !s.overflowing() || s.OverflowM3 <= 0 {
		t.Errorf("inflödet %.0f l/s ska ge bräddning", s.Inflow)
	}
	if s.NH4In >= s.cfg.NH4InMgl {
		t.Error("regnvatten ska späda inkommande ammonium")
	}
}

func TestBlockedScreenOverflowsEarlier(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Rain.Intensity", Ctrl{F: 12})
	ctrl(t, s, "Screen.FaultSim", Ctrl{On: true})
	run(s, 1800)
	if !s.overflowing() {
		t.Errorf("igensatt galler vid regn ska ge bräddning, sump %.2f m", s.WellLevel)
	}
	ctrl(t, s, "Screen.FaultSim", Ctrl{On: false})
	ctrl(t, s, "Rain.Intensity", Ctrl{F: 0})
	run(s, 1800)
	if s.overflowing() {
		t.Error("bräddningen ska upphöra när gallret är rensat")
	}
}

func TestBlowerTripLowersOxygen(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "B2.Run", Ctrl{On: false})
	ctrl(t, s, "B1.FaultSim", Ctrl{On: true})
	run(s, 600)
	if !s.B[0].Trip || s.Air != 0 {
		t.Fatalf("B1 ska ha löst ut och luften upphört, luft %.0f", s.Air)
	}
	if !s.doLow() {
		t.Errorf("syrehalten ska falla utan luftning, %.2f mg/l", s.DO)
	}
	nh4 := s.NH4Out
	run(s, 1800)
	if s.NH4Out <= nh4 {
		t.Error("ammonium ut ska stiga när nitrifikationen stannar")
	}
	if err := ctrl(t, s, "B1.Run", Ctrl{On: true}); !errors.Is(err, ErrLockout) {
		t.Errorf("start efter utlösning ska blockeras, fick %v", err)
	}
	ctrl(t, s, "B2.Run", Ctrl{On: true})
	run(s, 900)
	if s.doLow() {
		t.Errorf("B2 ska återställa syrehalten, %.2f mg/l", s.DO)
	}
}

func TestColdWaterNitrification(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Water.Temp", Ctrl{F: 4})
	run(s, 5400)
	if !s.nh4High() {
		t.Errorf("kallt vatten ska ge ammonium över gränsvärdet, %.2f mg/l", s.NH4Out)
	}
}

func TestUnderdosedPrecipitant(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Prec.Auto", Ctrl{On: false})
	ctrl(t, s, "Prec.DoseSet", Ctrl{F: 3})
	run(s, 3600)
	if !s.ptotHigh() {
		t.Errorf("för låg fällningsdos ska ge fosfor över gränsvärde, %.2f mg/l", s.PtotOut)
	}
}

func TestBoilerStopCoolsDigester(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Boiler.Run", Ctrl{On: false})
	run(s, 6*3600)
	if !s.digesterLow() {
		t.Errorf("rötkammaren ska svalna utan gaspanna, %.1f °C", s.DigesterTemp)
	}
	if s.Biogas > 0.5*s.biogasTarget()+1 && s.Biogas > 20 {
		t.Errorf("gasproduktionen ska avta i kall rötkammare, %.0f Nm³/h", s.Biogas)
	}
}

func TestExcessGasIsFlared(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Load.Scale", Ctrl{F: 250})
	run(s, 4*3600)
	if !s.Flaring {
		t.Errorf("överskottsgas ska facklas, gasklocka %.0f %%", s.gasholderPct())
	}
	if s.gasholderPct() > 96 {
		t.Errorf("facklan ska begränsa gasklockan, %.0f %%", s.gasholderPct())
	}
}

func TestSupplyBreaker(t *testing.T) {
	s := newTest(t)
	if err := ctrl(t, s, "Supply.CB", Ctrl{On: false}); err != nil {
		t.Fatal(err)
	}
	run(s, 1800)
	if s.Pumped != 0 || s.Air != 0 || s.PowerKW != 0 {
		t.Error("utan elmatning ska pumpar och blåsmaskiner stå")
	}
	if !s.overflowing() || !s.commonAlarm() {
		t.Error("strömavbrott ska leda till bräddning och summalarm")
	}
}

func TestSetpointRange(t *testing.T) {
	s := newTest(t)
	if err := ctrl(t, s, "Bio.DOSet", Ctrl{F: 50}); !errors.Is(err, ErrRange) {
		t.Fatalf("förväntade ErrRange, fick %v", err)
	}
}
