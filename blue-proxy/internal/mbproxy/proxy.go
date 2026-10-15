// Package mbproxy is a transparent Modbus TCP proxy. It forwards requests and
// responses between a master and an upstream device unchanged, while decoding
// read responses to remember collected values and rewriting them with the
// frozen value for points that are held. Writes are remembered as the last
// command and forwarded unchanged.
package mbproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"blue-proxy/internal/config"
	"blue-proxy/internal/state"
)

// excDeviceFailure (0x04) is returned to the master for a blocked command.
const excDeviceFailure = 0x04

type Proxy struct {
	Listen   string
	Upstream string
	Timeout  time.Duration
	Store    *state.Store
	Logf     func(string, ...any)

	byArea    map[string][]*config.Tag // area -> tags sorted by address
	cmdStatus map[string]*config.Tag   // writable tag ID -> its status tag
}

// New builds the proxy and registers every tag of the device as a point.
func New(listen, upstream string, timeout time.Duration, cfg *config.Loaded, store *state.Store) *Proxy {
	p := &Proxy{Listen: listen, Upstream: upstream, Timeout: timeout, Store: store,
		byArea: map[string][]*config.Tag{}, cmdStatus: map[string]*config.Tag{}}
	byID := map[string]*config.Tag{}
	for i := range cfg.Tags {
		byID[cfg.Tags[i].ID] = &cfg.Tags[i]
	}
	for i := range cfg.Tags {
		t := &cfg.Tags[i]
		p.byArea[t.Area] = append(p.byArea[t.Area], t)
		if t.Writable {
			p.cmdStatus[t.ID] = statusFor(byID, t.ID)
		}
		store.Register(&state.Point{
			Key: t.ID, Name: t.Name, Category: config.Category(t), Proto: "modbus",
			Unit: t.Unit, Decimals: t.Decimals, Writable: t.Writable, StateText: t.StateText,
		})
	}
	return p
}

// statusFor finds the status (position) tag for a writable command tag id.
func statusFor(byID map[string]*config.Tag, cmdID string) *config.Tag {
	var cands []string
	if s, ok := strings.CutSuffix(cmdID, ".Cmd"); ok {
		cands = append(cands, s, s+"_POS", s+".Pos")
	}
	if s, ok := strings.CutSuffix(cmdID, "_CMD"); ok {
		cands = append(cands, s+"_POS", s)
	}
	for _, id := range cands {
		if t := byID[id]; t != nil && !t.Writable {
			return t
		}
	}
	return nil
}

// recordTarget returns the point id and state tag to record a command against:
// the status point when there is one, otherwise the command tag itself.
func (p *Proxy) recordTarget(cmd *config.Tag) *config.Tag {
	if st := p.cmdStatus[cmd.ID]; st != nil {
		return st
	}
	return cmd
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
	p.logf("Modbus-proxy lyssnar på %s → %s", p.Listen, p.Upstream)
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
		p.logf("Modbus: kan inte nå uppströms %s: %v", p.Upstream, err)
		return
	}
	defer up.Close()
	stop := context.AfterFunc(ctx, func() { master.Close(); up.Close() })
	defer stop()
	p.logf("Modbus: master %s ansluten", master.RemoteAddr())

	for {
		hdr, body, err := readFrame(master, 10*time.Minute)
		if err != nil {
			return
		}
		req := body[1:] // body[0] = unit id

		if p.onRequest(req) {
			// Blocked command: reject with a Modbus exception, do not forward.
			resp := []byte{body[0], req[0] | 0x80, excDeviceFailure}
			if err := writeFrame(master, hdr, resp, p.Timeout); err != nil {
				return
			}
			continue
		}

		if err := writeFrame(up, hdr, body, p.Timeout); err != nil {
			return
		}
		rhdr, rbody, err := readFrame(up, p.Timeout)
		if err != nil {
			return
		}
		if len(rbody) > 1 {
			p.onResponse(req, rbody[1:]) // may modify rbody in place
		}
		if err := writeFrame(master, rhdr, rbody, p.Timeout); err != nil {
			return
		}
	}
}

func u16(b []byte, i int) int { return int(binary.BigEndian.Uint16(b[i:])) }

// onRequest records writes (commands) as they pass towards the device and
// reports whether the request targets a blocked point (and must be rejected).
func (p *Proxy) onRequest(req []byte) (block bool) {
	if len(req) < 1 {
		return false
	}
	switch req[0] {
	case 5: // write single coil
		if len(req) >= 5 {
			block = p.recordCoil(u16(req, 1), u16(req, 3) == 0xFF00)
		}
	case 6: // write single register
		if len(req) >= 5 {
			block = p.recordRegisters(u16(req, 1), []uint16{uint16(u16(req, 3))})
		}
	case 15: // write multiple coils
		if len(req) >= 6 {
			start, qty := u16(req, 1), u16(req, 3)
			bits := req[6:]
			for i := 0; i < qty && 6+i/8 < len(req); i++ {
				if p.recordCoil(start+i, bits[i/8]&(1<<(i%8)) != 0) {
					block = true
				}
			}
		}
	case 16: // write multiple registers
		if len(req) >= 6 {
			start, qty := u16(req, 1), u16(req, 3)
			regs := make([]uint16, 0, qty)
			for i := 0; i < qty && 6+2*i+1 < len(req); i++ {
				regs = append(regs, uint16(u16(req, 6+2*i)))
			}
			block = p.recordRegisters(start, regs)
		}
	}
	return block
}

func (p *Proxy) recordCoil(addr int, on bool) (blocked bool) {
	for _, t := range p.byArea[config.AreaCoil] {
		if t.Writable && t.Address == addr {
			v := 0.0
			if on {
				v = 1
			}
			tgt := p.recordTarget(t)
			blocked = p.Store.IsBlocked(tgt.ID)
			p.Store.RecordCommand(tgt.ID, v, cmdText(tgt, v), blocked)
		}
	}
	return blocked
}

func (p *Proxy) recordRegisters(start int, regs []uint16) (blocked bool) {
	for _, t := range p.byArea[config.AreaHolding] {
		if !t.Writable {
			continue
		}
		n := t.RegisterCount()
		if t.Address >= start && t.Address+n <= start+len(regs) {
			v := t.DecodeRegisters(regs[t.Address-start : t.Address-start+n])
			tgt := p.recordTarget(t)
			b := p.Store.IsBlocked(tgt.ID)
			blocked = blocked || b
			p.Store.RecordCommand(tgt.ID, v, cmdText(tgt, v), b)
		}
	}
	return blocked
}

// onResponse decodes a read response, updates collected values and rewrites
// held points with their frozen value.
func (p *Proxy) onResponse(req, resp []byte) {
	if len(req) < 5 || len(resp) < 2 || resp[0]&0x80 != 0 || resp[0] != req[0] {
		return
	}
	start, qty := u16(req, 1), u16(req, 3)
	switch resp[0] {
	case 1, 2: // read coils / discrete inputs
		area := config.AreaCoil
		if resp[0] == 2 {
			area = config.AreaDiscrete
		}
		payload := resp[2:]
		for _, t := range p.byArea[area] {
			if t.Address < start || t.Address >= start+qty {
				continue
			}
			i := t.Address - start
			if i/8 >= len(payload) {
				continue
			}
			bit := payload[i/8]&(1<<(i%8)) != 0
			cv := 0.0
			if bit {
				cv = 1
			}
			emit, held := p.Store.Report(t.ID, cv)
			if held {
				if emit != 0 {
					payload[i/8] |= 1 << (i % 8)
				} else {
					payload[i/8] &^= 1 << (i % 8)
				}
			}
		}
	case 3, 4: // read holding / input registers
		area := config.AreaHolding
		if resp[0] == 4 {
			area = config.AreaInput
		}
		regs := make([]uint16, qty)
		for i := 0; i < qty && 2+2*i+1 < len(resp); i++ {
			regs[i] = uint16(u16(resp, 2+2*i))
		}
		for _, t := range p.byArea[area] {
			n := t.RegisterCount()
			if t.Address < start || t.Address+n > start+qty {
				continue
			}
			off := t.Address - start
			cv := t.DecodeRegisters(regs[off : off+n])
			emit, held := p.Store.Report(t.ID, cv)
			if held {
				enc := t.EncodeRegisters(emit)
				for k, r := range enc {
					binary.BigEndian.PutUint16(resp[2+2*(off+k):], r)
				}
			}
		}
	}
}

func cmdText(t *config.Tag, v float64) string {
	if t.StateText != nil {
		if s, ok := t.StateText[strconv.FormatFloat(v, 'f', -1, 64)]; ok {
			return s
		}
	}
	s := "skriv " + strconv.FormatFloat(v, 'f', -1, 64)
	if t.Unit != "" {
		s += " " + t.Unit
	}
	return s
}

// ---- framing ----

func readFrame(c net.Conn, timeout time.Duration) (hdr [6]byte, body []byte, err error) {
	c.SetReadDeadline(time.Now().Add(timeout))
	if _, err = io.ReadFull(c, hdr[:]); err != nil {
		return
	}
	if binary.BigEndian.Uint16(hdr[2:]) != 0 {
		err = fmt.Errorf("ogiltig protokoll-id")
		return
	}
	n := int(binary.BigEndian.Uint16(hdr[4:]))
	if n < 1 || n > 260 {
		err = fmt.Errorf("ogiltig längd %d", n)
		return
	}
	body = make([]byte, n)
	_, err = io.ReadFull(c, body)
	return
}

func writeFrame(c net.Conn, hdr [6]byte, body []byte, timeout time.Duration) error {
	binary.BigEndian.PutUint16(hdr[4:], uint16(len(body)))
	c.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := c.Write(hdr[:]); err != nil {
		return err
	}
	_, err := c.Write(body)
	return err
}
