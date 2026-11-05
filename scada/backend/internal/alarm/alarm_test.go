package alarm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"blueot/scada/internal/model"
	"blueot/scada/internal/realtime"
	"blueot/scada/internal/store"
)

const testConfig = `{
  "devices": [{"id": "d", "name": "Enhet", "host": "127.0.0.1", "port": 502, "enabled": true}],
  "tags": [
    {"id": "F1.CB", "name": "F1 brytare", "device": "d", "area": "discrete", "dataType": "bool",
     "stateText": {"0": "Från", "1": "Till"}, "alarm": {"values": [0], "uncommanded": true}},
    {"id": "F1.Trip", "name": "F1 skydd", "device": "d", "area": "discrete", "dataType": "bool",
     "stateText": {"0": "Normal", "1": "Utlöst"}, "alarm": {"values": [1]}}
  ]
}`

func setup(t *testing.T) (*Engine, *realtime.Hub) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := realtime.New()
	return New(st, h), h
}

func set(h *realtime.Hub, tag string, v float64) {
	h.Update([]model.Value{{Tag: tag, V: v, Quality: model.QualityGood, TS: time.Now()}})
}

func alarm(h *realtime.Hub, id string) *model.Alarm {
	for _, a := range h.Alarms() {
		if a.ID == id {
			return &a
		}
	}
	return nil
}

func TestUncommanded(t *testing.T) {
	e, h := setup(t)
	set(h, "F1.CB", 0) // initial state is not a change
	if alarm(h, "F1.CB") != nil {
		t.Fatal("larm vid uppstart")
	}
	set(h, "F1.CB", 1)
	e.Commanded("F1.CB.Cmd", 0)
	set(h, "F1.CB", 0)
	if alarm(h, "F1.CB") != nil {
		t.Fatal("larm vid beordrat frånslag")
	}
	set(h, "F1.CB", 1)
	set(h, "F1.CB", 0)
	a := alarm(h, "F1.CB")
	if a == nil || !a.Active || a.Acked {
		t.Fatalf("ej beordrat frånslag: %+v", a)
	}
	h.AckAlarms(nil, "op")
	if a = alarm(h, "F1.CB"); a == nil || !a.Acked {
		t.Fatalf("kvitterat aktivt larm ska stå kvar: %+v", a)
	}
	set(h, "F1.CB", 1)
	if alarm(h, "F1.CB") != nil {
		t.Fatal("kvitterat larm som återgått ska tas bort")
	}
}

func TestStateAlarm(t *testing.T) {
	_, h := setup(t)
	set(h, "F1.Trip", 1)
	a := alarm(h, "F1.Trip")
	if a == nil || a.Message != "Utlöst" {
		t.Fatalf("larm: %+v", a)
	}
	if a.Station != "Enhet" {
		t.Errorf("station = %q, vill ha \"Enhet\"", a.Station)
	}
	set(h, "F1.Trip", 0)
	if a = alarm(h, "F1.Trip"); a == nil || a.Active {
		t.Fatalf("okvitterat larm som återgått ska stå kvar: %+v", a)
	}
	h.AckAlarms([]string{"F1.Trip"}, "op")
	if alarm(h, "F1.Trip") != nil {
		t.Fatal("larmet ska tas bort efter kvittering")
	}
}

func TestComm(t *testing.T) {
	_, h := setup(t)
	h.SetDevice(model.DeviceStatus{Device: "d", Connected: false, Error: "timeout"})
	if a := alarm(h, model.CommAlarmID("d")); a == nil || a.Source != "Enhet" {
		t.Fatalf("kommunikationslarm: %+v", a)
	}
	h.SetDevice(model.DeviceStatus{Device: "d", Connected: true})
	if a := alarm(h, model.CommAlarmID("d")); a == nil || a.Active {
		t.Fatalf("kommunikationslarm efter återgång: %+v", a)
	}
}
