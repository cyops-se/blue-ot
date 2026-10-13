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

func TestSteadyState(t *testing.T) {
	s := newTest(t)
	run(s, 600)
	if math.Abs(s.PressBar-s.PressSetBar) > 1 {
		t.Errorf("ångtryck %.1f bar, börvärde %.1f", s.PressBar, s.PressSetBar)
	}
	if s.GenP < 10 || s.GenP > 35 {
		t.Errorf("generatoreffekt %.1f MW orimlig", s.GenP)
	}
	if math.Abs(s.SupplyC-s.SupplySetC) > 2 {
		t.Errorf("framledning %.1f °C, börvärde %.1f", s.SupplyC, s.SupplySetC)
	}
	if math.Abs(s.Delivered-s.Demand) > 0.01 {
		t.Errorf("levererat %.1f MW, behov %.1f MW", s.Delivered, s.Demand)
	}
	if s.BypassMW > 1 {
		t.Errorf("bypass ska vara stängd i normaldrift, %.1f MW", s.BypassMW)
	}
	if s.commonAlarm() {
		t.Error("inget summalarm förväntas i normaldrift")
	}
	t.Logf("P1 %.0f %%, ånga %.1f MW, el %.1f MW, värme %.1f/%.1f MW, ack %.1f %%",
		s.B[0].LoadPct, s.steamIn(), s.GenP, s.Production, s.Demand, s.soc())
}

func TestTurbineTripBypassTakesOver(t *testing.T) {
	s := newTest(t)
	run(s, 60)
	if err := ctrl(t, s, "Turbine.EmergencyStop", Ctrl{On: true}); err != nil {
		t.Fatal(err)
	}
	if s.GenCB.Closed || !s.TurbineTrip {
		t.Fatal("snabbstopp ska lösa turbinen och öppna generatorbrytaren")
	}
	maxP := 0.0
	for i := 0; i < 300; i++ {
		s.Tick(1)
		maxP = math.Max(maxP, s.PressBar)
	}
	if maxP > s.cfg.HeaderTripBar {
		t.Errorf("trycket %.1f bar ska begränsas av bypass", maxP)
	}
	if s.BypassMW < 30 || s.GenP != 0 {
		t.Errorf("bypass %.1f MW, el %.1f MW", s.BypassMW, s.GenP)
	}
	if math.Abs(s.SupplyC-s.SupplySetC) > 3 {
		t.Errorf("värmeleveransen ska fortsätta via bypass, framledning %.1f °C", s.SupplyC)
	}
	if err := ctrl(t, s, "Turbine.Run", Ctrl{On: true}); !errors.Is(err, ErrLockout) {
		t.Errorf("start efter utlösning ska blockeras, fick %v", err)
	}
}

func TestHighPressureTripsBoilers(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "Bypass.Auto", Ctrl{On: false})
	ctrl(t, s, "Turbine.EmergencyStop", Ctrl{On: true})
	run(s, 120)
	for _, b := range s.B[:1] {
		if !b.Trip || b.firing() {
			t.Fatalf("%s ska ha lösts ut på högt tryck (%.1f bar)", b.Name, s.PressBar)
		}
	}
	if err := ctrl(t, s, "P1.Run", Ctrl{On: true}); !errors.Is(err, ErrLockout) {
		t.Errorf("start ska blockeras, fick %v", err)
	}
	// Restore pressure relief before restarting, otherwise the boiler trips again.
	ctrl(t, s, "Bypass.Auto", Ctrl{On: true})
	run(s, 60)
	if s.PressBar > s.cfg.HeaderAlarmBar {
		t.Fatalf("bypass ska sänka trycket, %.1f bar", s.PressBar)
	}
	ctrl(t, s, "Plant.Reset", Ctrl{On: true})
	if err := ctrl(t, s, "P1.Run", Ctrl{On: true}); err != nil {
		t.Fatal(err)
	}
	run(s, 200)
	if s.B[0].State != BoilerRunning {
		t.Errorf("P1 ska vara i drift efter återställning och start, läge %v", s.B[0].State)
	}
}

func TestLossOfMains(t *testing.T) {
	s := newTest(t)
	out := s.Link(LinkIn{BusLive: true, BusKV: 20.4, Hz: 50, BayClosed: true})
	if !out.GenOnline || out.P <= 0 || !s.LinkOK {
		t.Fatalf("generatorn ska vara i drift via länken: %+v", out)
	}
	out = s.Link(LinkIn{BusLive: true, BusKV: 20.4, Hz: 50, BayClosed: false})
	if out.GenOnline || !s.GenTrip {
		t.Fatal("öppet fack ska ge nätbortfallsskydd")
	}
	run(s, 2)
	if s.SpeedRPM < 0.98*s.cfg.RatedRPM {
		t.Errorf("turbinen ska fortsätta på varvtal, %.0f rpm", s.SpeedRPM)
	}
	if err := ctrl(t, s, "Gen.CB", Ctrl{On: true}); !errors.Is(err, ErrLockout) {
		t.Errorf("tillslag ska blockeras, fick %v", err)
	}
	ctrl(t, s, "Plant.Reset", Ctrl{On: true})
	if err := ctrl(t, s, "Gen.CB", Ctrl{On: true}); !errors.Is(err, ErrSync) {
		t.Errorf("synkronisering utan spänning ska nekas, fick %v", err)
	}
	s.Link(LinkIn{BusLive: true, BusKV: 20.4, Hz: 50, BayClosed: true})
	if err := ctrl(t, s, "Gen.CB", Ctrl{On: true}); err != nil {
		t.Fatal(err)
	}
	run(s, 30)
	if s.GenP <= 0 {
		t.Error("generatorn ska producera efter återinkoppling")
	}
}

func TestBoilerFaultAndAutoStart(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "P2.Run", Ctrl{On: true})
	run(s, 200)
	if s.B[1].State != BoilerRunning {
		t.Fatalf("P2 ska vara i drift, läge %v", s.B[1].State)
	}
	ctrl(t, s, "P1.FaultSim", Ctrl{On: true})
	s.Tick(1)
	if !s.B[0].Trip {
		t.Fatal("P1 ska lösas ut av bränslematningsfel")
	}
	run(s, 300)
	if s.B[1].LoadPct < 99 {
		t.Errorf("P2 ska gå upp i last när P1 löser ut, %.0f %%", s.B[1].LoadPct)
	}
}

func TestPumpStopLowDP(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "DH.Pump1", Ctrl{On: false})
	ctrl(t, s, "DH.Pump2", Ctrl{On: false})
	s.Tick(1)
	if !s.lowDP() || s.Delivered != 0 {
		t.Fatal("utan pumpar ska differenstrycket falla och leveransen upphöra")
	}
	run(s, 60)
	if s.AccMW <= 0 {
		t.Error("ackumulatorn ska ta emot produktionen")
	}
}

func TestSetpointRange(t *testing.T) {
	s := newTest(t)
	if err := ctrl(t, s, "Steam.PressureSet", Ctrl{F: 150}); !errors.Is(err, ErrRange) {
		t.Fatalf("förväntade ErrRange, fick %v", err)
	}
}
