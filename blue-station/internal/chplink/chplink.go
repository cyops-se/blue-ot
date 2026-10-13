// Package chplink couples the generator bay G1 to the CHP plant simulated by
// blue-chp. It models the 20 kV cable between the two sites, not a SCADA
// interface: every period the busbar state at G1 is posted to the plant and
// the generator output in the response is fed into the station.
package chplink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"blue-station/internal/station"
)

type linkIn struct {
	BusLive   bool    `json:"busLive"`
	BusKV     float64 `json:"busKV"`
	Hz        float64 `json:"hz"`
	BayClosed bool    `json:"bayClosed"`
}

type linkOut struct {
	GenOnline bool    `json:"genOnline"`
	P         float64 `json:"p"`
	Q         float64 `json:"q"`
}

// Run exchanges state with the plant at baseURL (e.g. http://127.0.0.1:8082)
// until ctx is cancelled. logf is called when the link goes up or down.
func Run(ctx context.Context, st *station.Station, baseURL string, period time.Duration, logf func(string, ...any)) {
	url := strings.TrimRight(baseURL, "/") + "/api/link"
	client := &http.Client{Timeout: period}
	t := time.NewTicker(period)
	defer t.Stop()
	up := false
	for {
		out, err := exchange(ctx, client, url, st)
		if err != nil {
			st.SetGenerator(false, false, 0, 0)
			if up {
				logf("Länk till kraftvärmeverket bruten: %v", err)
			}
			up = false
		} else {
			st.SetGenerator(true, out.GenOnline, out.P, out.Q)
			if !up {
				logf("Länk till kraftvärmeverket upprättad (%s)", baseURL)
			}
			up = true
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func exchange(ctx context.Context, client *http.Client, url string, st *station.Station) (linkOut, error) {
	var out linkOut
	in := linkIn{}
	in.BusLive, in.BusKV, in.Hz, in.BayClosed = st.GridSide()
	body, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("HTTP %s", resp.Status)
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}
