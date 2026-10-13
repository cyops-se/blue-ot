package mbus

import (
	"math"
	"testing"

	"blueot/scada/internal/model"
)

func TestRoundTrip(t *testing.T) {
	cases := []struct {
		tag model.Tag
		v   float64
	}{
		{model.Tag{DataType: model.TypeFloat32, Scale: 1}, 10.5},
		{model.Tag{DataType: model.TypeFloat32, Scale: 1, WordSwap: true}, -3.25},
		{model.Tag{DataType: model.TypeFloat32, Scale: 1, ByteSwap: true, WordSwap: true}, 1234.5},
		{model.Tag{DataType: model.TypeFloat64, Scale: 1, WordSwap: true}, math.Pi},
		{model.Tag{DataType: model.TypeInt16, Scale: 0.1}, -12.3},
		{model.Tag{DataType: model.TypeUint32, Scale: 1, Offset: 100}, 70100},
		{model.Tag{DataType: model.TypeInt32, Scale: 1}, -70000},
		{model.Tag{DataType: model.TypeUint16, Scale: 1}, 2},
		{model.Tag{DataType: model.TypeBool, Bit: -1, Scale: 1}, 1},
	}
	for _, c := range cases {
		regs, err := encode(&c.tag, c.v)
		if err != nil {
			t.Fatalf("%+v: %v", c.tag, err)
		}
		if len(regs) != c.tag.RegisterCount() {
			t.Fatalf("%s: got %d registers", c.tag.DataType, len(regs))
		}
		got := decode(&c.tag, regs)
		if math.Abs(got-c.v) > 1e-4*math.Max(1, math.Abs(c.v)) {
			t.Errorf("%+v: got %v want %v", c.tag, got, c.v)
		}
	}
}

func TestFloat32WordOrder(t *testing.T) {
	// 10.5 as IEEE754 float32 = 0x41280000
	abcd := model.Tag{DataType: model.TypeFloat32, Scale: 1}
	if v := decode(&abcd, []uint16{0x4128, 0x0000}); v != 10.5 {
		t.Errorf("ABCD: got %v", v)
	}
	cdab := model.Tag{DataType: model.TypeFloat32, Scale: 1, WordSwap: true}
	if v := decode(&cdab, []uint16{0x0000, 0x4128}); v != 10.5 {
		t.Errorf("CDAB: got %v", v)
	}
}

func TestBitExtraction(t *testing.T) {
	tag := model.Tag{DataType: model.TypeBool, Bit: 3, Scale: 1}
	if v := decode(&tag, []uint16{0x0008}); v != 1 {
		t.Errorf("bit 3 set: got %v", v)
	}
	if v := decode(&tag, []uint16{0x0007}); v != 0 {
		t.Errorf("bit 3 clear: got %v", v)
	}
}

func TestBuildBlocks(t *testing.T) {
	tags := []model.Tag{
		{ID: "a", Area: model.AreaInput, Address: 0, DataType: model.TypeFloat32},
		{ID: "b", Area: model.AreaInput, Address: 10, DataType: model.TypeFloat32},
		{ID: "c", Area: model.AreaInput, Address: 200, DataType: model.TypeUint16},
		{ID: "d", Area: model.AreaCoil, Address: 3, DataType: model.TypeBool},
	}
	blocks := buildBlocks(tags)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %+v", len(blocks), blocks)
	}
	for _, b := range blocks {
		if b.area == model.AreaInput && b.start == 0 && b.count != 12 {
			t.Errorf("merged block count = %d, want 12", b.count)
		}
	}
}
