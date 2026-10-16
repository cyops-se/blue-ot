package iec104

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"blue-wwtp/internal/plant"
)

type master struct {
	t      *testing.T
	conn   net.Conn
	vs, vr uint16
}

func (m *master) read() []byte {
	m.t.Helper()
	m.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	f, err := readAPDU(m.conn)
	if err != nil {
		m.t.Fatal(err)
	}
	if f[0]&1 == 0 {
		m.vr++
	}
	return f
}

// readASDU returns the next I-frame ASDU, skipping S/U frames.
func (m *master) readASDU() []byte {
	for {
		if f := m.read(); f[0]&1 == 0 {
			return f[4:]
		}
	}
}

func (m *master) sendI(asdu ...byte) {
	b := append([]byte{0x68, byte(4 + len(asdu)), byte(m.vs << 1), byte(m.vs >> 7), byte(m.vr << 1), byte(m.vr >> 7)}, asdu...)
	m.vs++
	if _, err := m.conn.Write(b); err != nil {
		m.t.Fatal(err)
	}
}

func start(t *testing.T) (*plant.Plant, *master) {
	st := plant.New(plant.DefaultConfig(), 1)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go New(st, 1).Serve(ctx, l)
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	m := &master{t: t, conn: conn}
	conn.Write([]byte{0x68, 4, uStartAct, 0, 0, 0})
	if f := m.read(); f[0] != uStartCon {
		t.Fatalf("förväntade STARTDT con: % x", f)
	}
	if a := m.readASDU(); a[0] != M_EI_NA_1 {
		t.Fatalf("förväntade M_EI_NA_1: % x", a)
	}
	return st, m
}

func TestInterrogation(t *testing.T) {
	st, m := start(t)
	m.sendI(C_IC_NA_1, 1, CotAct, 0, 1, 0, 0, 0, 0, 20)
	if a := m.readASDU(); a[0] != C_IC_NA_1 || a[2] != CotActCon {
		t.Fatalf("förväntade actcon: % x", a)
	}
	want := 0
	for _, p := range st.Points() {
		if p.IOA != 0 {
			want++
		}
	}
	got := 0
	for {
		a := m.readASDU()
		if a[0] == C_IC_NA_1 {
			if a[2] != CotActTerm {
				t.Fatalf("förväntade actterm: % x", a)
			}
			break
		}
		if a[2] != CotInrogen {
			t.Fatalf("förväntade COT 20: % x", a)
		}
		got += int(a[1] & 0x7f)
	}
	if got != want {
		t.Fatalf("GI gav %d objekt, förväntat %d", got, want)
	}
}

func TestDoubleCommand(t *testing.T) {
	st, m := start(t)
	p := st.PointByID("Supply.CB")
	ioa := p.CtrlIOA
	// Open the supply breaker with direct execute.
	m.sendI(C_DC_NA_1, 1, CotAct, 0, 1, 0, byte(ioa), byte(ioa>>8), byte(ioa>>16), 1)
	if a := m.readASDU(); a[0] != C_DC_NA_1 || a[2] != CotActCon {
		t.Fatalf("förväntade positiv actcon: % x", a)
	}
	if a := m.readASDU(); a[2] != CotActTerm {
		t.Fatalf("förväntade actterm: % x", a)
	}
	// Expect a spontaneous M_DP_TB_1 with state OFF for the status IOA.
	for {
		a := m.readASDU()
		if a[0] != M_DP_TB_1 || a[2] != CotSpont {
			continue
		}
		for i, n := 0, int(a[1]&0x7f); i < n; i++ {
			o := a[6+i*11:]
			if ioaOf(o) == p.IOA {
				if o[3] != 1 {
					t.Fatalf("Supply.CB ska vara FRÅN, DIQ=%d", o[3])
				}
				return
			}
		}
	}
}

func TestNegativeResponses(t *testing.T) {
	st, m := start(t)
	m.sendI(C_DC_NA_1, 1, CotAct, 0, 1, 0, 0x39, 0x30, 0, 1) // IOA 12345
	if a := m.readASDU(); a[2] != CotUnknownIOA|cotNegative {
		t.Fatalf("förväntade COT 47 negativ: % x", a)
	}
	m.sendI(C_IC_NA_1, 1, CotAct, 0, 9, 0, 0, 0, 0, 20)
	if a := m.readASDU(); a[2] != CotUnknownCA|cotNegative {
		t.Fatalf("förväntade COT 46 negativ: % x", a)
	}
	f := float32(10)
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, math.Float32bits(f))
	ioa := st.PointByID("Rain.Intensity").CtrlIOA
	m.sendI(append([]byte{C_SE_NC_1, 1, CotAct, 0, 1, 0, byte(ioa), byte(ioa >> 8), 0}, append(b, 0)...)...) // rain = 10 mm/h
	if a := m.readASDU(); a[2] != CotActCon {
		t.Fatalf("förväntade positiv actcon för börvärde: % x", a)
	}
}
