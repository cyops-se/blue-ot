// Package modbus implements a Modbus TCP server that exposes the plant points.
//
// Supported function codes: 1, 2, 3, 4, 5, 15, 16. Analog values are IEEE 754
// float32 in two consecutive registers, high word first (big endian, "ABCD").
// Float setpoints must be written as register pairs with FC16.
package modbus

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"time"

	"blue-wwtp/internal/plant"
)

const (
	excIllegalFunction = 1
	excIllegalAddress  = 2
	excIllegalValue    = 3
	excDeviceFailure   = 4
)

type Server struct {
	Logf func(format string, args ...any)

	st                               *plant.Plant
	coils, discrete, input, holding  map[uint16]*plant.Point
	nCoils, nDiscrete, nInput, nHold int
}

func New(st *plant.Plant) *Server {
	s := &Server{
		st:       st,
		coils:    map[uint16]*plant.Point{},
		discrete: map[uint16]*plant.Point{},
		input:    map[uint16]*plant.Point{},
		holding:  map[uint16]*plant.Point{},
	}
	for _, p := range st.Points() {
		a := uint16(p.Modbus)
		switch p.Kind {
		case plant.KindBreaker, plant.KindSwitch:
			s.coils[a], s.discrete[a] = p, p
			s.nCoils = max(s.nCoils, p.Modbus+1)
			s.nDiscrete = max(s.nDiscrete, p.Modbus+1)
		case plant.KindPulse:
			s.coils[a] = p
			s.nCoils = max(s.nCoils, p.Modbus+1)
		case plant.KindStatus:
			s.discrete[a] = p
			s.nDiscrete = max(s.nDiscrete, p.Modbus+1)
		case plant.KindMeasurement, plant.KindStep:
			s.input[a] = p
			s.nInput = max(s.nInput, p.Modbus+2)
		case plant.KindSetpoint:
			s.holding[a] = p
			s.nHold = max(s.nHold, p.Modbus+2)
		}
	}
	return s
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, l)
}

func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		l.Close()
	}()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handle(ctx, c)
	}
}

func (s *Server) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	src := "Modbus " + conn.RemoteAddr().String()
	s.logf("Modbus: anslutning från %s", conn.RemoteAddr())
	defer s.logf("Modbus: %s frånkopplad", conn.RemoteAddr())

	hdr := make([]byte, 7)
	for {
		conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return
		}
		length := binary.BigEndian.Uint16(hdr[4:])
		if binary.BigEndian.Uint16(hdr[2:]) != 0 || length < 2 || length > 254 {
			return
		}
		pdu := make([]byte, length-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			return
		}
		resp := s.Process(pdu, src)
		out := make([]byte, 7+len(resp))
		copy(out, hdr[:4])
		binary.BigEndian.PutUint16(out[4:], uint16(len(resp)+1))
		out[6] = hdr[6] // unit id is echoed, all unit ids are accepted
		copy(out[7:], resp)
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := conn.Write(out); err != nil {
			return
		}
	}
}

// Process handles one request PDU and returns the response PDU.
func (s *Server) Process(pdu []byte, src string) []byte {
	fc, d := pdu[0], pdu[1:]
	exc := func(code byte) []byte { return []byte{fc | 0x80, code} }
	u16 := func(i int) int { return int(binary.BigEndian.Uint16(d[i:])) }

	switch fc {
	case 1, 2:
		if len(d) != 4 {
			return exc(excIllegalValue)
		}
		m, size := s.coils, s.nCoils
		if fc == 2 {
			m, size = s.discrete, s.nDiscrete
		}
		start, qty := u16(0), u16(2)
		if qty < 1 || qty > 2000 {
			return exc(excIllegalValue)
		}
		if start+qty > size {
			return exc(excIllegalAddress)
		}
		vals, _ := s.st.Snapshot()
		nb := (qty + 7) / 8
		out := make([]byte, 2+nb)
		out[0], out[1] = fc, byte(nb)
		for i := 0; i < qty; i++ {
			if p := m[uint16(start+i)]; p != nil && bitValue(p, vals[p.Index]) {
				out[2+i/8] |= 1 << (i % 8)
			}
		}
		return out

	case 3, 4:
		if len(d) != 4 {
			return exc(excIllegalValue)
		}
		m, size := s.holding, s.nHold
		if fc == 4 {
			m, size = s.input, s.nInput
		}
		start, qty := u16(0), u16(2)
		if qty < 1 || qty > 125 {
			return exc(excIllegalValue)
		}
		if start+qty > size {
			return exc(excIllegalAddress)
		}
		vals, _ := s.st.Snapshot()
		out := make([]byte, 2+2*qty)
		out[0], out[1] = fc, byte(2*qty)
		for i := 0; i < qty; i++ {
			a := uint16(start + i)
			var reg uint16
			if p := m[a]; p != nil {
				reg = uint16(math.Float32bits(float32(vals[p.Index].F)) >> 16)
			} else if p := m[a-1]; p != nil {
				reg = uint16(math.Float32bits(float32(vals[p.Index].F)))
			}
			binary.BigEndian.PutUint16(out[2+2*i:], reg)
		}
		return out

	case 5:
		if len(d) != 4 {
			return exc(excIllegalValue)
		}
		v := u16(2)
		if v != 0xFF00 && v != 0 {
			return exc(excIllegalValue)
		}
		p := s.coils[uint16(u16(0))]
		if p == nil {
			return exc(excIllegalAddress)
		}
		if err := s.st.Control(p, plant.Ctrl{On: v == 0xFF00}, src); err != nil {
			return exc(excDeviceFailure)
		}
		return pdu

	case 15:
		if len(d) < 5 {
			return exc(excIllegalValue)
		}
		start, qty, bc := u16(0), u16(2), int(d[4])
		if qty < 1 || qty > 1968 || bc != (qty+7)/8 || len(d) != 5+bc {
			return exc(excIllegalValue)
		}
		if start+qty > s.nCoils {
			return exc(excIllegalAddress)
		}
		vals, _ := s.st.Snapshot()
		var failed error
		for i := 0; i < qty; i++ {
			p := s.coils[uint16(start+i)]
			if p == nil {
				continue
			}
			on := d[5+i/8]&(1<<(i%8)) != 0
			// Only act on changes so that writing back a read image is harmless.
			if p.Kind != plant.KindPulse && on == bitValue(p, vals[p.Index]) {
				continue
			}
			if err := s.st.Control(p, plant.Ctrl{On: on}, src); err != nil {
				failed = errors.Join(failed, err)
			}
		}
		if failed != nil {
			return exc(excDeviceFailure)
		}
		return pdu[:5]

	case 16:
		if len(d) < 5 {
			return exc(excIllegalValue)
		}
		start, qty, bc := u16(0), u16(2), int(d[4])
		if qty < 1 || qty > 123 || bc != 2*qty || len(d) != 5+bc {
			return exc(excIllegalValue)
		}
		if start+qty > s.nHold {
			return exc(excIllegalAddress)
		}
		// Every float must be written completely.
		if s.holding[uint16(start)] == nil || s.holding[uint16(start+qty-1)] != nil {
			return exc(excIllegalAddress)
		}
		for a := start; a < start+qty; a += 2 {
			p := s.holding[uint16(a)]
			bits := binary.BigEndian.Uint32(d[5+2*(a-start):])
			if err := s.st.Control(p, plant.Ctrl{F: float64(math.Float32frombits(bits))}, src); err != nil {
				return exc(excDeviceFailure)
			}
		}
		return pdu[:5]
	}
	return exc(excIllegalFunction)
}

func bitValue(p *plant.Point, v plant.Value) bool {
	if p.Kind == plant.KindBreaker {
		return v.DP == 2
	}
	return v.B
}
