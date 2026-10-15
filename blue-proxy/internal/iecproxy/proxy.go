// Package iecproxy is a transparent IEC 60870-5-104 proxy. It forwards APDUs
// between a master and an upstream controlled station unchanged, decoding
// monitoring ASDUs (towards the master) to remember collected values and
// rewriting them in place with the frozen value for held points, and decoding
// command ASDUs (towards the device) to remember the last command. Frame
// lengths and sequence numbers are preserved, so the proxy is transparent to
// the protocol state machine.
package iecproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"blue-proxy/internal/config"
	"blue-proxy/internal/state"
)

// Monitoring type identifications.
const (
	mSPNA = 1
	mDPNA = 3
	mSTNA = 5
	mMENC = 13
	mSPTB = 30
	mDPTB = 31
	mSTTB = 32
	mMETF = 36
)

// Command type identifications.
const (
	cSCNA = 45
	cDCNA = 46
	cRCNA = 47
	cSENC = 50
	cSCTA = 58
	cDCTA = 59
	cRCTA = 60
	cSETC = 63
)

type Proxy struct {
	Listen   string
	Upstream string
	Timeout  time.Duration
	Store    *state.Store
	Logf     func(string, ...any)

	monByIOA  map[uint32]*config.Tag
	ctrlByIOA map[uint32]*config.Tag
}

// New builds the proxy and registers the device's monitorable points.
func New(listen, upstream string, timeout time.Duration, cfg *config.Loaded, store *state.Store) *Proxy {
	p := &Proxy{
		Listen: listen, Upstream: upstream, Timeout: timeout, Store: store,
		monByIOA: map[uint32]*config.Tag{}, ctrlByIOA: map[uint32]*config.Tag{},
	}
	for i := range cfg.Tags {
		t := &cfg.Tags[i]
		if t.IOA != 0 {
			p.monByIOA[t.IOA] = t
			store.Register(&state.Point{
				Key: t.ID, Name: t.Name, Category: config.Category(t), Proto: "iec104",
				Unit: t.Unit, Decimals: t.Decimals, Writable: t.Writable, StateText: t.StateText,
			})
		}
		if t.CtrlIOA != 0 {
			p.ctrlByIOA[t.CtrlIOA] = t
		}
	}
	return p
}

func (p *Proxy) logf(f string, a ...any) {
	if p.Logf != nil {
		p.Logf(f, a...)
	}
}

func (p *Proxy) ListenAndServe(ctx context.Context) error {
	l, err := net.Listen("tcp", p.Listen)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); l.Close() }()
	p.logf("IEC 104-proxy lyssnar på %s → %s", p.Listen, p.Upstream)
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go p.handle(ctx, c)
	}
}

func (p *Proxy) handle(ctx context.Context, master net.Conn) {
	defer master.Close()
	up, err := net.DialTimeout("tcp", p.Upstream, p.Timeout)
	if err != nil {
		p.logf("IEC 104: kan inte nå uppströms %s: %v", p.Upstream, err)
		return
	}
	defer up.Close()
	stop := context.AfterFunc(ctx, func() { master.Close(); up.Close() })
	defer stop()
	p.logf("IEC 104: master %s ansluten", master.RemoteAddr())

	var once sync.Once
	done := make(chan struct{})
	closeBoth := func() { once.Do(func() { close(done) }); master.Close(); up.Close() }

	go p.pump(master, up, false, closeBoth) // master → device (commands)
	go p.pump(up, master, true, closeBoth)  // device → master (monitoring)
	<-done
}

// pump copies APDUs from src to dst. monitoring selects the processing done.
func (p *Proxy) pump(src, dst net.Conn, monitoring bool, closeBoth func()) {
	defer closeBoth()
	for {
		frame, err := readAPDU(src)
		if err != nil {
			return
		}
		// I-frame? control octet 0, LSB == 0.
		if len(frame) >= 6 && frame[2]&0x01 == 0 {
			asdu := frame[6:]
			if monitoring {
				p.onMonitoring(asdu)
			} else {
				p.onCommand(asdu)
			}
		}
		if _, err := dst.Write(frame); err != nil {
			return
		}
	}
}

// ---- monitoring (device → master) ----

func (p *Proxy) onMonitoring(asdu []byte) {
	if len(asdu) < 6 {
		return
	}
	typeID := asdu[0]
	vsq := asdu[1]
	sq := vsq&0x80 != 0
	n := int(vsq & 0x7f)
	valLen, elemLen, ok := monLens(typeID)
	if !ok {
		return
	}
	body := asdu[6:]
	off := 0
	var baseIOA uint32
	for i := 0; i < n; i++ {
		var ioa uint32
		if sq {
			if i == 0 {
				if off+3 > len(body) {
					return
				}
				baseIOA = ioa3(body[off:])
				off += 3
			}
			ioa = baseIOA + uint32(i)
		} else {
			if off+3 > len(body) {
				return
			}
			ioa = ioa3(body[off:])
			off += 3
		}
		if off+elemLen > len(body) {
			return
		}
		elem := body[off : off+elemLen]
		off += elemLen
		t := p.monByIOA[ioa]
		if t == nil {
			continue
		}
		cv := decodeMon(typeID, elem)
		emit, held := p.Store.Report(t.ID, cv)
		if held {
			encodeMon(typeID, elem, emit, valLen)
		}
	}
}

func decodeMon(typeID byte, elem []byte) float64 {
	switch family(typeID) {
	case mSPNA:
		if elem[0]&0x01 != 0 {
			return 1
		}
		return 0
	case mDPNA:
		if elem[0]&0x03 == 2 { // DPI: 2 = ON
			return 1
		}
		return 0
	case mSTNA:
		v := int8(elem[0] << 1) // bits0-6 two's complement, drop transient bit7
		return float64(v >> 1)
	case mMENC:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(elem[:4])))
	}
	return 0
}

func encodeMon(typeID byte, elem []byte, v float64, valLen int) {
	switch family(typeID) {
	case mSPNA:
		elem[0] = (elem[0] &^ 0x01) | boolBit(v != 0)
	case mDPNA:
		dpi := byte(1) // OFF
		if v != 0 {
			dpi = 2 // ON
		}
		elem[0] = (elem[0] &^ 0x03) | dpi
	case mSTNA:
		iv := int(math.Round(v))
		if iv > 63 {
			iv = 63
		} else if iv < -64 {
			iv = -64
		}
		elem[0] = (elem[0] & 0x80) | byte(iv&0x7f)
	case mMENC:
		binary.LittleEndian.PutUint32(elem[:4], math.Float32bits(float32(v)))
	}
	_ = valLen
}

// ---- commands (master → device) ----

func (p *Proxy) onCommand(asdu []byte) {
	if len(asdu) < 9 {
		return
	}
	typeID := asdu[0]
	base := cmdBase(typeID)
	if base == 0 {
		return
	}
	ioa := ioa3(asdu[6:])
	t := p.ctrlByIOA[ioa]
	if t == nil {
		return
	}
	e := asdu[9:]
	if len(e) < 1 {
		return
	}
	var v float64
	var text string
	switch base {
	case cSCNA:
		v = boolToF(e[0]&0x01 != 0)
		text = cmdText(t, v)
	case cDCNA:
		v = boolToF(e[0]&0x03 == 2)
		text = cmdText(t, v)
	case cRCNA:
		if e[0]&0x03 == 2 {
			v, text = 1, "höj"
		} else {
			v, text = 0, "sänk"
		}
	case cSENC:
		if len(e) >= 4 {
			v = float64(math.Float32frombits(binary.LittleEndian.Uint32(e[:4])))
			text = cmdText(t, v)
		}
	}
	blocked := p.Store.IsBlocked(t.ID)
	if blocked {
		// Rewrite the IOA to 0 (unused) so the device rejects the command with
		// a negative confirmation. The frame length and sequence number are
		// preserved, so the link stays transparent.
		asdu[6], asdu[7], asdu[8] = 0, 0, 0
	}
	p.Store.RecordCommand(t.ID, v, text, blocked)
}

// ---- helpers ----

// monLens returns (value length, element length, ok) for a monitoring type.
func monLens(typeID byte) (int, int, bool) {
	switch typeID {
	case mSPNA:
		return 1, 1, true
	case mDPNA:
		return 1, 1, true
	case mSTNA:
		return 1, 2, true
	case mMENC:
		return 4, 5, true
	case mSPTB:
		return 1, 8, true
	case mDPTB:
		return 1, 8, true
	case mSTTB:
		return 1, 9, true
	case mMETF:
		return 4, 12, true
	}
	return 0, 0, false
}

// family maps a (possibly time-tagged) monitoring type to its base type.
func family(typeID byte) byte {
	switch typeID {
	case mSPNA, mSPTB:
		return mSPNA
	case mDPNA, mDPTB:
		return mDPNA
	case mSTNA, mSTTB:
		return mSTNA
	case mMENC, mMETF:
		return mMENC
	}
	return 0
}

func cmdBase(typeID byte) byte {
	switch typeID {
	case cSCNA, cSCTA:
		return cSCNA
	case cDCNA, cDCTA:
		return cDCNA
	case cRCNA, cRCTA:
		return cRCNA
	case cSENC, cSETC:
		return cSENC
	}
	return 0
}

func ioa3(b []byte) uint32 { return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 }

func boolBit(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func boolToF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func cmdText(t *config.Tag, v float64) string {
	if t.StateText != nil {
		if s, ok := t.StateText[strconv.FormatFloat(v, 'f', -1, 64)]; ok {
			return s
		}
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if t.Unit != "" {
		s += " " + t.Unit
	}
	return s
}

// readAPDU reads one complete APDU (0x68, len, then len octets).
func readAPDU(c net.Conn) ([]byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(c, h[:]); err != nil {
		return nil, err
	}
	if h[0] != 0x68 || h[1] < 4 {
		return nil, fmt.Errorf("ogiltig APDU-header % x", h)
	}
	frame := make([]byte, 2+int(h[1]))
	frame[0], frame[1] = h[0], h[1]
	if _, err := io.ReadFull(c, frame[2:]); err != nil {
		return nil, err
	}
	return frame, nil
}
