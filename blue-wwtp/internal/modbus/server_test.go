package modbus

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"testing"

	"blue-wwtp/internal/plant"
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

func start(t *testing.T) (*plant.Plant, *client) {
	st := plant.New(plant.DefaultConfig(), 1)
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
	reg := func(id string) (byte, byte) {
		a := st.PointByID(id).Modbus
		return byte(a >> 8), byte(a)
	}

	// Coil 0: incoming supply breaker, closed at start.
	r := c.do(1, 0, 0, 0, 1)
	if r[0] != 1 || r[1] != 1 || r[2] != 1 {
		t.Fatalf("FC1: % x", r)
	}

	// A measurement within its normal range.
	hi, lo := reg("Bio.DO")
	r = c.do(4, hi, lo, 0, 2)
	if p := float(r[2:]); p < 0.5 || p > 5 {
		t.Fatalf("Bio.DO = %.2f", p)
	}

	// Open the supply breaker (coil 0) and check discrete input 0.
	if r = c.do(5, 0, 0, 0, 0); r[0] != 5 {
		t.Fatalf("FC5: % x", r)
	}
	if r = c.do(2, 0, 0, 0, 1); r[2] != 0 {
		t.Fatalf("Supply.CB ska vara frånslagen: % x", r)
	}

	// A setpoint.
	hi, lo = reg("Bio.DOSet")
	bits := math.Float32bits(2.5)
	if r = c.do(16, hi, lo, 0, 2, 4, byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits)); r[0] != 16 {
		t.Fatalf("FC16: % x", r)
	}
	r = c.do(3, hi, lo, 0, 2)
	if v := float(r[2:]); v != 2.5 {
		t.Fatalf("Bio.DOSet = %v", v)
	}

	// Out-of-range setpoint and misaligned write give exceptions.
	bits = math.Float32bits(50)
	if r = c.do(16, hi, lo, 0, 2, 4, byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits)); r[0] != 0x90 || r[1] != 4 {
		t.Fatalf("förväntade undantag 4: % x", r)
	}
	if r = c.do(16, 0, 1, 0, 2, 4, 0, 0, 0, 0); r[0] != 0x90 || r[1] != 2 {
		t.Fatalf("förväntade undantag 2: % x", r)
	}
	if r = c.do(4, 0x10, 0, 0, 1); r[0] != 0x84 || r[1] != 2 {
		t.Fatalf("förväntade undantag 2: % x", r)
	}
}
