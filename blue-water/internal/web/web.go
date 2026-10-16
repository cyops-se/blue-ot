// Package web serves a small operator dashboard and a JSON API.
package web

import (
	_ "embed"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"blue-water/internal/plant"
)

//go:embed index.html
var index []byte

type pointJSON struct {
	ID      string    `json:"id"`
	Group   string    `json:"group"`
	Desc    string    `json:"desc"`
	Unit    string    `json:"unit"`
	Kind    string    `json:"kind"`
	Value   any       `json:"value"`
	Text    string    `json:"text"`
	Time    time.Time `json:"time"`
	Modbus  string    `json:"modbus"`
	IEC     string    `json:"iec"`
	IECCtrl string    `json:"iecCtrl"`
	Ctrl    bool      `json:"ctrl"`
}

type controlReq struct {
	ID    string  `json:"id"`
	On    bool    `json:"on"`
	Value float64 `json:"value"`
}

func Handler(st *plant.Plant) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		vals, _ := st.Snapshot()
		pts := make([]pointJSON, 0, len(vals))
		for i, p := range st.Points() {
			v := vals[i]
			var val any
			switch p.Kind {
			case plant.KindBreaker:
				val = v.DP == 2
			case plant.KindSwitch, plant.KindStatus, plant.KindPulse:
				val = v.B
			default:
				val = math.Round(v.F*1000) / 1000
			}
			pts = append(pts, pointJSON{
				ID: p.ID, Group: p.Group, Desc: p.Desc, Unit: p.Unit, Kind: p.Kind.String(),
				Value: val, Text: p.StateText(v), Time: v.T,
				Modbus: p.ModbusRef(), IEC: p.IECMonitorRef(), IECCtrl: p.IECControlRef(), Ctrl: p.Controllable(),
			})
		}
		ev := st.Events()
		if len(ev) > 50 {
			ev = ev[len(ev)-50:]
		}
		writeJSON(w, http.StatusOK, map[string]any{"time": time.Now(), "points": pts, "events": ev})
	})

	mux.HandleFunc("/api/control", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req controlReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		p := st.PointByID(req.ID)
		if p == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "okänd punkt " + req.ID})
			return
		}
		if err := st.Control(p, plant.Ctrl{On: req.On, F: req.Value}, "Webb "+r.RemoteAddr); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
