package app

import (
	"testing"

	"github.com/waygu-music-plugin/sdk/pluginapi/eq"
)

// Tests that enabling the equalizer updates the player configuration and applies presets.
func TestEqualizerSettingsAndPresets(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = true
	preset, _ := eq.Builtin("Vocal")
	a.settings.EQ = eq.Settings{
		Enabled: true,
		Type:    preset.Type,
		Preamp:  preset.Preamp,
		Bands:   append([]float64(nil), preset.Bands...),
		Preset:  preset.Name,
	}
	a.syncMPVSettings()

	cfg := a.pl.MPVConfig()
	if !cfg.EqualizerEnabled {
		t.Error("expected EqualizerEnabled to be true in player config")
	}
	if cfg.EqualizerType != preset.Type {
		t.Errorf("got type %q, want %q", cfg.EqualizerType, preset.Type)
	}
	if len(cfg.EqualizerBands) != len(preset.Bands) {
		t.Fatalf("got %d bands, want %d", len(cfg.EqualizerBands), len(preset.Bands))
	}
	for i := range preset.Bands {
		if cfg.EqualizerBands[i] != preset.Bands[i] {
			t.Errorf("band %d: got %v, want %v", i, cfg.EqualizerBands[i], preset.Bands[i])
		}
	}
}

// Tests that audio output devices are enumerated and configurable through MPV settings.
func TestAudioDeviceEnumerationAndSync(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = true
	a.pl.SetUseMPV(true)
	if !a.mpvReady() {
		t.Skip("mpv not ready on this host")
	}

	devs := a.pl.AudioDevices()
	if len(devs) == 0 {
		t.Fatal("expected at least one audio device from player")
	}

	a.settings.MPVAudioDevice = "auto"
	a.syncMPVSettings()

	cfg := a.pl.MPVConfig()
	if cfg.AudioDevice != "auto" {
		t.Errorf("got AudioDevice %q, want 'auto'", cfg.AudioDevice)
	}
}
