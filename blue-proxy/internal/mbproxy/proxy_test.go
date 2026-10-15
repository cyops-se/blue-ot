package mbproxy

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

func reported(store *state.Store, key string) float64 {
	for _, v := range store.Snapshot() {
		if v.Key == key && v.Reported != nil {
			return *v.Reported
		}
	}
	return math.NaN()
}

// float32ReadResponse builds an FC3 response of two registers (ABCD) for v.
func float32Resp(v float32) []byte {
	resp := []byte{3, 4, 0, 0, 0, 0}
	bits := math.Float32bits(v)
	binary.BigEndian.PutUint16(resp[2:], uint16(bits>>16))
	binary.BigEndian.PutUint16(resp[4:], uint16(bits))
	return resp
}

func float32Of(resp []byte) float32 {
	bits := uint32(binary.BigEndian.Uint16(resp[2:]))<<16 | uint32(binary.BigEndian.Uint16(resp[4:]))
	return math.Float32frombits(bits)
}

func TestResponseFloatHold(t *testing.T) {
	p, store := newProxy(config.Tag{ID: "U", Area: config.AreaHolding, Address: 10,
		DataType: config.TypeFloat32, Scale: 1})
	req := []byte{3, 0, 10, 0, 2} // read holding 10, qty 2

	resp := float32Resp(20.5)
	p.onResponse(req, resp)
	if r := reported(store, "U"); r != 20.5 {
		t.Fatalf("reported = %v; want 20.5", r)
	}

	store.SetHold("U", true)
	resp2 := float32Resp(99.0) // source now 99
	p.onResponse(req, resp2)
	if got := float32Of(resp2); got != 20.5 {
		t.Errorf("rewritten response = %v; want 20.5 (frozen)", got)
	}
	if r := reported(store, "U"); r != 20.5 {
		t.Errorf("reported = %v; want 20.5", r)
	}
}

func TestResponseCoilHold(t *testing.T) {
	p, store := newProxy(config.Tag{ID: "CB", Area: config.AreaDiscrete, Address: 3,
		DataType: config.TypeBool, Bit: -1, Scale: 1})
	req := []byte{2, 0, 0, 0, 8} // read 8 discrete inputs from 0

	// bit 3 set (closed)
	p.onResponse(req, []byte{2, 1, 0b0000_1000})
	if r := reported(store, "CB"); r != 1 {
		t.Fatalf("reported = %v; want 1", r)
	}

	store.SetHold("CB", true)
	resp := []byte{2, 1, 0b0000_0000} // source now open (bit 3 clear)
	p.onResponse(req, resp)
	if resp[2]&0b0000_1000 == 0 {
		t.Errorf("bit 3 = 0; want 1 (frozen closed)")
	}
}

func TestRecordWriteOnStatus(t *testing.T) {
	// A coil command at address 10 with a matching discrete status tag.
	p, store := newProxy(
		config.Tag{ID: "CB", Area: config.AreaDiscrete, Address: 10, Scale: 1,
			StateText: map[string]string{"0": "Från", "1": "Till"}},
		config.Tag{ID: "CB.Cmd", Area: config.AreaCoil, Address: 10, Writable: true, Scale: 1},
	)
	// FC5 write single coil addr 10 = ON
	p.onRequest([]byte{5, 0, 10, 0xFF, 0x00})
	for _, v := range store.Snapshot() {
		if v.Key == "CB" {
			if v.LastCmd[:4] != "Till" {
				t.Errorf("lastCmd = %q; want Till", v.LastCmd)
			}
			return
		}
	}
	t.Error("status point CB not updated")
}

func TestBlockWrite(t *testing.T) {
	p, store := newProxy(
		config.Tag{ID: "CB", Area: config.AreaDiscrete, Address: 10, Scale: 1,
			StateText: map[string]string{"0": "Från", "1": "Till"}},
		config.Tag{ID: "CB.Cmd", Area: config.AreaCoil, Address: 10, Writable: true, Scale: 1},
	)
	// Not blocked: onRequest reports "do not block".
	if p.onRequest([]byte{5, 0, 10, 0xFF, 0x00}) {
		t.Fatal("unblocked write should forward")
	}
	// Block the status point; the write must now be rejected.
	store.SetBlock("CB", true)
	if !p.onRequest([]byte{5, 0, 10, 0xFF, 0x00}) {
		t.Fatal("blocked write should report block")
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
