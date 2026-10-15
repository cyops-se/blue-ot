// Package iec polls a device over IEC 60870-5-104 as a controlling station
// (master): it connects, performs STARTDT and a general interrogation,
// receives monitoring ASDUs and feeds them to the hub, and executes commands.
// The Worker mirrors mbus.Worker so the comm package can treat both alike.
package iec

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"time"

	"blueot/scada/internal/model"
	"blueot/scada/internal/realtime"
)

// Monitoring type identifications.
const (
	mSPNA = 1
	mDPNA = 3
	mSTNA = 5
	mMENA = 9
	mMENB = 11
	mMENC = 13
	mSPTB = 30
	mDPTB = 31
	mSTTB = 32
	mMETD = 34
	mMETE = 35
	mMETF = 36
)

// Command and system type identifications.
const (
	cSCNA = 45
	cDCNA = 46
	cRCNA = 47
	cSENC = 50
	cICNA = 100
)

// Causes of transmission.
const (
	cotAct      = 6
	cotActCon   = 7
	cotActTerm  = 10
	cotInrogen  = 20
	cotNegative = 0x40
)

// U-frame functions.
const (
	uStartAct = 0x07
	uStartCon = 0x0B
	uStopAct  = 0x13
	uTestAct  = 0x43
	uTestCon  = 0x83
)

type cmdReq struct {
	tag   *model.Tag
	value float64
	reply chan error
}

// Worker polls one IEC 104 device.
type Worker struct {
	dev      model.Device
	tags     []model.Tag
	byID     map[string]*model.Tag
	monByIOA map[uint32]*model.Tag
	hub      *realtime.Hub
	ca       uint16
	timeout  time.Duration

	cmdCh  chan cmdReq
	cancel context.CancelFunc
	done   chan struct{}

	vs, vr uint16
}

// NewWorker builds an IEC 104 worker for a device and its tags.
func NewWorker(d model.Device, tags []model.Tag, hub *realtime.Hub) *Worker {
	w := &Worker{
		dev: d, tags: tags, hub: hub, ca: uint16(d.CA),
		timeout:  time.Duration(d.TimeoutMs) * time.Millisecond,
		byID:     map[string]*model.Tag{},
		monByIOA: map[uint32]*model.Tag{},
		cmdCh:    make(chan cmdReq),
		done:     make(chan struct{}),
	}
	for i := range w.tags {
		t := &w.tags[i]
		w.byID[t.ID] = t
		if t.IOA != 0 {
			w.monByIOA[t.IOA] = t
		}
	}
	return w
}

// Start begins polling in the background.
func (w *Worker) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.run(ctx)
}

// Stop stops polling and waits for the worker to finish.
func (w *Worker) Stop() {
	w.cancel()
	<-w.done
}

// Write executes a command on a writable tag.
func (w *Worker) Write(t *model.Tag, value float64) error {
	if t.CtrlIOA == 0 {
		return fmt.Errorf("taggen %q saknar styr-IOA", t.ID)
	}
	req := cmdReq{tag: t, value: value, reply: make(chan error, 1)}
	select {
	case w.cmdCh <- req:
	case <-time.After(w.timeout):
		return fmt.Errorf("enheten %q är inte ansluten", w.dev.ID)
	}
	select {
	case err := <-req.reply:
		return err
	case <-time.After(w.timeout + 3*time.Second):
		return fmt.Errorf("inget svar på kommandot")
	}
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := w.session(ctx)
		w.markDisconnected(err)
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (w *Worker) markDisconnected(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	now := time.Now()
	if w.hub.SetDevice(model.DeviceStatus{Device: w.dev.ID, Connected: false, Error: msg, LastPoll: now}) {
		w.hub.AddEvent("comm", "alarm", w.dev.Name, "Kommunikationsfel: "+msg, "")
	}
	vals := make([]model.Value, 0, len(w.tags))
	for _, t := range w.tags {
		vals = append(vals, model.Value{Tag: t.ID, Quality: model.QualityBad, TS: now})
	}
	w.hub.Update(vals)
}

// session runs one connection: connect, STARTDT, GI, then the I/O loop.
func (w *Worker) session(ctx context.Context) error {
	d := net.Dialer{Timeout: w.timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", w.dev.Host, w.dev.Port))
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	w.vs, w.vr = 0, 0

	recv := make(chan []byte, 64)
	readErr := make(chan error, 1)
	go reader(conn, recv, readErr)

	if err := w.writeU(conn, uStartAct); err != nil {
		return err
	}
	// Wait for STARTDT con.
	deadline := time.Now().Add(w.timeout)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return err
		case f := <-recv:
			if isU(f) {
				if f[2] == uStartCon {
					goto started
				}
				continue
			}
			if isI(f) {
				w.vr++
			}
		case <-time.After(time.Until(deadline)):
			return fmt.Errorf("ingen STARTDT-bekräftelse")
		}
	}
started:
	if err := w.sendInterrogation(conn); err != nil {
		return err
	}
	if w.hub.SetDevice(model.DeviceStatus{Device: w.dev.ID, Connected: true, LastPoll: time.Now()}) {
		w.hub.AddEvent("comm", "info", w.dev.Name, "Kommunikation OK", "")
	}
	return w.loop(ctx, conn, recv, readErr)
}

type pending struct {
	base  byte
	ioa   uint32
	reply chan error
}

func (w *Worker) loop(ctx context.Context, conn net.Conn, recv <-chan []byte, readErr <-chan error) error {
	ackTicker := time.NewTicker(500 * time.Millisecond)
	defer ackTicker.Stop()
	testTicker := time.NewTicker(20 * time.Second)
	defer testTicker.Stop()
	giTicker := time.NewTicker(time.Duration(w.dev.PollMs) * time.Millisecond)
	defer giTicker.Stop()

	var unacked int
	var pend *pending
	var pendC <-chan time.Time

	clearPend := func(err error) {
		if pend != nil {
			pend.reply <- err
			pend = nil
			pendC = nil
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			clearPend(fmt.Errorf("förbindelsen bröts"))
			return err
		case f := <-recv:
			switch {
			case isU(f):
				if f[2] == uTestAct {
					if err := w.writeU(conn, uTestCon); err != nil {
						return err
					}
				}
			case isS(f):
				// peer acknowledging our I-frames; nothing to do
			case isI(f):
				w.vr++
				unacked++
				w.onASDU(f[6:], &pend, &pendC)
				if unacked >= 8 {
					if err := w.sendS(conn); err != nil {
						return err
					}
					unacked = 0
				}
			}
		case req := <-w.cmdCh:
			if pend != nil {
				req.reply <- fmt.Errorf("en manöver pågår redan")
				continue
			}
			base, ioa, asdu, err := w.buildCommand(req.tag, req.value)
			if err != nil {
				req.reply <- err
				continue
			}
			if err := w.sendI(conn, asdu); err != nil {
				req.reply <- err
				return err
			}
			pend = &pending{base: base, ioa: ioa, reply: req.reply}
			pendC = time.After(w.timeout + 2*time.Second)
		case <-pendC:
			clearPend(fmt.Errorf("inget svar på manövern (timeout)"))
		case <-ackTicker.C:
			if unacked > 0 {
				if err := w.sendS(conn); err != nil {
					return err
				}
				unacked = 0
			}
		case <-testTicker.C:
			if err := w.writeU(conn, uTestAct); err != nil {
				return err
			}
		case <-giTicker.C:
			if err := w.sendInterrogation(conn); err != nil {
				return err
			}
		}
	}
}

// onASDU processes one incoming ASDU: monitoring values go to the hub, command
// confirmations resolve a pending command.
func (w *Worker) onASDU(asdu []byte, pend **pending, pendC *<-chan time.Time) {
	if len(asdu) < 6 {
		return
	}
	typeID := asdu[0]
	cot := asdu[2] & 0x3f
	negative := asdu[2]&cotNegative != 0

	// command confirmation?
	if base := cmdBase(typeID); base != 0 && *pend != nil {
		ioa := ioa3(asdu[6:])
		if base == (*pend).base && (ioa == (*pend).ioa || ioa == 0) {
			switch {
			case cot == cotActCon && negative:
				(*pend).reply <- fmt.Errorf("manövern nekades (negativ ACTCON)")
				*pend, *pendC = nil, nil
			case cot >= 44 && cot <= 47:
				(*pend).reply <- fmt.Errorf("manövern nekades (COT %d)", cot)
				*pend, *pendC = nil, nil
			case cot == cotActTerm:
				(*pend).reply <- nil
				*pend, *pendC = nil, nil
			}
		}
		return
	}

	vl, el, ok := monLens(typeID)
	if !ok {
		return
	}
	sq := asdu[1]&0x80 != 0
	n := int(asdu[1] & 0x7f)
	body := asdu[6:]
	now := time.Now()
	vals := make([]model.Value, 0, n)
	off := 0
	var baseIOA uint32
	for i := 0; i < n; i++ {
		var ioa uint32
		if sq {
			if i == 0 {
				if off+3 > len(body) {
					break
				}
				baseIOA = ioa3(body[off:])
				off += 3
			}
			ioa = baseIOA + uint32(i)
		} else {
			if off+3 > len(body) {
				break
			}
			ioa = ioa3(body[off:])
			off += 3
		}
		if off+el > len(body) {
			break
		}
		elem := body[off : off+el]
		off += el
		t := w.monByIOA[ioa]
		if t == nil {
			continue
		}
		v, good := decodeMon(typeID, elem, vl, t)
		q := model.QualityGood
		if !good {
			q = model.QualityBad
		}
		vals = append(vals, model.Value{Tag: t.ID, V: v, Quality: q, TS: now})
	}
	if len(vals) == 0 {
		return
	}
	_ = cot
	w.emit(vals)
}

// emit pushes values to the hub and logs state changes for logged tags.
func (w *Worker) emit(vals []model.Value) {
	changed, prev := w.hub.Update(vals)
	for i, v := range changed {
		t := w.byID[v.Tag]
		if t == nil || !t.LogEvents || v.Quality != model.QualityGood || prev[i].Quality != model.QualityGood {
			continue
		}
		w.hub.AddEvent("state", "info", t.Name, fmt.Sprintf("%s → %s", stateText(t, prev[i].V), stateText(t, v.V)), "")
	}
}

func stateText(t *model.Tag, v float64) string {
	key := strconv.FormatFloat(v, 'f', -1, 64)
	if s, ok := t.StateText[key]; ok {
		return s
	}
	s := strconv.FormatFloat(v, 'f', t.Decimals, 64)
	if t.Unit != "" {
		s += " " + t.Unit
	}
	return s
}

// ---- decoding ----

func decodeMon(typeID byte, elem []byte, vl int, t *model.Tag) (float64, bool) {
	switch family(typeID) {
	case mSPNA:
		return bit(elem[0]&0x01 != 0), elem[0]&0x80 == 0 // IV in bit7
	case mDPNA:
		dpi := elem[0] & 0x03
		good := elem[0]&0x80 == 0 && (dpi == 1 || dpi == 2)
		if dpi == 2 {
			return 1, good
		}
		return 0, good
	case mSTNA:
		iv := int8(elem[0]<<1) >> 1 // 7-bit signed
		good := len(elem) < 2 || elem[1]&0x80 == 0
		return scale(t, float64(iv)), good
	case mMENA: // normalized
		raw := float64(int16(binary.LittleEndian.Uint16(elem[:2]))) / 32768
		good := len(elem) < 3 || elem[2]&0x80 == 0
		return scale(t, raw), good
	case mMENB: // scaled
		raw := float64(int16(binary.LittleEndian.Uint16(elem[:2])))
		good := len(elem) < 3 || elem[2]&0x80 == 0
		return scale(t, raw), good
	case mMENC: // short float
		raw := float64(math.Float32frombits(binary.LittleEndian.Uint32(elem[:4])))
		good := len(elem) < 5 || elem[4]&0x80 == 0
		return scale(t, raw), good
	}
	return 0, false
}

func scale(t *model.Tag, raw float64) float64 {
	s := t.Scale
	if s == 0 {
		s = 1
	}
	return raw*s + t.Offset
}

func bit(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// ---- command building ----

func (w *Worker) buildCommand(t *model.Tag, value float64) (base byte, ioa uint32, asdu []byte, err error) {
	ioa = t.CtrlIOA
	cmd := t.Cmd
	var typeID, element byte
	var seElem []byte
	switch cmd {
	case "C_SC":
		typeID = cSCNA
		if value != 0 {
			element = 1
		}
	case "", "C_DC":
		typeID = cDCNA
		if value != 0 {
			element = 2
		} else {
			element = 1
		}
	case "C_RC":
		typeID = cRCNA
		if value != 0 {
			element = 2
		} else {
			element = 1
		}
	case "C_SE_NC":
		typeID = cSENC
		raw := value
		if t.Scale != 0 {
			raw = (value - t.Offset) / t.Scale
		}
		seElem = make([]byte, 5)
		binary.LittleEndian.PutUint32(seElem, math.Float32bits(float32(raw)))
	default:
		return 0, 0, nil, fmt.Errorf("okänd kommandotyp %q", cmd)
	}
	asdu = []byte{typeID, 1, cotAct, 0, byte(w.ca), byte(w.ca >> 8), byte(ioa), byte(ioa >> 8), byte(ioa >> 16)}
	if seElem != nil {
		asdu = append(asdu, seElem...)
	} else {
		asdu = append(asdu, element)
	}
	return cmdBase(typeID), ioa, asdu, nil
}

func (w *Worker) sendInterrogation(conn net.Conn) error {
	asdu := []byte{cICNA, 1, cotAct, 0, byte(w.ca), byte(w.ca >> 8), 0, 0, 0, 20}
	return w.sendI(conn, asdu)
}

// ---- framing ----

func (w *Worker) writeU(conn net.Conn, fn byte) error {
	conn.SetWriteDeadline(time.Now().Add(w.timeout))
	_, err := conn.Write([]byte{0x68, 4, fn, 0, 0, 0})
	return err
}

func (w *Worker) sendS(conn net.Conn) error {
	conn.SetWriteDeadline(time.Now().Add(w.timeout))
	_, err := conn.Write([]byte{0x68, 4, 0x01, 0, byte(w.vr << 1), byte(w.vr >> 7)})
	return err
}

func (w *Worker) sendI(conn net.Conn, asdu []byte) error {
	b := make([]byte, 6+len(asdu))
	b[0], b[1] = 0x68, byte(4+len(asdu))
	b[2], b[3] = byte(w.vs<<1), byte(w.vs>>7)
	b[4], b[5] = byte(w.vr<<1), byte(w.vr>>7)
	copy(b[6:], asdu)
	w.vs = (w.vs + 1) & 0x7fff
	conn.SetWriteDeadline(time.Now().Add(w.timeout))
	_, err := conn.Write(b)
	return err
}

func reader(conn net.Conn, recv chan<- []byte, readErr chan<- error) {
	for {
		var h [2]byte
		if _, err := io.ReadFull(conn, h[:]); err != nil {
			readErr <- err
			return
		}
		if h[0] != 0x68 || h[1] < 4 {
			readErr <- fmt.Errorf("ogiltig APDU")
			return
		}
		frame := make([]byte, 2+int(h[1]))
		frame[0], frame[1] = h[0], h[1]
		if _, err := io.ReadFull(conn, frame[2:]); err != nil {
			readErr <- err
			return
		}
		recv <- frame
	}
}

func isI(f []byte) bool { return len(f) >= 6 && f[2]&0x01 == 0 }
func isS(f []byte) bool { return len(f) >= 6 && f[2]&0x03 == 0x01 }
func isU(f []byte) bool { return len(f) >= 6 && f[2]&0x03 == 0x03 }

func ioa3(b []byte) uint32 {
	if len(b) < 3 {
		return 0
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
}

func monLens(typeID byte) (valLen, elemLen int, ok bool) {
	switch typeID {
	case mSPNA:
		return 1, 1, true
	case mDPNA:
		return 1, 1, true
	case mSTNA:
		return 1, 2, true
	case mMENA, mMENB:
		return 2, 3, true
	case mMENC:
		return 4, 5, true
	case mSPTB, mDPTB:
		return 1, 8, true
	case mSTTB:
		return 1, 9, true
	case mMETD, mMETE:
		return 2, 10, true
	case mMETF:
		return 4, 12, true
	}
	return 0, 0, false
}

func family(typeID byte) byte {
	switch typeID {
	case mSPNA, mSPTB:
		return mSPNA
	case mDPNA, mDPTB:
		return mDPNA
	case mSTNA, mSTTB:
		return mSTNA
	case mMENA, mMETD:
		return mMENA
	case mMENB, mMETE:
		return mMENB
	case mMENC, mMETF:
		return mMENC
	}
	return 0
}

func cmdBase(typeID byte) byte {
	switch typeID {
	case cSCNA, cDCNA, cRCNA, cSENC:
		return typeID
	}
	return 0
}
