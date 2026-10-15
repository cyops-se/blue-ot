// Package iec104client is a minimal IEC 60870-5-104 controlling station
// (master) that connects, sends a single command with direct execute and
// waits for ACTCON/ACTTERM. It is used by the opctl tool, not by the SCADA
// backend. Only what is needed to operate breakers (C_DC) and switches
// (C_SC) is implemented.
package iec104client

import (
	"fmt"
	"io"
	"net"
	"time"
)

// Type identifications.
const (
	C_SC_NA_1 = 45 // single command (switch)
	C_DC_NA_1 = 46 // double command (breaker)
)

// Causes of transmission.
const (
	cotAct         = 6
	cotActCon      = 7
	cotActTerm     = 10
	cotUnknownType = 44
	cotUnknownCOT  = 45
	cotUnknownCA   = 46
	cotUnknownIOA  = 47
	cotNegative    = 0x40
)

// U-frame functions.
const (
	uStartAct = 0x07
	uStartCon = 0x0B
	uStopAct  = 0x13
)

// Client is a single-connection IEC 104 master.
type Client struct {
	conn    net.Conn
	timeout time.Duration
	vs, vr  uint16
	// Logf, when set, receives a short trace of the exchange.
	Logf func(format string, args ...any)
}

// Dial connects to a controlled station and performs STARTDT.
func Dial(addr string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, timeout: timeout}
	if err := c.startDT(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// Close sends STOPDT and closes the connection.
func (c *Client) Close() error {
	_ = c.writeU(uStopAct)
	return c.conn.Close()
}

func (c *Client) startDT() error {
	if err := c.writeU(uStartAct); err != nil {
		return err
	}
	// Wait for STARTDT con, skipping any I/S frames that arrive first.
	deadline := time.Now().Add(c.timeout)
	for {
		f, err := c.readAPDU(deadline)
		if err != nil {
			return fmt.Errorf("STARTDT: %w", err)
		}
		if f[0]&0x03 == 0x03 { // U-frame
			if f[0] == uStartCon {
				c.logf("STARTDT bekräftad")
				return nil
			}
			continue
		}
		if f[0]&0x01 == 0 { // I-frame: count and keep looking
			c.vr = (c.vr + 1) & 0x7fff
		}
	}
}

// Breaker sends a double command (C_DC). close=true closes (DCS=2, TILL),
// false opens (DCS=1, FRÅN).
func (c *Client) Breaker(ca uint16, ioa uint32, close bool) error {
	dcs := byte(1)
	if close {
		dcs = 2
	}
	return c.command(C_DC_NA_1, ca, ioa, dcs)
}

// Switch sends a single command (C_SC). on=true sets the switch on (SCS=1).
func (c *Client) Switch(ca uint16, ioa uint32, on bool) error {
	var scs byte
	if on {
		scs = 1
	}
	return c.command(C_SC_NA_1, ca, ioa, scs)
}

// command sends one command ASDU (direct execute) and waits for the terminal
// response.
func (c *Client) command(typeID byte, ca uint16, ioa uint32, element byte) error {
	asdu := make([]byte, 0, 10)
	asdu = append(asdu,
		typeID,
		1,         // VSQ: one information object, SQ=0
		cotAct, 0, // cause of transmission + originator
		byte(ca), byte(ca>>8),
		byte(ioa), byte(ioa>>8), byte(ioa>>16),
		element,
	)
	if err := c.writeI(asdu); err != nil {
		return err
	}
	c.logf("skickade kommando typ %d IOA %d element 0x%02x", typeID, ioa, element)

	gotCon := false
	deadline := time.Now().Add(c.timeout)
	for {
		f, err := c.readAPDU(deadline)
		if err != nil {
			return fmt.Errorf("väntar på svar: %w", err)
		}
		if f[0]&0x01 != 0 { // S- or U-frame, ignore
			continue
		}
		c.vr = (c.vr + 1) & 0x7fff
		c.ackRecv()
		a := f[4:] // ASDU after the 4 control octets
		if len(a) < 6 || a[0] != typeID {
			continue
		}
		cause := a[2] & 0x3f
		negative := a[2]&cotNegative != 0
		switch cause {
		case cotActCon:
			if negative {
				return fmt.Errorf("kommandot nekades (negativ ACTCON)")
			}
			c.logf("ACTCON (positiv)")
			gotCon = true
		case cotActTerm:
			c.logf("ACTTERM")
			return nil
		case cotUnknownType, cotUnknownCOT, cotUnknownCA, cotUnknownIOA:
			// The controlled station rejected the command (e.g. a proxy blocked
			// it by invalidating the address).
			return fmt.Errorf("kommandot nekades (COT %d)", cause)
		default:
			if negative {
				return fmt.Errorf("kommandot nekades (negativt svar, COT %d)", cause)
			}
		}
		_ = gotCon
	}
}

// ---- framing ----

func (c *Client) writeU(fn byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	_, err := c.conn.Write([]byte{0x68, 4, fn, 0, 0, 0})
	return err
}

func (c *Client) ackRecv() {
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	c.conn.Write([]byte{0x68, 4, 0x01, 0, byte(c.vr << 1), byte(c.vr >> 7)})
}

func (c *Client) writeI(asdu []byte) error {
	b := make([]byte, 6+len(asdu))
	b[0], b[1] = 0x68, byte(4+len(asdu))
	b[2], b[3] = byte(c.vs<<1), byte(c.vs>>7)
	b[4], b[5] = byte(c.vr<<1), byte(c.vr>>7)
	copy(b[6:], asdu)
	c.vs = (c.vs + 1) & 0x7fff
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	_, err := c.conn.Write(b)
	return err
}

// readAPDU reads one full APDU (control octets + ASDU) with the given deadline.
func (c *Client) readAPDU(deadline time.Time) ([]byte, error) {
	c.conn.SetReadDeadline(deadline)
	var h [2]byte
	if _, err := io.ReadFull(c.conn, h[:]); err != nil {
		return nil, err
	}
	if h[0] != 0x68 || h[1] < 4 {
		return nil, fmt.Errorf("ogiltig APDU-header % x", h)
	}
	b := make([]byte, h[1])
	if _, err := io.ReadFull(c.conn, b); err != nil {
		return nil, err
	}
	return b, nil
}
