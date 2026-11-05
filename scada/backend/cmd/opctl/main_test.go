package main

import (
	"errors"
	"fmt"
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

func TestOperateTarget(t *testing.T) {
	cases := []struct {
		category  string
		args      []string
		wantWant  string
		wantState string
		wantBulk  bool
		wantErr   bool
	}{
		{"", []string{"Q1_POS", "open"}, "", "", false, false}, // single tag
		{"", []string{"all", "open"}, "", "open", true, false}, // all points
		{"", []string{"type:brytare", "close"}, "brytare", "close", true, false},
		{"", []string{"type:pump", "open"}, "pump", "open", true, false},
		{"brytare", []string{"open"}, "brytare", "open", true, false}, // -category
		{"disconnector", []string{"close"}, "frånskiljare", "close", true, false},
		{"", []string{"type:nope", "open"}, "", "", true, true}, // bad type
		{"nope", []string{"open"}, "", "", true, true},          // bad -category
		{"brytare", []string{}, "", "", true, true},             // missing state
		{"brytare", []string{"a", "b"}, "", "", true, true},     // too many args
	}
	for _, c := range cases {
		want, state, bulk, err := operateTarget(c.category, c.args)
		if (err != nil) != c.wantErr {
			t.Errorf("operateTarget(%q,%v) err = %v, wantErr %v", c.category, c.args, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if want != c.wantWant || state != c.wantState || bulk != c.wantBulk {
			t.Errorf("operateTarget(%q,%v) = (%q,%q,%v); want (%q,%q,%v)",
				c.category, c.args, want, state, bulk, c.wantWant, c.wantState, c.wantBulk)
		}
	}
}

func TestOperability(t *testing.T) {
	cfg := testConfig()
	byID := indexByID(cfg)
	get := func(id string) *model.Tag { return byID[id] }
	cases := []struct {
		id    string
		proto []string
		cmd   string
	}{
		{"Q1_POS", []string{"modbus"}, "Q1_CMD"},             // holding command, no IEC
		{"QS1_POS", []string{"modbus"}, "QS1_CMD"},           // coil command
		{"G1.CB", []string{"modbus", "iec104"}, "G1.CB.Cmd"}, // coil command + IEC ctrlIoa
	}
	for _, c := range cases {
		protos, cmd := operability(cfg, byID, get(c.id))
		if cmd != c.cmd {
			t.Errorf("operability(%s) cmd = %q, want %q", c.id, cmd, c.cmd)
		}
		if len(protos) != len(c.proto) {
			t.Errorf("operability(%s) protos = %v, want %v", c.id, protos, c.proto)
			continue
		}
		for i := range protos {
			if protos[i] != c.proto[i] {
				t.Errorf("operability(%s) protos = %v, want %v", c.id, protos, c.proto)
				break
			}
		}
	}
}

func TestCanonProtocol(t *testing.T) {
	for _, in := range []string{"modbus", "MODBUS"} {
		if p, err := canonProtocol(in); err != nil || p != "modbus" {
			t.Errorf("canonProtocol(%q) = %q,%v", in, p, err)
		}
	}
	for _, in := range []string{"iec104", "iec", "104"} {
		if p, err := canonProtocol(in); err != nil || p != "iec104" {
			t.Errorf("canonProtocol(%q) = %q,%v", in, p, err)
		}
	}
	if _, err := canonProtocol("nope"); err == nil {
		t.Error("canonProtocol(nope) should error")
	}
}

func TestConnError(t *testing.T) {
	base := errors.New("connection refused")
	ce := &connError{base}
	if !isConnError(ce) {
		t.Error("connError should be recognised")
	}
	// Still recognised when wrapped further up the chain.
	if !isConnError(fmt.Errorf("point X: %w", ce)) {
		t.Error("wrapped connError should be recognised")
	}
	// A plain (per-point) error is not a connection error.
	if isConnError(fmt.Errorf("illegal data value")) {
		t.Error("plain error should not be a connError")
	}
	if !errors.Is(ce, base) {
		t.Error("connError should unwrap to its cause")
	}
}

func TestConnEndpoint(t *testing.T) {
	dev := model.Device{Host: "10.0.0.5", Port: 502, IEC: &model.IEC104{Port: 2404}}
	if got := connEndpoint(dev, "modbus"); got != "10.0.0.5:502" {
		t.Errorf("modbus endpoint = %q, want 10.0.0.5:502", got)
	}
	if got := connEndpoint(dev, "iec104"); got != "10.0.0.5:2404" {
		t.Errorf("iec104 endpoint = %q, want 10.0.0.5:2404", got)
	}
	// Without an IEC endpoint the device port is used.
	noIEC := model.Device{Host: "h", Port: 1502}
	if got := connEndpoint(noIEC, "iec104"); got != "h:1502" {
		t.Errorf("fallback endpoint = %q, want h:1502", got)
	}
}

func id(t *model.Tag) string {
	if t == nil {
		return ""
	}
	return t.ID
}
