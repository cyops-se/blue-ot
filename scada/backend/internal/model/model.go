// Package model contains the data types shared between the backend packages
// and exposed through the REST/WebSocket API.
package model

import (
	"encoding/json"
	"strconv"
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

// Protocols the backend can poll a device with.
const (
	ProtoModbus = "modbus"
	ProtoIEC104 = "iec104"
)

// Device is a field device the backend polls, over Modbus TCP or IEC 104.
type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Protocol is "modbus" (default) or "iec104".
	Protocol  string `json:"protocol,omitempty"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	UnitID    int    `json:"unitId"`       // Modbus unit id
	CA        int    `json:"ca,omitempty"` // IEC 104 common address of ASDU
	PollMs    int    `json:"pollMs"`       // Modbus poll period; IEC 104 re-interrogation period
	TimeoutMs int    `json:"timeoutMs"`
	Enabled   bool   `json:"enabled"`
	// IEC holds a secondary IEC 60870-5-104 endpoint of a Modbus device, used by
	// the opctl tool and the blue-proxy proxy. It is not used for polling.
	IEC *IEC104 `json:"iec104,omitempty"`
}

// IsIEC reports whether the device is polled over IEC 104.
func (d *Device) IsIEC() bool { return d.Protocol == ProtoIEC104 }

// IEC104 is the IEC 60870-5-104 endpoint of a device.
type IEC104 struct {
	Port int `json:"port"` // control station TCP port (host is Device.Host)
	CA   int `json:"ca"`   // common address of ASDU
}

// Tag is a single telemetry point mapped to a Modbus address.
type Tag struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Device      string `json:"device"`
	Area        string `json:"area"`
	Address     int    `json:"address"`
	DataType    string `json:"dataType"`
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
	// IOA is the IEC 60870-5-104 monitoring information object address of this
	// point. CtrlIOA is its control address, and Cmd its command type ("C_DC"
	// for breakers, "C_SC" for switches). Set by the generator scripts; used by
	// the opctl tool and the blue-proxy proxy, ignored by the backend.
	IOA     uint32 `json:"ioa,omitempty"`
	CtrlIOA uint32 `json:"ctrlIoa,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
	// Alarm enables alarm supervision of the tag.
	Alarm *TagAlarm `json:"alarm,omitempty"`
}

// TagAlarm configures alarm supervision of a tag. The alarm is active while
// the raw value of the tag is one of Values.
type TagAlarm struct {
	Values []float64 `json:"values"`
	// Uncommanded only raises the alarm when the tag changes to an alarm value
	// without a matching command from the HMI, e.g. a breaker that opens
	// without an operator manoeuvre. A command matches when it wrote the new
	// value to the command tag within the last 30 seconds.
	Uncommanded bool `json:"uncommanded,omitempty"`
	// CmdTag is the command tag of the object. Empty means the tag itself,
	// "<id>.Cmd", "<id>_CMD" or, for "<x>_POS", "<x>_CMD".
	CmdTag string `json:"cmdTag,omitempty"`
	// Text is the alarm text. Empty means the state text of the value.
	Text string `json:"text,omitempty"`
}

// Text returns the state text of a raw value, or the value with its unit.
func (t *Tag) Text(v float64) string {
	if s, ok := t.StateText[strconv.FormatFloat(v, 'f', -1, 64)]; ok {
		return s
	}
	s := strconv.FormatFloat(v, 'f', t.Decimals, 64)
	if t.Unit != "" {
		s += " " + t.Unit
	}
	return s
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

// Alarm is an entry in the alarm list. It stays in the list while it is
// active or not yet acknowledged.
type Alarm struct {
	ID      string     `json:"id"`      // tag id, or CommAlarmID(device)
	Kind    string     `json:"kind"`    // state, comm
	Source  string     `json:"source"`  // tag or device name
	Station string     `json:"station"` // device name the alarm belongs to
	View    string     `json:"view"`    // id of a view that shows the object
	Message string     `json:"message"`
	Active  bool       `json:"active"`
	Acked   bool       `json:"acked"`
	Raised  time.Time  `json:"raised"`
	Cleared *time.Time `json:"cleared,omitempty"`
	AckedAt *time.Time `json:"ackedAt,omitempty"`
	AckedBy string     `json:"ackedBy,omitempty"`
}

// CommAlarmID is the alarm id of the communication alarm of a device.
func CommAlarmID(device string) string { return "comm:" + device }

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
