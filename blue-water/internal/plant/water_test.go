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
	run(s, 600)
	if math.Abs(s.PressBar-s.PressSetBar) > 0.2 {
		t.Errorf("nättryck %.2f bar, börvärde %.2f", s.PressBar, s.PressSetBar)
	}
	if math.Abs(s.Delivered-s.Demand) > 0.01 {
		t.Errorf("levererat %.1f l/s, förbrukning %.1f l/s", s.Delivered, s.Demand)
	}
	if math.Abs(s.level()-s.LevelSetM) > 0.5 {
		t.Errorf("reservoarnivå %.2f m, börvärde %.2f", s.level(), s.LevelSetM)
	}
	if s.TurbTreated > 0.5 || s.ClResidual < 0.1 || s.ClResidual > 1 {
		t.Errorf("vattenkvalitet: turbiditet %.2f NTU, restklor %.2f mg/l", s.TurbTreated, s.ClResidual)
	}
	if s.commonAlarm() {
		t.Error("inget summalarm förväntas i normaldrift")
	}
	t.Logf("förbrukning %.0f l/s, råvatten %.0f l/s, nivå %.2f m, tryck %.2f bar, turb %.2f NTU, klor %.2f mg/l, effekt %.0f kW",
		s.Demand, s.RawFlow, s.level(), s.PressBar, s.TurbTreated, s.ClResidual, s.PowerKW)
}

func TestSupplyBreakerStopsPumps(t *testing.T) {
	s := newTest(t)
	run(s, 10)
	if err := ctrl(t, s, "Supply.CB", Ctrl{On: false}); err != nil {
		t.Fatal(err)
	}
	run(s, 60)
	for _, p := range append(s.RP[:], s.HP[:]...) {
		if p.running || p.FlowLps != 0 {
			t.Errorf("%s ska stå utan matning", p.Name)
		}
	}
	if !s.commonAlarm() || s.PowerKW != 0 || s.Delivered != 0 {
		t.Error("spänningslöst verk ska larma och inte leverera")
	}
	if s.PressBar > 1 {
		t.Errorf("nättrycket ska falla till statiskt tryck, är %.2f bar", s.PressBar)
	}
	ctrl(t, s, "Supply.CB", Ctrl{On: true})
	run(s, 60)
	if s.Delivered == 0 || s.PressBar < 4 {
		t.Errorf("leveransen ska återupptas, tryck %.2f bar", s.PressBar)
	}
}

func TestPumpTripAndReset(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "HP1.FaultSim", Ctrl{On: true})
	s.Tick(1)
	if !s.HP[0].Trip || s.HP[0].running {
		t.Fatal("HP1 ska lösa ut på motorfel")
	}
	if err := ctrl(t, s, "HP1.Run", Ctrl{On: true}); !errors.Is(err, ErrLockout) {
		t.Errorf("start efter utlösning ska blockeras, fick %v", err)
	}
	ctrl(t, s, "HP1.FaultSim", Ctrl{On: false})
	ctrl(t, s, "Plant.Reset", Ctrl{On: true})
	s.Tick(1)
	if s.HP[0].Trip || !s.HP[0].running {
		t.Error("HP1 ska gå igen efter återställning")
	}
}

func TestOneHighLiftPumpAtPeakGivesLowPressure(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Demand.Scale", Ctrl{F: 250})
	ctrl(t, s, "HP2.Run", Ctrl{On: false})
	run(s, 60)
	if s.Demand <= s.cfg.HPPumpLps {
		t.Skipf("förbrukningen %.0f l/s räcker inte för att överbelasta en pump", s.Demand)
	}
	if !s.pressLow() {
		t.Errorf("en pump vid hög förbrukning ska ge lågt tryck, %.2f bar", s.PressBar)
	}
	ctrl(t, s, "HP2.Run", Ctrl{On: true})
	run(s, 60)
	if s.pressLow() {
		t.Errorf("två pumpar ska återställa trycket, %.2f bar", s.PressBar)
	}
}

func TestBackwash(t *testing.T) {
	s := newTest(t)
	f := s.F[1]
	if err := ctrl(t, s, "F2.Backwash", Ctrl{On: true}); err != nil {
		t.Fatal(err)
	}
	if err := ctrl(t, s, "F1.Backwash", Ctrl{On: true}); err == nil {
		t.Error("två filter får inte backspolas samtidigt")
	}
	s.Tick(1)
	if f.inService() || s.WashFlow == 0 || s.F[0].FlowLps == 0 {
		t.Fatal("F2 ska vara ur drift och F1 ta hela flödet")
	}
	run(s, int(s.cfg.BackwashS))
	if !f.inService() || math.Abs(f.HeadlossM-s.cfg.HeadlossClean) > 0.05 {
		t.Errorf("F2 ska vara tillbaka med rent filter, motstånd %.2f m", f.HeadlossM)
	}
}

func TestFilterCloggingWithStorm(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Raw.StormNTU", Ctrl{F: 40})
	run(s, 1800)
	if !s.clogged(s.F[1]) {
		t.Errorf("F2 ska sättas igen vid hög turbiditet, motstånd %.2f m", s.F[1].HeadlossM)
	}
}

func TestUnderdosingAndRecovery(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Coag.Auto", Ctrl{On: false})
	ctrl(t, s, "Coag.DoseSet", Ctrl{F: 5})
	ctrl(t, s, "Raw.StormNTU", Ctrl{F: 30})
	run(s, 900)
	if !s.turbHigh() {
		t.Errorf("underdosering vid storm ska ge turbiditetslarm, %.2f NTU", s.TurbTreated)
	}
	// Restore dosing; the overloaded filters must be backwashed one at a time.
	ctrl(t, s, "Coag.Auto", Ctrl{On: true})
	run(s, 120)
	bw := int(s.cfg.BackwashS) + 1
	if err := ctrl(t, s, "F1.Backwash", Ctrl{On: true}); err != nil {
		t.Fatal(err)
	}
	run(s, bw)
	if err := ctrl(t, s, "F2.Backwash", Ctrl{On: true}); err != nil {
		t.Fatal(err)
	}
	run(s, bw)
	run(s, 300)
	if s.turbHigh() {
		t.Errorf("rätt dosering och backspolade filter ska ge rent vatten igen, %.2f NTU", s.TurbTreated)
	}
}

func TestOutletClosed(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Outlet.Valve", Ctrl{On: false})
	run(s, 60)
	if s.Delivered != 0 || !s.pressLow() {
		t.Errorf("stängd ventil ska ge noll leverans och lågt tryck, %.2f bar", s.PressBar)
	}
}

func TestSetpointRange(t *testing.T) {
	s := newTest(t)
	if err := ctrl(t, s, "Net.PressureSet", Ctrl{F: 50}); !errors.Is(err, ErrRange) {
		t.Fatalf("förväntade ErrRange, fick %v", err)
	}
}
