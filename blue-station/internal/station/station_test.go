package station

import (
	"errors"
	"math"
	"testing"
)

func newTest(t *testing.T) *Station {
	t.Helper()
	s := New(DefaultConfig(), 1)
	return s
}

func ctrl(t *testing.T, s *Station, id string, c Ctrl) error {
	t.Helper()
	p := s.PointByID(id)
	if p == nil {
		t.Fatalf("okänd punkt %s", id)
	}
	return s.Control(p, c, "test")
}

func TestNormalOperation(t *testing.T) {
	s := newTest(t)
	s.Tick(1)
	if !s.BusLive[0] || !s.BusLive[1] {
		t.Fatal("båda sektionerna ska vara spänningssatta")
	}
	for i, u := range s.BusKV {
		if math.Abs(u-20.5) > 1 {
			t.Errorf("sektion %d: %.2f kV, förväntat ca 20.5", i, u)
		}
	}
	var load float64
	for _, f := range s.F {
		load += f.P
	}
	if s.GridP <= load {
		t.Errorf("uttag från stamnät %.2f MW ska överstiga lasten %.2f MW (förluster)", s.GridP, load)
	}
}

func TestFeederFaultTripsAndLocksOut(t *testing.T) {
	s := newTest(t)
	if err := ctrl(t, s, "F2.FaultSim", Ctrl{On: true}); err != nil {
		t.Fatal(err)
	}
	if s.BusKV[0] > 10 {
		t.Errorf("spänningen ska sjunka vid fel, är %.2f kV", s.BusKV[0])
	}
	s.Tick(1)
	f := s.F[1]
	if f.CB.Closed || !f.Trip {
		t.Fatal("F2 ska ha lösts ut av I>>")
	}
	if s.T[0].Trip {
		t.Error("T1 ska inte lösa ut när avgången bryter felet")
	}
	if err := ctrl(t, s, "F2.CB", Ctrl{On: true}); !errors.Is(err, ErrLockout) {
		t.Fatalf("tillslag ska blockeras, fick %v", err)
	}
	ctrl(t, s, "F2.FaultSim", Ctrl{On: false})
	ctrl(t, s, "Station.Reset", Ctrl{On: true})
	if err := ctrl(t, s, "F2.CB", Ctrl{On: true}); err != nil {
		t.Fatalf("tillslag efter återställning: %v", err)
	}
	s.Tick(1)
	if !f.CB.Closed || f.P <= 0 {
		t.Error("F2 ska vara inkopplad och ha last")
	}
}

func TestBusCouplerTransfer(t *testing.T) {
	s := newTest(t)
	ctrl(t, s, "T1.LV.CB", Ctrl{On: false})
	if s.BusLive[0] || s.F[0].P != 0 {
		t.Fatal("sektion A ska vara spänningslös")
	}
	ctrl(t, s, "BUS.Coupler.CB", Ctrl{On: true})
	if !s.BusLive[0] || s.F[0].P == 0 {
		t.Fatal("sektion A ska matas via sektioneringen")
	}
	if s.T[0].ILV != 0 || s.T[1].ILV == 0 {
		t.Error("all last ska gå genom T2")
	}
}

func TestAVRRegulatesVoltage(t *testing.T) {
	s := newTest(t)
	if err := ctrl(t, s, "AVR.USet", Ctrl{F: 21.5}); err != nil {
		t.Fatal(err)
	}
	tap := s.T[0].Tap
	for i := 0; i < 60; i++ {
		s.Tick(1)
	}
	if s.T[0].Tap <= tap {
		t.Fatalf("lindningskopplaren ska ha höjts, läge %d", s.T[0].Tap)
	}
	if math.Abs(s.BusKV[0]-21.5)/21.5*100 > s.DeadbandPct {
		t.Errorf("spänning %.2f kV utanför dödband kring 21.5", s.BusKV[0])
	}
	if err := ctrl(t, s, "T1.Tap.Raise", Ctrl{On: true}); !errors.Is(err, ErrAuto) {
		t.Errorf("manuell höjning i auto ska nekas, fick %v", err)
	}
}

func TestSetpointRange(t *testing.T) {
	s := newTest(t)
	if err := ctrl(t, s, "AVR.USet", Ctrl{F: 50}); !errors.Is(err, ErrRange) {
		t.Fatalf("förväntade ErrRange, fick %v", err)
	}
}

func TestGeneratorBay(t *testing.T) {
	s := newTest(t)
	s.Tick(1)
	before := s.T[1].P
	s.SetGenerator(true, true, 20, 4)
	s.Tick(1)
	g := s.G1
	if g.P != 20 || g.I <= 0 {
		t.Fatalf("G1 ska mata in 20 MW, P=%.1f I=%.0f", g.P, g.I)
	}
	if s.T[1].P > before-19 {
		t.Errorf("T2 ska avlastas av generatorn: före %.1f MW, efter %.1f MW", before, s.T[1].P)
	}
	if s.T[1].P >= 0 {
		t.Errorf("med 20 MW inmatning och ca 13 MW last ska T2 mata tillbaka mot stamnätet, P=%.1f", s.T[1].P)
	}
	if live, _, _, closed := s.GridSide(); !live || !closed {
		t.Error("anslutningspunkten ska vara spänningssatt och facket slutet")
	}

	// Opening the bay disconnects the generator immediately.
	if err := ctrl(t, s, "G1.CB", Ctrl{On: false}); err != nil {
		t.Fatal(err)
	}
	if g.P != 0 {
		t.Errorf("öppet fack ska inte mata in, P=%.1f", g.P)
	}
	if _, _, _, closed := s.GridSide(); closed {
		t.Error("GridSide ska rapportera öppet fack")
	}

	// A dead busbar is reported to the plant (which then trips on loss of mains).
	ctrl(t, s, "G1.CB", Ctrl{On: true})
	ctrl(t, s, "T2.LV.CB", Ctrl{On: false})
	if live, _, _, _ := s.GridSide(); live {
		t.Error("sektion B ska vara spänningslös")
	}
	if g.P != 0 {
		t.Error("generatorn får inte mata en spänningslös skena (ingen ödrift)")
	}

	// Without link the generator contributes nothing.
	ctrl(t, s, "T2.LV.CB", Ctrl{On: true})
	s.SetGenerator(false, true, 20, 4)
	s.Tick(1)
	if g.P != 0 || g.GenOnline {
		t.Error("utan länk ska G1 inte mata in")
	}
}
