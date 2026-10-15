package state

import "testing"

func TestHoldFreezesReported(t *testing.T) {
	s := New()
	s.Register(&Point{Key: "x"})
	if e, _ := s.Report("x", 10); e != 10 {
		t.Fatalf("report=%v", e)
	}
	s.SetHold("x", true)
	if e, held := s.Report("x", 99); e != 10 || !held {
		t.Errorf("held report=%v held=%v; want 10,true", e, held)
	}
	s.SetHold("x", false)
	if e, _ := s.Report("x", 99); e != 99 {
		t.Errorf("released report=%v; want 99", e)
	}
}

func TestBlock(t *testing.T) {
	s := New()
	s.Register(&Point{Key: "a", Category: "brytare"})
	s.Register(&Point{Key: "b", Category: "pump"})
	if s.IsBlocked("a") {
		t.Fatal("not blocked initially")
	}
	if n := s.SetBlockCategory("brytare", true); n != 1 {
		t.Errorf("affected=%d; want 1", n)
	}
	if !s.IsBlocked("a") || s.IsBlocked("b") {
		t.Error("only brytare should be blocked")
	}
	if n := s.SetBlockCategory("all", false); n != 2 {
		t.Errorf("affected=%d; want 2", n)
	}
	if s.IsBlocked("a") {
		t.Error("should be unblocked")
	}
}
