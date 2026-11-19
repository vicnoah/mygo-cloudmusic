package eq

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Preset is a named equalizer curve, built in or saved by the listener.
type Preset struct {
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Preamp  float64   `json:"preamp"`
	Bands   []float64 `json:"bands"`
	Builtin bool      `json:"-"`
	// Label is what the interface shows, in Chinese for the built-ins.
	Label string `json:"-"`
}

// Builtins returns the presets the app ships with, in the order the
// interface lists them.
func Builtins() []Preset {
	presets := []struct {
		name, label string
		gains       []float64
	}{
		{"Flat", "平直", []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"Rock", "摇滚", []float64{5, 4, 3, 1, -1, -1, 0, 2, 3, 4, 4, 4, 3, 2, 2}},
		{"Pop", "流行", []float64{-1, -1, 0, 2, 4, 4, 2, 0, -1, -1, 0, 1, 2, 3, 3}},
		{"Jazz", "爵士", []float64{4, 3, 1, 2, -2, -2, 0, 2, 3, 3, 3, 4, 4, 4, 4}},
		{"Classical", "古典", []float64{5, 4, 3, 2, -1, -1, 0, 2, 3, 3, 3, 2, 2, 2, -1}},
		{"Bass Boost", "低音增强", []float64{6, 5, 4, 3, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"Treble Boost", "高音增强", []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 3, 4, 5, 6, 6}},
		{"Vocal", "人声", []float64{-2, -3, -3, 1, 4, 4, 4, 3, 2, 1, 0, -1, -2, -2, -3}},
		{"Electronic", "电子", []float64{5, 4, 2, 0, -2, -2, 0, 2, 3, 4, 4, 3, 4, 4, 3}},
		{"Acoustic", "原声", []float64{5, 4, 3, 1, 2, 1, 1, 2, 2, 2, 1, 2, 2, 3, 2}},
		{"R&B", "R&B", []float64{3, 6, 5, 2, -2, -2, 2, 3, 2, 2, 3, 3, 3, 3, 4}},
		{"Loudness", "响度", []float64{6, 5, 3, 0, -1, -1, -1, -1, 0, 1, 2, 4, 5, 5, 3}},
	}
	out := make([]Preset, len(presets))
	for i, p := range presets {
		out[i] = Preset{
			Name:    p.name,
			Label:   p.label,
			Type:    TypeISO15,
			Bands:   p.gains,
			Builtin: true,
		}
	}
	return out
}

// Builtin finds a built-in preset by name.
func Builtin(name string) (Preset, bool) {
	for _, p := range Builtins() {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// PresetStore keeps the listener's own presets as JSON files in Dir.
type PresetStore struct {
	Dir string
}

func NewPresetStore(dir string) *PresetStore { return &PresetStore{Dir: dir} }

// List returns the built-in presets and then the listener's own.
func (s *PresetStore) List() []Preset {
	all := Builtins()
	if s == nil || s.Dir == "" {
		return all
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return all
	}
	var user []Preset
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			continue
		}
		var p Preset
		if err := json.Unmarshal(b, &p); err != nil {
			continue
		}
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" || (p.Type != TypeISO10 && p.Type != TypeISO15) || len(p.Bands) != BandCount(p.Type) {
			continue
		}
		_, builtin := Builtin(p.Name)
		if builtin {
			continue
		}
		p.Builtin, p.Label = false, p.Name
		user = append(user, p)
	}
	sort.Slice(user, func(i, j int) bool { return user[i].Name < user[j].Name })
	return append(all, user...)
}

// Save writes a preset of the listener's own. It may not replace a
// built-in.
func (s *PresetStore) Save(p Preset) error {
	if s == nil || s.Dir == "" {
		return errors.New("eq: no preset folder")
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return errors.New("eq: empty preset name")
	}
	if _, ok := Builtin(p.Name); ok {
		return fmt.Errorf("eq: %q is a built-in preset", p.Name)
	}
	if p.Type != TypeISO10 && p.Type != TypeISO15 {
		return errors.New("eq: unknown equalizer type")
	}
	if len(p.Bands) != BandCount(p.Type) {
		return errors.New("eq: wrong number of bands")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(Preset{Name: p.Name, Type: p.Type, Preamp: p.Preamp, Bands: p.Bands}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, presetFile(p.Name)), b, 0o644)
}

// Delete removes a preset of the listener's own.
func (s *PresetStore) Delete(name string) error {
	if _, ok := Builtin(name); ok {
		return fmt.Errorf("eq: %q is a built-in preset", name)
	}
	if s == nil || s.Dir == "" {
		return errors.New("eq: no preset folder")
	}
	return os.Remove(filepath.Join(s.Dir, presetFile(name)))
}

// presetFile names a preset's file, keeping letters, digits, spaces and
// dashes, and replacing what a file name cannot hold.
func presetFile(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == ' ', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), " .-")
	if out == "" {
		out = "preset"
	}
	return out + ".json"
}
