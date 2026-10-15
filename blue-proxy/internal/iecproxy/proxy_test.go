package iecproxy

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"blue-proxy/internal/config"
	"blue-proxy/internal/state"
)

func newProxy(tags ...config.Tag) (*Proxy, *state.Store) {
	store := state.New()
	p := New(":0", "127.0.0.1:0", time.Second, &config.Loaded{Tags: tags}, store)
	return p, store
}

func collected(store *state.Store, key string) (float64, bool) {
	for _, v := range store.Snapshot() {
		if v.Key == key && v.Collected != nil {
			return *v.Collected, true
		}
	}
	return 0, false
}

func reported(store *state.Store, key string) float64 {
	for _, v := range store.Snapshot() {
		if v.Key == key && v.Reported != nil {
			return *v.Reported
		}
	}
	return math.NaN()
}

func meAsdu(ioa uint32, v float32) []byte {
	a := []byte{mMENC, 1, 3, 0, 1, 0, byte(ioa), byte(ioa >> 8), byte(ioa >> 16)}
	a = binary.LittleEndian.AppendUint32(a, math.Float32bits(v))
	return append(a, 0x00) // QDS
}

func meValue(asdu []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(asdu[9:13]))
}

func TestMonitoringFloatHold(t *testing.T) {
	p, store := newProxy(config.Tag{ID: "X", IOA: 3001, DataType: config.TypeFloat32, Scale: 1})

	p.onMonitoring(meAsdu(3001, 42.5))
	if v, ok := collected(store, "X"); !ok || v != 42.5 {
		t.Fatalf("collected = %v,%v; want 42.5", v, ok)
	}

	store.SetHold("X", true)
	a := meAsdu(3001, 99.0) // source now reports 99
	p.onMonitoring(a)
	if v, _ := collected(store, "X"); v != 99 {
		t.Errorf("collected = %v; want 99 (source value still recorded)", v)
	}
	if r := reported(store, "X"); r != 42.5 {
		t.Errorf("reported = %v; want 42.5 (frozen)", r)
	}
	if got := meValue(a); got != 42.5 {
		t.Errorf("rewritten ASDU value = %v; want 42.5 (frozen value sent downstream)", got)
	}
}

func dpAsdu(ioa uint32, on bool) []byte {
	diq := byte(1) // OFF
	if on {
		diq = 2
	}
	return []byte{mDPNA, 1, 3, 0, 1, 0, byte(ioa), byte(ioa >> 8), byte(ioa >> 16), diq}
}

func TestMonitoringDoublePointHold(t *testing.T) {
	p, store := newProxy(config.Tag{ID: "CB", IOA: 1001, Scale: 1})

	p.onMonitoring(dpAsdu(1001, true)) // closed
	if v, _ := collected(store, "CB"); v != 1 {
		t.Fatalf("collected = %v; want 1 (closed)", v)
	}
	store.SetHold("CB", true)

	a := dpAsdu(1001, false) // source opens
	p.onMonitoring(a)
	if v, _ := collected(store, "CB"); v != 0 {
		t.Errorf("collected = %v; want 0 (source opened)", v)
	}
	if a[9]&0x03 != 2 {
		t.Errorf("DIQ = %d; want 2 (ON frozen)", a[9]&0x03)
	}
}

func TestMonitoringSequence(t *testing.T) {
	// SQ=1: one base IOA then consecutive elements (two M_ME_NC values).
	p, store := newProxy(
		config.Tag{ID: "A", IOA: 3001, DataType: config.TypeFloat32, Scale: 1},
		config.Tag{ID: "B", IOA: 3002, DataType: config.TypeFloat32, Scale: 1},
	)
	a := []byte{mMENC, 0x82, 3, 0, 1, 0, 0xB9, 0x0B, 0x00} // VSQ: SQ=1, count=2, base IOA 3001
	a = binary.LittleEndian.AppendUint32(a, math.Float32bits(10))
	a = append(a, 0x00)
	a = binary.LittleEndian.AppendUint32(a, math.Float32bits(20))
	a = append(a, 0x00)
	p.onMonitoring(a)
	if v, _ := collected(store, "A"); v != 10 {
		t.Errorf("A = %v; want 10", v)
	}
	if v, _ := collected(store, "B"); v != 20 {
		t.Errorf("B = %v; want 20", v)
	}
}

func TestCommandRecord(t *testing.T) {
	p, store := newProxy(config.Tag{ID: "CB", IOA: 1001, CtrlIOA: 5001, Cmd: "C_DC",
		StateText: map[string]string{"0": "Från", "1": "Till"}})
	// C_DC close: DCS=2
	asdu := []byte{cDCNA, 1, 6, 0, 1, 0, 0x89, 0x13, 0x00, 2} // IOA 5001
	p.onCommand(asdu)
	for _, v := range store.Snapshot() {
		if v.Key == "CB" {
			if v.LastCmd != "Till  @ "+time.Now().Format("15:04:05") && v.LastCmd[:4] != "Till" {
				t.Errorf("lastCmd = %q; want starting with Till", v.LastCmd)
			}
			return
		}
	}
	t.Error("CB point not found")
}

func TestBlockCommand(t *testing.T) {
	p, store := newProxy(config.Tag{ID: "CB", IOA: 1001, CtrlIOA: 5001, Cmd: "C_DC",
		StateText: map[string]string{"0": "Från", "1": "Till"}})
	store.SetBlock("CB", true)
	asdu := []byte{cDCNA, 1, 6, 0, 1, 0, 0x89, 0x13, 0x00, 2} // IOA 5001, close
	p.onCommand(asdu)
	// The command IOA must be zeroed so the device rejects it.
	if asdu[6] != 0 || asdu[7] != 0 || asdu[8] != 0 {
		t.Errorf("IOA not zeroed: % x", asdu[6:9])
	}
	for _, v := range store.Snapshot() {
		if v.Key == "CB" {
			if !v.LastCmdBad {
				t.Error("blocked command should be marked bad")
			}
			return
		}
	}
	t.Error("CB point not found")
}
