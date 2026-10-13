// Package store persists the communication configuration and the HMI views
// as JSON files in a data directory.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"blueot/scada/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,64}$`)
)

// Store gives synchronized access to configuration and views.
type Store struct {
	dir string
	mu  sync.RWMutex
	cfg model.Config
}

// Open loads (or initialises) the store in dir.
func Open(dir string) (*Store, error) {
	for _, d := range []string{dir, filepath.Join(dir, "views"), filepath.Join(dir, "images")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	s := &Store{dir: dir}
	b, err := os.ReadFile(s.configPath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.cfg = model.Config{Devices: []model.Device{}, Tags: []model.Tag{}}
		return s, s.saveConfig()
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(b, &s.cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.configPath(), err)
	}
	for i := range s.cfg.Tags {
		normalizeTag(&s.cfg.Tags[i])
	}
	return s, nil
}

func (s *Store) configPath() string { return filepath.Join(s.dir, "config.json") }

// ImageDir is where uploaded static graphics are stored.
func (s *Store) ImageDir() string { return filepath.Join(s.dir, "images") }

func (s *Store) saveConfig() error {
	return writeJSON(s.configPath(), s.cfg)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Config returns a deep-enough copy of the configuration.
func (s *Store) Config() model.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := model.Config{
		Devices: append([]model.Device(nil), s.cfg.Devices...),
		Tags:    append([]model.Tag(nil), s.cfg.Tags...),
	}
	return c
}

// Tag returns the tag with the given id.
func (s *Store) Tag(id string) (model.Tag, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.cfg.Tags {
		if t.ID == id {
			return t, true
		}
	}
	return model.Tag{}, false
}

// ---- devices ----

func validateDevice(d *model.Device) error {
	if !idPattern.MatchString(d.ID) {
		return fmt.Errorf("ogiltigt id %q (tillåtna tecken: A-Z a-z 0-9 _ . -)", d.ID)
	}
	if d.Host == "" {
		return errors.New("host saknas")
	}
	if d.Port <= 0 || d.Port > 65535 {
		d.Port = 502
	}
	if d.UnitID < 0 || d.UnitID > 255 {
		return errors.New("unitId måste vara 0-255")
	}
	if d.PollMs < 100 {
		d.PollMs = 1000
	}
	if d.TimeoutMs < 100 {
		d.TimeoutMs = 2000
	}
	if d.Name == "" {
		d.Name = d.ID
	}
	return nil
}

// UpsertDevice creates or replaces a device. If oldID is non-empty and
// differs from d.ID the device is renamed and its tags follow.
func (s *Store) UpsertDevice(oldID string, d model.Device) (model.Device, error) {
	if err := validateDevice(&d); err != nil {
		return d, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, x := range s.cfg.Devices {
		if x.ID == oldID && oldID != "" {
			idx = i
		} else if x.ID == d.ID {
			return d, fmt.Errorf("enhet %q finns redan", d.ID)
		}
	}
	if oldID != "" && idx < 0 {
		return d, ErrNotFound
	}
	if idx >= 0 {
		s.cfg.Devices[idx] = d
		if oldID != d.ID {
			for i := range s.cfg.Tags {
				if s.cfg.Tags[i].Device == oldID {
					s.cfg.Tags[i].Device = d.ID
				}
			}
		}
	} else {
		s.cfg.Devices = append(s.cfg.Devices, d)
	}
	return d, s.saveConfig()
}

// DeleteDevice removes a device; it fails if tags still reference it.
func (s *Store) DeleteDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.cfg.Tags {
		if t.Device == id {
			return fmt.Errorf("enheten används av tagg %q", t.ID)
		}
	}
	for i, d := range s.cfg.Devices {
		if d.ID == id {
			s.cfg.Devices = append(s.cfg.Devices[:i], s.cfg.Devices[i+1:]...)
			return s.saveConfig()
		}
	}
	return ErrNotFound
}

// ---- tags ----

func normalizeTag(t *model.Tag) {
	if t.Scale == 0 {
		t.Scale = 1
	}
	if t.IsBitArea() {
		t.DataType = model.TypeBool
		t.Bit = -1
	}
	if t.DataType == "" {
		t.DataType = model.TypeUint16
	}
	if t.DataType != model.TypeBool {
		t.Bit = -1
	}
	if t.Area == model.AreaDiscrete || t.Area == model.AreaInput {
		t.Writable = false
	}
}

func (s *Store) validateTag(t *model.Tag) error {
	if !idPattern.MatchString(t.ID) {
		return fmt.Errorf("ogiltigt id %q (tillåtna tecken: A-Z a-z 0-9 _ . -)", t.ID)
	}
	switch t.Area {
	case model.AreaCoil, model.AreaDiscrete, model.AreaInput, model.AreaHolding:
	default:
		return fmt.Errorf("okänt område %q", t.Area)
	}
	switch t.DataType {
	case "", model.TypeBool, model.TypeUint16, model.TypeInt16, model.TypeUint32,
		model.TypeInt32, model.TypeFloat32, model.TypeFloat64:
	default:
		return fmt.Errorf("okänd datatyp %q", t.DataType)
	}
	if t.Address < 0 || t.Address > 65535 {
		return errors.New("adress måste vara 0-65535")
	}
	if t.Bit > 15 {
		return errors.New("bit måste vara -1..15")
	}
	found := false
	for _, d := range s.cfg.Devices {
		if d.ID == t.Device {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("okänd enhet %q", t.Device)
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	normalizeTag(t)
	return nil
}

// UpsertTag creates or replaces a tag (renaming when oldID != t.ID).
func (s *Store) UpsertTag(oldID string, t model.Tag) (model.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateTag(&t); err != nil {
		return t, err
	}
	idx := -1
	for i, x := range s.cfg.Tags {
		if x.ID == oldID && oldID != "" {
			idx = i
		} else if x.ID == t.ID {
			return t, fmt.Errorf("tagg %q finns redan", t.ID)
		}
	}
	if oldID != "" && idx < 0 {
		return t, ErrNotFound
	}
	if idx >= 0 {
		s.cfg.Tags[idx] = t
	} else {
		s.cfg.Tags = append(s.cfg.Tags, t)
	}
	return t, s.saveConfig()
}

// DeleteTag removes a tag.
func (s *Store) DeleteTag(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.cfg.Tags {
		if t.ID == id {
			s.cfg.Tags = append(s.cfg.Tags[:i], s.cfg.Tags[i+1:]...)
			return s.saveConfig()
		}
	}
	return ErrNotFound
}

// ---- views ----

func (s *Store) viewPath(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", ErrNotFound
	}
	return filepath.Join(s.dir, "views", id+".json"), nil
}

// Views lists all stored views sorted by name.
func (s *Store) Views() ([]model.ViewSummary, error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "views", "*.json"))
	if err != nil {
		return nil, err
	}
	out := []model.ViewSummary{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var v model.View
		if json.Unmarshal(b, &v) != nil {
			continue
		}
		out = append(out, model.ViewSummary{ID: v.ID, Name: v.Name, Updated: v.Updated})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// View loads a view.
func (s *Store) View(id string) (model.View, error) {
	var v model.View
	p, err := s.viewPath(id)
	if err != nil {
		return v, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return v, ErrNotFound
	} else if err != nil {
		return v, err
	}
	err = json.Unmarshal(b, &v)
	if v.Elements == nil {
		v.Elements = []json.RawMessage{}
	}
	return v, err
}

// SaveView creates (empty id) or updates a view.
func (s *Store) SaveView(v model.View) (model.View, error) {
	if v.ID == "" {
		v.ID = newID()
	}
	p, err := s.viewPath(v.ID)
	if err != nil {
		return v, fmt.Errorf("ogiltigt vy-id %q", v.ID)
	}
	if v.Name == "" {
		v.Name = "Ny vy"
	}
	if v.Width <= 0 {
		v.Width = 1600
	}
	if v.Height <= 0 {
		v.Height = 900
	}
	if v.Grid <= 0 {
		v.Grid = 10
	}
	if v.Elements == nil {
		v.Elements = []json.RawMessage{}
	}
	v.Updated = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return v, writeJSON(p, v)
}

// DeleteView removes a view.
func (s *Store) DeleteView(id string) error {
	p, err := s.viewPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else {
		return err
	}
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
