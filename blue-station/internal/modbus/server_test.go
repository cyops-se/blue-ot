package modbus

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"testing"

	"blue-station/internal/station"
)

type client struct {
	t    *testing.T
	conn net.Conn
	tid  uint16
}

func (c *client) do(pdu ...byte) []byte {
	c.t.Helper()
	c.tid++
	req := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(req[0:], c.tid)
	binary.BigEndian.PutUint16(req[4:], uint16(len(pdu)+1))
	req[6] = 1
	copy(req[7:], pdu)
	if _, err := c.conn.Write(req); err != nil {
		c.t.Fatal(err)
	}
	hdr := make([]byte, 7)
	if _, err := io.ReadFull(c.conn, hdr); err != nil {
		c.t.Fatal(err)
	}
	if binary.BigEndian.Uint16(hdr) != c.tid {
		c.t.Fatal("fel transaktions-id")
	}
	resp := make([]byte, binary.BigEndian.Uint16(hdr[4:])-1)
	if _, err := io.ReadFull(c.conn, resp); err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func start(t *testing.T) (*station.Station, *client) {
	st := station.New(station.DefaultConfig(), 1)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go New(st).Serve(ctx, l)
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return st, &client{t: t, conn: conn}
}

func float(r []byte) float64 { return float64(math.Float32frombits(binary.BigEndian.Uint32(r))) }

func TestReadWrite(t *testing.T) {
	st, c := start(t)

	// Coils 0-9: breakers, all closed except the bus coupler (5).
	r := c.do(1, 0, 0, 0, 10)
	if r[0] != 1 || r[1] != 2 || r[2] != 0b11011111 || r[3] != 0b11 {
		t.Fatalf("FC1: % x", r)
	}

	// Grid voltage, IR 0-1.
	r = c.do(4, 0, 0, 0, 2)
	if u := float(r[2:]); u < 200 || u > 240 {
		t.Fatalf("Grid.U = %.1f", u)
	}

	// Open F1 (coil 6) and check discrete input 6.
	if r = c.do(5, 0, 6, 0, 0); r[0] != 5 {
		t.Fatalf("FC5: % x", r)
	}
	if r = c.do(2, 0, 6, 0, 1); r[2] != 0 {
		t.Fatalf("F1 ska vara frånslagen: % x", r)
	}

	// Voltage setpoint AVR.USet.
	p := st.PointByID("AVR.USet")
	a := uint16(p.Modbus)
	bits := math.Float32bits(21.0)
	if r = c.do(16, byte(a>>8), byte(a), 0, 2, 4, byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits)); r[0] != 16 {
		t.Fatalf("FC16: % x", r)
	}
	r = c.do(3, byte(a>>8), byte(a), 0, 2)
	if v := float(r[2:]); v != 21 {
		t.Fatalf("AVR.USet = %v", v)
	}

	// Out-of-range setpoint and misaligned write give exceptions.
	bits = math.Float32bits(99)
	if r = c.do(16, byte(a>>8), byte(a), 0, 2, 4, byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits)); r[0] != 0x90 || r[1] != 4 {
		t.Fatalf("förväntade undantag 4: % x", r)
	}
	if r = c.do(16, 0, 1, 0, 2, 4, 0, 0, 0, 0); r[0] != 0x90 || r[1] != 2 {
		t.Fatalf("förväntade undantag 2: % x", r)
	}
	if r = c.do(4, 0x10, 0, 0, 1); r[0] != 0x84 || r[1] != 2 {
		t.Fatalf("förväntade undantag 2: % x", r)
	}
}
