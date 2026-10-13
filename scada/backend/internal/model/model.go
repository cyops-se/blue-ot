// Package model contains the data types shared between the backend packages
// and exposed through the REST/WebSocket API.
package model

import (
	"encoding/json"
	"time"
)

// Modbus data areas.
const (
	AreaCoil     = "coil"     // 0x – read/write bits
	AreaDiscrete = "discrete" // 1x – read-only bits
	AreaInput    = "input"    // 3x – read-only registers
	AreaHolding  = "holding"  // 4x – read/write registers
)

// Tag data types.
const (
	TypeBool    = "bool"
	TypeUint16  = "uint16"
	TypeInt16   = "int16"
	TypeUint32  = "uint32"
	TypeInt32   = "int32"
	TypeFloat32 = "float32"
	TypeFloat64 = "float64"
)

// Quality codes for realtime values.
const (
	QualityGood = "good"
	QualityBad  = "bad"
)

// Device is a Modbus TCP server (RTU, PLC, IED ...) that the backend polls.
type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	UnitID    int    `json:"unitId"`
	PollMs    int    `json:"pollMs"`
	TimeoutMs int    `json:"timeoutMs"`
	Enabled   bool   `json:"enabled"`
}

// Tag is a single telemetry point mapped to a Modbus address.
type Tag struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Device      string  `json:"device"`
	Area        string  `json:"area"`
	Address     int     `json:"address"`
	DataType    string  `json:"dataType"`
	// Bit extracts a single bit (0-15) from a register when DataType is bool.
	// -1 (or omitted for bit areas) means "whole register != 0".
	Bit int `json:"bit"`
	// WordSwap selects low word first for 32/64-bit values.
	WordSwap bool `json:"wordSwap"`
	// ByteSwap swaps the two bytes in each register.
	ByteSwap bool    `json:"byteSwap"`
	Scale    float64 `json:"scale"`
	Offset   float64 `json:"offset"`
	Unit     string  `json:"unit"`
	Decimals int     `json:"decimals"`
	Writable bool    `json:"writable"`
	// LogEvents records every value change in the event list (useful for
	// breaker/disconnector positions).
	LogEvents bool `json:"logEvents"`
	// StateText maps raw values to texts used in events, e.g. {"1":"Öppen","2":"Sluten"}.
	StateText map[string]string `json:"stateText,omitempty"`
}

// RegisterCount returns how many 16-bit registers the tag occupies.
func (t *Tag) RegisterCount() int {
	switch t.DataType {
	case TypeUint32, TypeInt32, TypeFloat32:
		return 2
	case TypeFloat64:
		return 4
	default:
		return 1
	}
}

// IsBitArea reports whether the tag lives in a coil/discrete input area.
func (t *Tag) IsBitArea() bool {
	return t.Area == AreaCoil || t.Area == AreaDiscrete
}

// Config is the persisted communication configuration.
type Config struct {
	Devices []Device `json:"devices"`
	Tags    []Tag    `json:"tags"`
}

// Value is the realtime state of a tag.
type Value struct {
	Tag     string    `json:"tag"`
	V       float64   `json:"v"`
	Quality string    `json:"q"`
	TS      time.Time `json:"ts"`
}

// Sample is one historic point of a tag.
type Sample struct {
	V  float64 `json:"v"`
	TS int64   `json:"t"` // unix milliseconds
}

// DeviceStatus describes the communication state of a device.
type DeviceStatus struct {
	Device    string    `json:"device"`
	Connected bool      `json:"connected"`
	Error     string    `json:"error,omitempty"`
	LastPoll  time.Time `json:"lastPoll"`
}

// Event is an entry in the event list.
type Event struct {
	ID       int64     `json:"id"`
	TS       time.Time `json:"ts"`
	Kind     string    `json:"kind"` // command, comm, state, system
	Severity string    `json:"severity"`
	Source   string    `json:"source"`
	Message  string    `json:"message"`
	User     string    `json:"user,omitempty"`
}

// View is an HMI picture. Elements are stored as opaque JSON so the
// frontend can evolve the symbol library without backend changes.
type View struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	Background string            `json:"background"`
	Grid       int               `json:"grid"`
	Elements   []json.RawMessage `json:"elements"`
	Updated    time.Time         `json:"updated"`
}

// ViewSummary is used for view listings.
type ViewSummary struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Updated time.Time `json:"updated"`
}

// CommandRequest asks the backend to write a value to a writable tag.
type CommandRequest struct {
	Tag    string  `json:"tag"`
	Value  float64 `json:"value"`
	Source string  `json:"source"` // e.g. symbol label, used in the event log
	Text   string  `json:"text"`   // e.g. "Frånslag"
	User   string  `json:"user"`
}
