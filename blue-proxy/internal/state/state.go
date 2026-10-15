// Package state is the proxy's memory: per point it keeps the value collected
// from the upstream source, the value reported downstream to the master, a
// "hold" flag that freezes the reported value, and the last command seen.
package state

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// Point is the live state of one tag.
type Point struct {
	Key       string
	Name      string
	Category  string
	Proto     string // modbus or iec104
	Unit      string
	Decimals  int
	Writable  bool
	StateText map[string]string

	Collected    float64
	HasCollected bool
	CollectedTS  time.Time

	Reported    float64
	HasReported bool
	ReportedTS  time.Time

	Held      bool
	HeldValue float64

	Blocked bool // commands to this point are rejected, not forwarded

	LastCmd        float64
	HasCmd         bool
	LastCmdTS      time.Time
	LastCmdText    string
	LastCmdBlocked bool
}

// Store is the thread-safe collection of points.
type Store struct {
	mu     sync.RWMutex
	points map[string]*Point
	order  []string
}

// New creates an empty store.
func New() *Store { return &Store{points: map[string]*Point{}} }

// Register adds a point. If the key already exists (e.g. the same tag is proxied
// over both protocols) the protocol label is merged and the rest is kept.
func (s *Store) Register(p *Point) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ex, ok := s.points[p.Key]; ok {
		if p.Proto != "" && !strings.Contains(ex.Proto, p.Proto) {
			ex.Proto += "+" + p.Proto
		}
		return
	}
	s.points[p.Key] = p
	s.order = append(s.order, p.Key)
}

// Report records a value collected from the source and returns the value that
// should be reported downstream (the collected value, or the frozen value when
// the point is held).
func (s *Store) Report(key string, collected float64) (emit float64, held bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.points[key]
	if p == nil {
		return collected, false
	}
	p.Collected, p.HasCollected, p.CollectedTS = collected, true, time.Now()
	if p.Held {
		emit = p.HeldValue
	} else {
		emit = collected
	}
	p.Reported, p.HasReported, p.ReportedTS = emit, true, time.Now()
	return emit, p.Held
}

// RecordCommand stores the last command seen for a point. blocked marks that
// the command was rejected by the proxy instead of being forwarded.
func (s *Store) RecordCommand(key string, v float64, text string, blocked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.points[key]; p != nil {
		p.LastCmd, p.HasCmd, p.LastCmdTS = v, true, time.Now()
		p.LastCmdText, p.LastCmdBlocked = text, blocked
	}
}

// IsBlocked reports whether commands to the point are blocked.
func (s *Store) IsBlocked(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := s.points[key]
	return p != nil && p.Blocked
}

// SetHold freezes or releases one point. When freezing, the currently reported
// value (or the collected value if nothing was reported yet) becomes the frozen
// value.
func (s *Store) SetHold(key string, on bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.points[key]
	if p == nil {
		return false
	}
	setHold(p, on)
	return true
}

// SetHoldCategory freezes or releases all points of a category ("all" for
// every point). It returns how many points were affected.
func (s *Store) SetHoldCategory(cat string, on bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, p := range s.points {
		if cat == "all" || p.Category == cat {
			setHold(p, on)
			n++
		}
	}
	return n
}

func setHold(p *Point, on bool) {
	if on && !p.Held {
		if p.HasReported {
			p.HeldValue = p.Reported
		} else {
			p.HeldValue = p.Collected
		}
	}
	p.Held = on
}

// SetBlock blocks or unblocks commands to one point.
func (s *Store) SetBlock(key string, on bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.points[key]
	if p == nil {
		return false
	}
	p.Blocked = on
	return true
}

// SetBlockCategory blocks or unblocks commands to all points of a category
// ("all" for every point). It returns how many points were affected.
func (s *Store) SetBlockCategory(cat string, on bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, p := range s.points {
		if cat == "all" || p.Category == cat {
			p.Blocked = on
			n++
		}
	}
	return n
}

// View is a snapshot of a point for the web UI.
type View struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Proto        string   `json:"proto"`
	Unit         string   `json:"unit"`
	Writable     bool     `json:"writable"`
	Held         bool     `json:"held"`
	Blocked      bool     `json:"blocked"`
	Collected    *float64 `json:"collected"`
	CollectedTxt string   `json:"collectedTxt"`
	Reported     *float64 `json:"reported"`
	ReportedTxt  string   `json:"reportedTxt"`
	Diff         bool     `json:"diff"`
	LastCmd      string   `json:"lastCmd"`
	LastCmdBad   bool     `json:"lastCmdBad"`
}

// Snapshot returns all points, in registration order.
func (s *Store) Snapshot() []View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]View, 0, len(s.order))
	for _, k := range s.order {
		p := s.points[k]
		v := View{
			Key: p.Key, Name: p.Name, Category: p.Category, Proto: p.Proto,
			Unit: p.Unit, Writable: p.Writable, Held: p.Held, Blocked: p.Blocked,
		}
		if p.HasCollected {
			c := p.Collected
			v.Collected = &c
			v.CollectedTxt = p.format(c)
		}
		if p.HasReported {
			r := p.Reported
			v.Reported = &r
			v.ReportedTxt = p.format(r)
		}
		v.Diff = p.HasCollected && p.HasReported && p.Collected != p.Reported
		if p.HasCmd {
			v.LastCmd = p.LastCmdText + "  @ " + p.LastCmdTS.Format("15:04:05")
			if p.LastCmdBlocked {
				v.LastCmd = "⛔ BLOCKERAD: " + v.LastCmd
				v.LastCmdBad = true
			}
		}
		out = append(out, v)
	}
	return out
}

// Categories returns the categories present with their point counts.
func (s *Store) Categories() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := map[string]int{}
	for _, p := range s.points {
		m[p.Category]++
	}
	return m
}

func (p *Point) format(v float64) string {
	if p.StateText != nil {
		if s, ok := p.StateText[strconv.FormatFloat(v, 'f', -1, 64)]; ok {
			return s
		}
	}
	s := strconv.FormatFloat(v, 'f', p.Decimals, 64)
	if p.Unit != "" {
		s += " " + p.Unit
	}
	return s
}
