package config

import (
	"encoding/binary"
	"math"
)

// toBytes converts registers to a big-endian "ABCD" byte stream applying the
// tag's word and byte ordering.
func toBytes(t *Tag, regs []uint16) []byte {
	n := len(regs)
	ordered := make([]uint16, n)
	for i, r := range regs {
		if t.WordSwap {
			ordered[n-1-i] = r
		} else {
			ordered[i] = r
		}
	}
	b := make([]byte, 2*n)
	for i, r := range ordered {
		if t.ByteSwap {
			r = r<<8 | r>>8
		}
		binary.BigEndian.PutUint16(b[2*i:], r)
	}
	return b
}

func fromBytes(t *Tag, b []byte) []uint16 {
	n := len(b) / 2
	ordered := make([]uint16, n)
	for i := 0; i < n; i++ {
		r := binary.BigEndian.Uint16(b[2*i:])
		if t.ByteSwap {
			r = r<<8 | r>>8
		}
		ordered[i] = r
	}
	if !t.WordSwap {
		return ordered
	}
	out := make([]uint16, n)
	for i, r := range ordered {
		out[n-1-i] = r
	}
	return out
}

// DecodeRegisters converts raw registers (as read) to an engineering value.
func (t *Tag) DecodeRegisters(regs []uint16) float64 {
	b := toBytes(t, regs)
	var raw float64
	switch t.DataType {
	case TypeBool:
		r := binary.BigEndian.Uint16(b)
		if t.Bit >= 0 {
			if r&(1<<uint(t.Bit)) != 0 {
				return 1
			}
			return 0
		}
		if r != 0 {
			return 1
		}
		return 0
	case TypeUint16:
		raw = float64(binary.BigEndian.Uint16(b))
	case TypeInt16:
		raw = float64(int16(binary.BigEndian.Uint16(b)))
	case TypeUint32:
		raw = float64(binary.BigEndian.Uint32(b))
	case TypeInt32:
		raw = float64(int32(binary.BigEndian.Uint32(b)))
	case TypeFloat32:
		raw = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
	case TypeFloat64:
		raw = math.Float64frombits(binary.BigEndian.Uint64(b))
	}
	return raw*t.Scale + t.Offset
}

// EncodeRegisters converts an engineering value to the registers to report.
func (t *Tag) EncodeRegisters(v float64) []uint16 {
	if t.DataType != TypeBool && t.Scale != 0 {
		v = (v - t.Offset) / t.Scale
	}
	var b []byte
	switch t.DataType {
	case TypeBool:
		b = make([]byte, 2)
		if v != 0 {
			binary.BigEndian.PutUint16(b, 1)
		}
	case TypeUint16:
		b = binary.BigEndian.AppendUint16(nil, uint16(clampU(v, math.MaxUint16)))
	case TypeInt16:
		b = binary.BigEndian.AppendUint16(nil, uint16(int16(clampS(v, math.MinInt16, math.MaxInt16))))
	case TypeUint32:
		b = binary.BigEndian.AppendUint32(nil, uint32(clampU(v, math.MaxUint32)))
	case TypeInt32:
		b = binary.BigEndian.AppendUint32(nil, uint32(int32(clampS(v, math.MinInt32, math.MaxInt32))))
	case TypeFloat32:
		b = binary.BigEndian.AppendUint32(nil, math.Float32bits(float32(v)))
	case TypeFloat64:
		b = binary.BigEndian.AppendUint64(nil, math.Float64bits(v))
	default:
		b = make([]byte, 2)
	}
	return fromBytes(t, b)
}

func clampU(v, hi float64) float64 {
	v = math.Round(v)
	if v < 0 {
		return 0
	}
	if v > hi {
		return hi
	}
	return v
}

func clampS(v, lo, hi float64) float64 {
	v = math.Round(v)
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
