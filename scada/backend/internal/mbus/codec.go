package mbus

import (
	"encoding/binary"
	"fmt"
	"math"

	"blueot/scada/internal/model"
)

// toBytes converts registers to a big-endian byte stream, applying the
// tag's byte and word ordering so the result is always "ABCD..." order.
func toBytes(t *model.Tag, regs []uint16) []byte {
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

// fromBytes is the inverse of toBytes.
func fromBytes(t *model.Tag, b []byte) []uint16 {
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

// decode converts raw registers to an engineering value.
func decode(t *model.Tag, regs []uint16) float64 {
	b := toBytes(t, regs)
	var raw float64
	switch t.DataType {
	case model.TypeBool:
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
	case model.TypeUint16:
		raw = float64(binary.BigEndian.Uint16(b))
	case model.TypeInt16:
		raw = float64(int16(binary.BigEndian.Uint16(b)))
	case model.TypeUint32:
		raw = float64(binary.BigEndian.Uint32(b))
	case model.TypeInt32:
		raw = float64(int32(binary.BigEndian.Uint32(b)))
	case model.TypeFloat32:
		raw = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
	case model.TypeFloat64:
		raw = math.Float64frombits(binary.BigEndian.Uint64(b))
	}
	return raw*t.Scale + t.Offset
}

// encode converts an engineering value to registers for writing.
func encode(t *model.Tag, v float64) ([]uint16, error) {
	if t.DataType != model.TypeBool && t.Scale != 0 {
		v = (v - t.Offset) / t.Scale
	}
	var b []byte
	switch t.DataType {
	case model.TypeBool:
		if t.Bit >= 0 {
			return nil, fmt.Errorf("skrivning av enskild bit i register stöds inte")
		}
		b = make([]byte, 2)
		if v != 0 {
			binary.BigEndian.PutUint16(b, 1)
		}
	case model.TypeUint16:
		if v < 0 || v > math.MaxUint16 {
			return nil, fmt.Errorf("värdet %v ryms inte i uint16", v)
		}
		b = binary.BigEndian.AppendUint16(nil, uint16(math.Round(v)))
	case model.TypeInt16:
		if v < math.MinInt16 || v > math.MaxInt16 {
			return nil, fmt.Errorf("värdet %v ryms inte i int16", v)
		}
		b = binary.BigEndian.AppendUint16(nil, uint16(int16(math.Round(v))))
	case model.TypeUint32:
		if v < 0 || v > math.MaxUint32 {
			return nil, fmt.Errorf("värdet %v ryms inte i uint32", v)
		}
		b = binary.BigEndian.AppendUint32(nil, uint32(math.Round(v)))
	case model.TypeInt32:
		if v < math.MinInt32 || v > math.MaxInt32 {
			return nil, fmt.Errorf("värdet %v ryms inte i int32", v)
		}
		b = binary.BigEndian.AppendUint32(nil, uint32(int32(math.Round(v))))
	case model.TypeFloat32:
		b = binary.BigEndian.AppendUint32(nil, math.Float32bits(float32(v)))
	case model.TypeFloat64:
		b = binary.BigEndian.AppendUint64(nil, math.Float64bits(v))
	default:
		return nil, fmt.Errorf("okänd datatyp %q", t.DataType)
	}
	return fromBytes(t, b), nil
}
