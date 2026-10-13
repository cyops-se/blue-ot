// Package iec104 implements an IEC 60870-5-104 controlled station (slave).
//
// Profile: COT 2 octets, common address 2 octets, IOA 3 octets.
// Monitoring: M_SP_NA/TB, M_DP_NA/TB, M_ST_NA/TB, M_ME_NC/TF, M_EI_NA.
// Control: C_SC, C_DC, C_RC, C_SE_NC (with and without time tag, select and
// execute), C_IC_NA (station interrogation), C_RD_NA, C_CS_NA, C_TS_TA.
package iec104

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"time"

	"blue-station/internal/station"
)

// Type identifications.
const (
	M_SP_NA_1 = 1
	M_DP_NA_1 = 3
	M_ST_NA_1 = 5
	M_ME_NC_1 = 13
	M_SP_TB_1 = 30
	M_DP_TB_1 = 31
	M_ST_TB_1 = 32
	M_ME_TF_1 = 36
	C_SC_NA_1 = 45
	C_DC_NA_1 = 46
	C_RC_NA_1 = 47
	C_SE_NC_1 = 50
	C_SC_TA_1 = 58
	C_DC_TA_1 = 59
	C_RC_TA_1 = 60
	C_SE_TC_1 = 63
	M_EI_NA_1 = 70
	C_IC_NA_1 = 100
	C_RD_NA_1 = 102
	C_CS_NA_1 = 103
	C_TS_TA_1 = 107
)

// Causes of transmission.
const (
	CotSpont       = 3
	CotInit        = 4
	CotReq         = 5
	CotAct         = 6
	CotActCon      = 7
	CotDeact       = 8
	CotDeactCon    = 9
	CotActTerm     = 10
	CotInrogen     = 20
	CotUnknownType = 44
	CotUnknownCOT  = 45
	CotUnknownCA   = 46
	CotUnknownIOA  = 47

	cotNegative = 0x40
)

// U-format control functions.
const (
	uStartAct = 0x07
	uStartCon = 0x0B
	uStopAct  = 0x13
	uStopCon  = 0x23
	uTestAct  = 0x43
	uTestCon  = 0x83
)

const maxASDU = 249 // 253 octet APDU minus 4 control octets

type Server struct {
	CA         uint16
	K, W       int
	T1, T2, T3 time.Duration
	UTC        bool // time tags in UTC instead of local time
	Logf       func(format string, args ...any)

	st     *station.Station
	points []*station.Point
	byIOA  map[uint32]*station.Point
	byCtrl map[uint32]*station.Point
}

func New(st *station.Station, ca uint16) *Server {
	s := &Server{
		CA: ca, K: 12, W: 8,
		T1: 15 * time.Second, T2: 10 * time.Second, T3: 20 * time.Second,
		st: st, points: st.Points(),
		byIOA:  map[uint32]*station.Point{},
		byCtrl: map[uint32]*station.Point{},
	}
	for _, p := range s.points {
		if p.IOA != 0 {
			s.byIOA[p.IOA] = p
		}
		if p.CtrlIOA != 0 {
			s.byCtrl[p.CtrlIOA] = p
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
	ss := &session{srv: s, conn: conn, remote: conn.RemoteAddr().String(), lastRecv: time.Now()}
	s.logf("IEC 104: anslutning från %s", ss.remote)
	err := ss.run(ctx)
	conn.Close()
	s.logf("IEC 104: %s frånkopplad: %v", ss.remote, err)
}

type session struct {
	srv    *Server
	conn   net.Conn
	remote string

	started  bool
	initSent bool
	vs, vr   uint16 // next send / expected receive sequence number
	ackVS    uint16 // first send sequence number not yet acknowledged by the peer
	unacked  []time.Time

	recvUnacked int
	recvSince   time.Time
	lastRecv    time.Time
	testSince   time.Time

	queue   [][]byte // ASDUs waiting for the send window
	last    []station.Value
	lastVer uint64
}

func (ss *session) run(ctx context.Context) error {
	frames := make(chan []byte, 32)
	rerr := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			f, err := readAPDU(ss.conn)
			if err != nil {
				rerr <- err
				return
			}
			select {
			case frames <- f:
			case <-done:
				return
			}
		}
	}()

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-rerr:
			return err
		case f := <-frames:
			ss.lastRecv = time.Now()
			if err := ss.onFrame(f); err != nil {
				return err
			}
		case <-tick.C:
			if err := ss.onTimer(); err != nil {
				return err
			}
			if ss.started {
				ss.poll()
			}
		}
		if err := ss.flush(); err != nil {
			return err
		}
	}
}

// readAPDU returns the control field and ASDU of one APDU.
func readAPDU(r io.Reader) ([]byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	if h[0] != 0x68 || h[1] < 4 || h[1] > 253 {
		return nil, fmt.Errorf("ogiltig APDU-header % x", h)
	}
	b := make([]byte, h[1])
	_, err := io.ReadFull(r, b)
	return b, err
}

func (ss *session) write(b []byte) error {
	ss.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := ss.conn.Write(b)
	return err
}

func (ss *session) sendU(fn byte) error { return ss.write([]byte{0x68, 4, fn, 0, 0, 0}) }

func (ss *session) sendS() error {
	ss.recvUnacked = 0
	return ss.write([]byte{0x68, 4, 0x01, 0, byte(ss.vr << 1), byte(ss.vr >> 7)})
}

func (ss *session) sendI(asdu []byte) error {
	b := make([]byte, 6+len(asdu))
	b[0], b[1] = 0x68, byte(4+len(asdu))
	b[2], b[3] = byte(ss.vs<<1), byte(ss.vs>>7)
	b[4], b[5] = byte(ss.vr<<1), byte(ss.vr>>7)
	copy(b[6:], asdu)
	ss.vs = (ss.vs + 1) & 0x7fff
	ss.unacked = append(ss.unacked, time.Now())
	ss.recvUnacked = 0
	return ss.write(b)
}

func (ss *session) flush() error {
	for ss.started && len(ss.queue) > 0 && len(ss.unacked) < ss.srv.K {
		a := ss.queue[0]
		ss.queue = ss.queue[1:]
		if err := ss.sendI(a); err != nil {
			return err
		}
	}
	return nil
}

func (ss *session) ack(nr uint16) error {
	n := int((nr - ss.ackVS) & 0x7fff)
	if n > len(ss.unacked) {
		return fmt.Errorf("ogiltig N(R)=%d", nr)
	}
	ss.unacked = ss.unacked[n:]
	ss.ackVS = nr
	return nil
}

func (ss *session) onFrame(f []byte) error {
	seq := func(i int) uint16 { return binary.LittleEndian.Uint16(f[i:]) >> 1 }
	switch {
	case f[0]&0x01 == 0: // I-format
		if ns := seq(0); ns != ss.vr {
			return fmt.Errorf("sekvensfel: N(S)=%d, förväntat %d", ns, ss.vr)
		}
		ss.vr = (ss.vr + 1) & 0x7fff
		if err := ss.ack(seq(2)); err != nil {
			return err
		}
		if ss.recvUnacked == 0 {
			ss.recvSince = time.Now()
		}
		ss.recvUnacked++
		ss.handleASDU(f[4:])
		if ss.recvUnacked >= ss.srv.W {
			return ss.sendS()
		}
	case f[0]&0x03 == 0x01: // S-format
		return ss.ack(seq(2))
	default: // U-format
		switch f[0] {
		case uStartAct:
			ss.started = true
			ss.last, ss.lastVer = ss.srv.st.Snapshot()
			if err := ss.sendU(uStartCon); err != nil {
				return err
			}
			if !ss.initSent {
				ss.initSent = true
				ss.queue = append(ss.queue, append(ss.header(M_EI_NA_1, 1, CotInit), 0, 0, 0, 0))
			}
		case uStopAct:
			ss.started = false
			if ss.recvUnacked > 0 {
				if err := ss.sendS(); err != nil {
					return err
				}
			}
			return ss.sendU(uStopCon)
		case uTestAct:
			return ss.sendU(uTestCon)
		case uTestCon:
			ss.testSince = time.Time{}
		default:
			return fmt.Errorf("okänd U-ram 0x%02x", f[0])
		}
	}
	return nil
}

func (ss *session) onTimer() error {
	now := time.Now()
	t1 := ss.srv.T1
	if len(ss.unacked) > 0 && now.Sub(ss.unacked[0]) > t1 {
		return fmt.Errorf("t1 löpte ut i väntan på kvittens")
	}
	if !ss.testSince.IsZero() && now.Sub(ss.testSince) > t1 {
		return fmt.Errorf("t1 löpte ut i väntan på TESTFR con")
	}
	if ss.recvUnacked > 0 && now.Sub(ss.recvSince) >= ss.srv.T2 {
		if err := ss.sendS(); err != nil {
			return err
		}
	}
	if ss.testSince.IsZero() && now.Sub(ss.lastRecv) >= ss.srv.T3 {
		ss.testSince = now
		return ss.sendU(uTestAct)
	}
	return nil
}

func (ss *session) header(typeID byte, n int, cot byte) []byte {
	ca := ss.srv.CA
	return []byte{typeID, byte(n), cot, 0, byte(ca), byte(ca >> 8)}
}

// enqueue packs information objects (SQ=0) into as few ASDUs as possible.
func (ss *session) enqueue(typeID, cot byte, objs [][]byte) {
	var cur []byte
	n := 0
	for _, o := range objs {
		if n > 0 && (6+len(cur)+len(o) > maxASDU || n == 127) {
			ss.queue = append(ss.queue, append(ss.header(typeID, n, cot), cur...))
			cur, n = nil, 0
		}
		cur = append(cur, o...)
		n++
	}
	if n > 0 {
		ss.queue = append(ss.queue, append(ss.header(typeID, n, cot), cur...))
	}
}

func (ss *session) handleASDU(raw []byte) {
	if len(raw) < 6 {
		return
	}
	typeID, cot, ca, body := raw[0], raw[2]&0x3f, binary.LittleEndian.Uint16(raw[4:]), raw[6:]

	reply := func(c byte, negative bool) {
		r := append([]byte(nil), raw...)
		r[2] = c | raw[2]&0x80
		if negative {
			r[2] |= cotNegative
		}
		binary.LittleEndian.PutUint16(r[4:], ss.srv.CA)
		ss.queue = append(ss.queue, r)
	}

	if ca != ss.srv.CA && ca != 0xFFFF {
		reply(CotUnknownCA, true)
		return
	}

	switch typeID {
	case C_IC_NA_1:
		ss.interrogation(cot, body, reply)
	case C_SC_NA_1, C_DC_NA_1, C_RC_NA_1, C_SE_NC_1, C_SC_TA_1, C_DC_TA_1, C_RC_TA_1, C_SE_TC_1:
		ss.command(typeID, cot, body, reply)
	case C_RD_NA_1:
		if cot != CotReq {
			reply(CotUnknownCOT, true)
			return
		}
		p := ss.srv.byIOA[ioaOf(body)]
		if len(body) < 3 || p == nil {
			reply(CotUnknownIOA, true)
			return
		}
		vals, _ := ss.srv.st.Snapshot()
		t, o := ss.srv.object(p, vals[p.Index], false)
		ss.enqueue(t, CotReq, [][]byte{o})
	case C_CS_NA_1, C_TS_TA_1:
		// Clock synchronisation is acknowledged; the simulator keeps the host clock.
		if cot != CotAct {
			reply(CotUnknownCOT, true)
			return
		}
		reply(CotActCon, false)
	default:
		reply(CotUnknownType, true)
	}
}

func (ss *session) interrogation(cot byte, body []byte, reply func(byte, bool)) {
	switch {
	case cot == CotDeact:
		reply(CotDeactCon, false)
		return
	case cot != CotAct:
		reply(CotUnknownCOT, true)
		return
	case len(body) < 4 || ioaOf(body) != 0:
		reply(CotUnknownIOA, true)
		return
	}
	qoi := body[3]
	reply(CotActCon, false)
	if qoi == CotInrogen {
		vals, ver := ss.srv.st.Snapshot()
		groups := map[byte][][]byte{}
		var order []byte
		for _, p := range ss.srv.points {
			if p.IOA == 0 {
				continue
			}
			t, o := ss.srv.object(p, vals[p.Index], false)
			if groups[t] == nil {
				order = append(order, t)
			}
			groups[t] = append(groups[t], o)
		}
		for _, t := range order {
			ss.enqueue(t, CotInrogen, groups[t])
		}
		ss.last, ss.lastVer = vals, ver
	}
	reply(CotActTerm, false)
}

func (ss *session) command(typeID, cot byte, body []byte, reply func(byte, bool)) {
	base, elemLen := typeID, 1
	switch typeID {
	case C_SC_TA_1:
		base = C_SC_NA_1
	case C_DC_TA_1:
		base = C_DC_NA_1
	case C_RC_TA_1:
		base = C_RC_NA_1
	case C_SE_TC_1:
		base = C_SE_NC_1
	}
	if base == C_SE_NC_1 {
		elemLen = 5
	}
	if base != typeID {
		elemLen += 7 // CP56Time2a
	}
	if len(body) < 3+elemLen {
		reply(CotActCon, true)
		return
	}
	if cot == CotDeact {
		reply(CotDeactCon, false)
		return
	}
	if cot != CotAct {
		reply(CotUnknownCOT, true)
		return
	}

	p := ss.srv.byCtrl[ioaOf(body)]
	e := body[3:]
	var c station.Ctrl
	var kindOK, valid = false, true
	var sel bool
	switch base {
	case C_SC_NA_1:
		kindOK = p != nil && (p.Kind == station.KindSwitch || p.Kind == station.KindPulse)
		c.On, sel = e[0]&0x01 != 0, e[0]&0x80 != 0
	case C_DC_NA_1:
		kindOK = p != nil && p.Kind == station.KindBreaker
		dcs := e[0] & 0x03
		valid, c.On, sel = dcs == 1 || dcs == 2, dcs == 2, e[0]&0x80 != 0
	case C_RC_NA_1:
		kindOK = p != nil && p.Kind == station.KindStep
		rcs := e[0] & 0x03
		valid, c.On, sel = rcs == 1 || rcs == 2, rcs == 2, e[0]&0x80 != 0
	case C_SE_NC_1:
		kindOK = p != nil && p.Kind == station.KindSetpoint
		c.F = float64(math.Float32frombits(binary.LittleEndian.Uint32(e)))
		sel = e[4]&0x80 != 0
	}
	if !kindOK {
		reply(CotUnknownIOA, true)
		return
	}
	if !valid {
		reply(CotActCon, true)
		return
	}
	if sel {
		reply(CotActCon, false)
		return
	}
	if err := ss.srv.st.Control(p, c, "IEC104 "+ss.remote); err != nil {
		reply(CotActCon, true)
		return
	}
	reply(CotActCon, false)
	reply(CotActTerm, false)
}

// poll queues spontaneous, time-tagged information for changed points.
func (ss *session) poll() {
	vals, ver := ss.srv.st.Snapshot()
	if ver == ss.lastVer {
		return
	}
	ss.lastVer = ver
	groups := map[byte][][]byte{}
	var order []byte
	for i, p := range ss.srv.points {
		if p.IOA == 0 {
			continue
		}
		v, l := vals[i], ss.last[i]
		var changed bool
		switch p.Kind {
		case station.KindBreaker:
			changed = v.DP != l.DP
		case station.KindSwitch, station.KindStatus:
			changed = v.B != l.B
		case station.KindStep:
			changed = v.F != l.F
		default:
			changed = v.F != l.F && math.Abs(v.F-l.F) >= p.Deadband
		}
		if !changed {
			continue
		}
		ss.last[i] = v
		t, o := ss.srv.object(p, v, true)
		if groups[t] == nil {
			order = append(order, t)
		}
		groups[t] = append(groups[t], o)
	}
	for _, t := range order {
		ss.enqueue(t, CotSpont, groups[t])
	}
}

// object encodes one information object, with or without CP56Time2a.
func (s *Server) object(p *station.Point, v station.Value, timed bool) (byte, []byte) {
	o := []byte{byte(p.IOA), byte(p.IOA >> 8), byte(p.IOA >> 16)}
	var na, tb byte
	switch p.Kind {
	case station.KindBreaker:
		na, tb = M_DP_NA_1, M_DP_TB_1
		o = append(o, v.DP&0x03)
	case station.KindSwitch, station.KindStatus:
		na, tb = M_SP_NA_1, M_SP_TB_1
		var siq byte
		if v.B {
			siq = 1
		}
		o = append(o, siq)
	case station.KindStep:
		na, tb = M_ST_NA_1, M_ST_TB_1
		o = append(o, byte(int8(v.F))&0x7f, 0)
	default:
		na, tb = M_ME_NC_1, M_ME_TF_1
		o = binary.LittleEndian.AppendUint32(o, math.Float32bits(float32(v.F)))
		o = append(o, 0)
	}
	if !timed {
		return na, o
	}
	return tb, append(o, s.cp56(v.T)...)
}

func (s *Server) cp56(t time.Time) []byte {
	if s.UTC {
		t = t.UTC()
	} else {
		t = t.Local()
	}
	ms := t.Second()*1000 + t.Nanosecond()/1e6
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return []byte{
		byte(ms), byte(ms >> 8),
		byte(t.Minute()),
		byte(t.Hour()),
		byte(t.Day()) | byte(wd<<5),
		byte(t.Month()),
		byte(t.Year()-2000) & 0x7f,
	}
}

func ioaOf(b []byte) uint32 {
	if len(b) < 3 {
		return 0
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
}
