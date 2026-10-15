package main

import (
	"testing"

	"blueot/scada/internal/model"
)

func testConfig() *model.Config {
	return &model.Config{
		Devices: []model.Device{
			{ID: "sim", Host: "127.0.0.1", Port: 5020, UnitID: 1},
			{ID: "bs", Host: "127.0.0.1", Port: 502, UnitID: 1, IEC: &model.IEC104{Port: 2404, CA: 1}},
		},
		Tags: []model.Tag{
			// modsim breaker: holding status + command, no IEC
			{ID: "Q1_POS", Device: "sim", Area: "holding", Address: 100},
			{ID: "Q1_CMD", Device: "sim", Area: "holding", Address: 200, Writable: true},
			// modsim disconnector: discrete status + coil command
			{ID: "QS1_POS", Device: "sim", Area: "discrete", Address: 0},
			{ID: "QS1_CMD", Device: "sim", Area: "coil", Address: 0, Writable: true},
			// blue-station breaker: discrete status (with IEC) + coil command
			{ID: "G1.CB", Device: "bs", Area: "discrete", Address: 10, CtrlIOA: 5011, Cmd: "C_DC"},
			{ID: "G1.CB.Cmd", Device: "bs", Area: "coil", Address: 10, Writable: true},
		},
	}
}

func TestResolve(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		in         string
		wantStatus string
		wantCmd    string
		wantDev    string
	}{
		{"Q1_POS", "Q1_POS", "Q1_CMD", "sim"},
		{"Q1_CMD", "Q1_POS", "Q1_CMD", "sim"},
		{"QS1_POS", "QS1_POS", "QS1_CMD", "sim"},
		{"QS1_CMD", "QS1_POS", "QS1_CMD", "sim"},
		{"G1.CB", "G1.CB", "G1.CB.Cmd", "bs"},
		{"G1.CB.Cmd", "G1.CB", "G1.CB.Cmd", "bs"},
	}
	for _, c := range cases {
		r, err := resolve(cfg, c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if id(r.status) != c.wantStatus {
			t.Errorf("%s: status = %q, want %q", c.in, id(r.status), c.wantStatus)
		}
		if id(r.cmd) != c.wantCmd {
			t.Errorf("%s: cmd = %q, want %q", c.in, id(r.cmd), c.wantCmd)
		}
		if r.device.ID != c.wantDev {
			t.Errorf("%s: device = %q, want %q", c.in, r.device.ID, c.wantDev)
		}
	}

	if _, err := resolve(cfg, "nope"); err == nil {
		t.Error("expected error for unknown tag")
	}
}

func TestResolveIEC(t *testing.T) {
	cfg := testConfig()
	// From the command tag, the status tag (which carries CtrlIOA) is found.
	r, err := resolve(cfg, "G1.CB.Cmd")
	if err != nil {
		t.Fatal(err)
	}
	if r.status.CtrlIOA != 5011 || r.status.Cmd != "C_DC" {
		t.Errorf("IEC mapping lost: ctrlIoa=%d cmd=%q", r.status.CtrlIOA, r.status.Cmd)
	}
	if r.device.IEC == nil || r.device.IEC.Port != 2404 {
		t.Errorf("device IEC endpoint missing")
	}
}

func TestDefaultValues(t *testing.T) {
	if o, c := defaultValues(model.AreaCoil); o != 0 || c != 1 {
		t.Errorf("coil defaults = %d/%d, want 0/1", o, c)
	}
	if o, c := defaultValues(model.AreaHolding); o != 1 || c != 2 {
		t.Errorf("holding defaults = %d/%d, want 1/2", o, c)
	}
}

func TestParseState(t *testing.T) {
	for _, s := range []string{"close", "closed", "on", "1", "till", "slut", "tillslag"} {
		if v, err := parseState(s); err != nil || !v {
			t.Errorf("%q should be close", s)
		}
	}
	for _, s := range []string{"open", "off", "0", "från", "öppna", "frånslag"} {
		if v, err := parseState(s); err != nil || v {
			t.Errorf("%q should be open", s)
		}
	}
	if _, err := parseState("maybe"); err == nil {
		t.Error("expected error for invalid state")
	}
}

func TestCategory(t *testing.T) {
	cases := []struct {
		id, name, want string
	}{
		{"Q1_POS", "J1 Q1 läge", "brytare"},
		{"QS1_POS", "J1 QS1 läge", "frånskiljare"},
		{"G1.CB", "G1 brytare kraftvärmeverk", "brytare"},
		{"Q0.CB", "Linjebrytare Q0 mot stamnät", "brytare"},
		{"BUS.Coupler.CB", "Sektioneringsbrytare A–B", "brytare"},
		{"CHP.DH.Pump1", "Fjärrvärmepump 1", "pump"},
		{"T1.OilTemp", "T1 oljetemperatur", ""},
		{"Grid.U", "Spänning stamnät", ""},
		{"AVR.USet", "Börvärde spänning 20 kV", ""},
	}
	for _, c := range cases {
		if got := category(&model.Tag{ID: c.id, Name: c.name}); got != c.want {
			t.Errorf("category(%q,%q) = %q, want %q", c.id, c.name, got, c.want)
		}
	}
}

func TestMatchFilter(t *testing.T) {
	ok := map[string]string{
		"": "", "brytare": "brytare", "breaker": "brytare",
		"frånskiljare": "frånskiljare", "disconnector": "frånskiljare",
		"pump": "pump", "pumps": "pump",
	}
	for in, want := range ok {
		if got, valid := matchFilter(in); !valid || got != want {
			t.Errorf("matchFilter(%q) = %q,%v; want %q,true", in, got, valid, want)
		}
	}
	if _, valid := matchFilter("nope"); valid {
		t.Error("matchFilter(nope) should be invalid")
	}
}

func id(t *model.Tag) string {
	if t == nil {
		return ""
	}
	return t.ID
}
