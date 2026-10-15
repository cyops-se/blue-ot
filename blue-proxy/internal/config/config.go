// Package config loads the subset of the SCADA configuration (scada's
// config.json) that the proxy needs: the device endpoint and the tags of one
// device, with their Modbus addressing and IEC 104 information object
// addresses. It also classifies each tag into a type.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Modbus data areas.
const (
	AreaCoil     = "coil"
	AreaDiscrete = "discrete"
	AreaInput    = "input"
	AreaHolding  = "holding"
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

// IEC104 is a device's IEC 60870-5-104 endpoint.
type IEC104 struct {
	Port int `json:"port"`
	CA   int `json:"ca"`
}

// Device is one upstream device.
type Device struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	UnitID    int     `json:"unitId"`
	TimeoutMs int     `json:"timeoutMs"`
	IEC       *IEC104 `json:"iec104,omitempty"`
}

// Tag is a telemetry or command point.
type Tag struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Device    string            `json:"device"`
	Area      string            `json:"area"`
	Address   int               `json:"address"`
	DataType  string            `json:"dataType"`
	Bit       int               `json:"bit"`
	WordSwap  bool              `json:"wordSwap"`
	ByteSwap  bool              `json:"byteSwap"`
	Scale     float64           `json:"scale"`
	Offset    float64           `json:"offset"`
	Unit      string            `json:"unit"`
	Decimals  int               `json:"decimals"`
	Writable  bool              `json:"writable"`
	StateText map[string]string `json:"stateText,omitempty"`
	IOA       uint32            `json:"ioa,omitempty"`
	CtrlIOA   uint32            `json:"ctrlIoa,omitempty"`
	Cmd       string            `json:"cmd,omitempty"`
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

// IsBitArea reports whether the tag lives in a coil/discrete area.
func (t *Tag) IsBitArea() bool { return t.Area == AreaCoil || t.Area == AreaDiscrete }

type file struct {
	Devices []Device `json:"devices"`
	Tags    []Tag    `json:"tags"`
}

// Loaded is the configuration for one device.
type Loaded struct {
	Device Device
	Tags   []Tag // tags belonging to Device, scale defaulted to 1
}

// Load reads config.json (path may be the file or its directory) and returns
// the device and its tags.
func Load(path, deviceID string) (*Loaded, error) {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		path = filepath.Join(path, "config.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("tolkar %s: %w", path, err)
	}
	var dev *Device
	for i := range f.Devices {
		if f.Devices[i].ID == deviceID {
			dev = &f.Devices[i]
		}
	}
	if dev == nil {
		return nil, fmt.Errorf("enheten %q finns inte i %s", deviceID, path)
	}
	l := &Loaded{Device: *dev}
	for _, t := range f.Tags {
		if t.Device != deviceID {
			continue
		}
		if t.Scale == 0 {
			t.Scale = 1
		}
		l.Tags = append(l.Tags, t)
	}
	return l, nil
}

// ---- type classification ----

var (
	reBreaker = regexp.MustCompile(`(^|[._])Q[0-9]`)
	reDisc    = regexp.MustCompile(`(^|[._])QS[0-9]`)
)

// Categories used by the proxy for "all of a type" selection.
const (
	CatBreaker      = "brytare"
	CatDisconnector = "frånskiljare"
	CatPump         = "pump"
	CatMeasurement  = "mätvärde"
	CatBinary       = "binär"
	CatSetpoint     = "börvärde"
	CatOther        = "övrigt"
)

// Category classifies a tag. Command tags are classified like their point.
func Category(t *Tag) string {
	low := strings.ToLower(t.ID + " " + t.Name)
	switch {
	case strings.Contains(low, "pump"):
		return CatPump
	case reDisc.MatchString(t.ID) || strings.Contains(low, "frånskilj") || strings.Contains(low, "disconnect"):
		return CatDisconnector
	case strings.Contains(t.ID, ".CB") || strings.Contains(t.ID, "_CB") ||
		reBreaker.MatchString(t.ID) || strings.Contains(low, "brytare") || strings.Contains(low, "breaker"):
		return CatBreaker
	}
	switch {
	case t.Writable && !t.IsBitArea():
		return CatSetpoint
	case t.DataType == TypeBool || t.IsBitArea():
		return CatBinary
	case t.Area == AreaInput || t.Area == AreaHolding:
		return CatMeasurement
	}
	return CatOther
}
