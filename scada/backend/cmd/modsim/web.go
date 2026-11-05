package main

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
)

//go:embed index.html
var indexHTML []byte

// pointJSON mirrors the shape used by blue-station and blue-chp so the three
// local dashboards can share the same index.html.
type pointJSON struct {
	ID      string `json:"id"`
	Group   string `json:"group"`
	Desc    string `json:"desc"`
	Unit    string `json:"unit"`
	Kind    string `json:"kind"`
	Value   any    `json:"value"`
	Text    string `json:"text"`
	Modbus  string `json:"modbus"`
	IEC     string `json:"iec"`
	IECCtrl string `json:"iecCtrl"`
	Ctrl    bool   `json:"ctrl"`
}

type controlReq struct {
	ID string `json:"id"`
	On bool   `json:"on"`
}

func webHandler(s *sim) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.state())
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
		if err := s.control(req.ID, req.On); err != nil {
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

type stateJSON struct {
	Time   time.Time   `json:"time"`
	Points []pointJSON `json:"points"`
	Events []evt       `json:"events"`
}

// state builds a snapshot of every point for the dashboard.
func (s *sim) state() stateJSON {
	s.mu.Lock()
	defer s.mu.Unlock()

	pts := make([]pointJSON, 0, 4*feeders+3)
	for i := 0; i < feeders; i++ {
		group := fmt.Sprintf("Fack J%d", i+1)
		text, closed := breakerText(s.breaker[i])
		pts = append(pts, pointJSON{
			ID: fmt.Sprintf("Q%d", i+1), Group: group, Desc: fmt.Sprintf("J%d Q%d brytare", i+1, i+1),
			Kind: "breaker", Value: closed, Text: text,
			Modbus: fmt.Sprintf("HR %d läge, HR %d manöver", 100+i, 200+i), IEC: "–", IECCtrl: "–", Ctrl: true,
		})
		pts = append(pts, pointJSON{
			ID: fmt.Sprintf("QS%d", i+1), Group: group, Desc: fmt.Sprintf("J%d QS%d frånskiljare", i+1, i+1),
			Kind: "switch", Value: s.disc[i], Text: discText(s.disc[i]),
			Modbus: fmt.Sprintf("DI %d, Coil %d", i, i), IEC: "–", IECCtrl: "–", Ctrl: true,
		})
		pts = append(pts, meas(fmt.Sprintf("J%d_U", i+1), group, fmt.Sprintf("J%d spänning", i+1), "kV", getFloat(s.input[:], 6*i), fmt.Sprintf("IR %d–%d", 6*i, 6*i+1)))
		pts = append(pts, meas(fmt.Sprintf("J%d_I", i+1), group, fmt.Sprintf("J%d ström", i+1), "A", getFloat(s.input[:], 6*i+2), fmt.Sprintf("IR %d–%d", 6*i+2, 6*i+3)))
		pts = append(pts, meas(fmt.Sprintf("J%d_P", i+1), group, fmt.Sprintf("J%d aktiv effekt", i+1), "MW", getFloat(s.input[:], 6*i+4), fmt.Sprintf("IR %d–%d", 6*i+4, 6*i+5)))
	}

	pts = append(pts, meas("UB", "Station", "Samlingsskena spänning", "kV", getFloat(s.input[:], 32), "IR 32–33"))
	pts = append(pts, meas("F", "Station", "Frekvens", "Hz", getFloat(s.input[:], 30), "IR 30–31"))
	pts = append(pts, meas("T1_TEMP", "Station", "T1 oljetemperatur", "°C", float64(int16(s.input[40]))/10, "IR 40"))

	ev := s.events
	if len(ev) > 50 {
		ev = ev[len(ev)-50:]
	}
	out := make([]evt, 0, len(ev))
	return stateJSON{Time: time.Now(), Points: pts, Events: append(out, ev...)}
}

func meas(id, group, desc, unit string, v float64, ref string) pointJSON {
	return pointJSON{
		ID: id, Group: group, Desc: desc, Unit: unit, Kind: "measurement",
		Value: math.Round(v*1000) / 1000, Text: fmt.Sprintf("%.2f %s", v, unit),
		Modbus: ref, IEC: "–", IECCtrl: "–",
	}
}

// control handles a web manoeuvre of a breaker (Qn) or disconnector (QSn).
func (s *sim) control(id string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < feeders; i++ {
		switch id {
		case fmt.Sprintf("Q%d", i+1):
			target := uint16(1) // frånslag
			if on {
				target = 2 // tillslag
			}
			if s.breaker[i] != target && s.breaker[i] != 0 {
				s.operateBreaker(i, target)
			}
			return nil
		case fmt.Sprintf("QS%d", i+1):
			if err := s.setDisc(i, on); err != nil {
				return fmt.Errorf("QS%d förreglad: Q%d måste vara öppen", i+1, i+1)
			}
			return nil
		}
	}
	return fmt.Errorf("okänd punkt %s", id)
}

func breakerText(dp uint16) (text string, closed bool) {
	switch dp {
	case 2:
		return "TILL", true
	case 1:
		return "FRÅN", false
	case 3:
		return "FEL", false
	}
	return "MELLANLÄGE", false
}

func breakerCmdText(target uint16) string {
	if target == 2 {
		return "TILLSLAG"
	}
	return "FRÅNSLAG"
}

func discText(closed bool) string {
	if closed {
		return "SLUTEN"
	}
	return "ÖPPEN"
}

// getFloat reads a big-endian float32 stored across two registers.
func getFloat(regs []uint16, addr int) float64 {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:], regs[addr])
	binary.BigEndian.PutUint16(b[2:], regs[addr+1])
	return float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
}
