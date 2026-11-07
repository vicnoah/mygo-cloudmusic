package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"netease-music-mygo/internal/eq"
)

// Settings from before the 15-band equalizer reset the equalizer, but
// keep the rest of the listener's choices.
func TestLoadSettingsResetsOldEqualizer(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	body := `{
		"volume": 0.5,
		"equalizerEnabled": true,
		"equalizerPreset": "bass",
		"equalizerPreamp": -3,
		"equalizerGains": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
	}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	if s.Volume != 0.5 {
		t.Errorf("volume %v, want 0.5", s.Volume)
	}
	if s.EQ.Enabled || s.EQ.Type != eq.TypeISO15 || s.EQ.Preset != "Flat" {
		t.Errorf("equalizer %+v, want defaults", s.EQ)
	}
	if len(s.EQ.Bands) != eq.BandCount(eq.TypeISO15) {
		t.Errorf("got %d bands, want %d", len(s.EQ.Bands), eq.BandCount(eq.TypeISO15))
	}
}

// The new equalizer settings are kept as they are.
func TestLoadSettingsKeepsNewEqualizer(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	body := `{
		"eq": {
			"enabled": true,
			"type": "ISO10Band",
			"preamp": -2,
			"bands": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
			"preset": "Vocal"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	if !s.EQ.Enabled || s.EQ.Type != eq.TypeISO10 || s.EQ.Preamp != -2 || s.EQ.Preset != "Vocal" {
		t.Errorf("equalizer %+v", s.EQ)
	}
	if len(s.EQ.Bands) != 10 || s.EQ.Bands[0] != 1 || s.EQ.Bands[9] != 10 {
		t.Errorf("bands %v", s.EQ.Bands)
	}
}
