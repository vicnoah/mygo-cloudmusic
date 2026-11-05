package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player/dlna"
	"netease-music-mygo/internal/player/mpv"
)

// Tests that clicking the cast button toggles the DLNA device selection popover.
func TestCastButtonTogglesPopover(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "Test Track", Duration: 240000}})

	tt := ui.NewTester(func(c *ui.Context) {
		a.playerBar(c)
	}, 1200, 200)

	if a.castOpen {
		t.Error("expected castOpen to be false initially")
	}

	// Click cast button
	if err := tt.Click("无线投送 (DLNA)"); err != nil {
		t.Fatalf("expected to click cast button: %v", err)
	}

	if !a.castOpen {
		t.Error("expected castOpen to be true after clicking cast button")
	}

	// Popover should now display local device option
	if err := tt.Click("本机播放"); err != nil {
		t.Fatalf("expected to click '本机播放' option in cast popover: %v", err)
	}

	if a.castOpen {
		t.Error("expected castOpen to close after selecting an option")
	}
	if a.pl.IsCasting() {
		t.Error("expected player not to be casting when local device is chosen")
	}
}

// Tests that an active DLNA device is displayed in the popover even while scanning.
func TestActiveCastDeviceVisibleWhileScanning(t *testing.T) {
	a := newTestApp()
	ctrl := a.pl.DLNA()
	if ctrl == nil {
		t.Fatal("expected dlna controller to be initialized")
	}

	mockDev := &dlna.Device{
		ID:           "http://192.168.1.88:1400/xml/desc.xml",
		FriendlyName: "Bedroom Speaker",
		ModelName:    "Smart-Speaker",
		URL:          "http://192.168.1.88:1400/xml/desc.xml",
	}

	// Connect active device
	ctrl.AddDevice(mockDev)
	a.pl.SwitchOutput(mockDev.ID)

	tt := ui.NewTester(func(c *ui.Context) {
		a.playerBar(c)
	}, 1200, 200)

	// Set scanning flag to true to simulate background scan
	a.dlnaScanning = true

	// Open popover
	if err := tt.Click("无线投送 (DLNA)"); err != nil {
		t.Fatalf("expected to click cast button: %v", err)
	}

	if !a.castOpen {
		t.Error("expected castOpen to be true")
	}

	// Device must still be present and clickable despite ongoing scan
	if err := tt.Click("Bedroom Speaker"); err != nil {
		t.Fatalf("expected Bedroom Speaker to be visible and clickable in popover: %v", err)
	}
}

// Tests that enabling the equalizer updates the player configuration and applies presets.
func TestEqualizerSettingsAndPresets(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = true
	a.settings.EqualizerEnabled = true
	a.settings.EqualizerPreset = "vocal"
	preset, _ := mpv.FindPreset("vocal")
	a.settings.EqualizerGains = preset.Gains
	a.syncMPVSettings()

	cfg := a.pl.MPVConfig()
	if !cfg.EqualizerEnabled {
		t.Error("expected EqualizerEnabled to be true in player config")
	}
	if cfg.EqualizerGains != preset.Gains {
		t.Errorf("got gains %v, want %v", cfg.EqualizerGains, preset.Gains)
	}
}

// Tests that clicking the equalizer button on the player bar toggles the EQ popover when MPV is ready.
func TestEqualizerButtonTogglesPopover(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = true
	a.pl.SetUseMPV(true)
	if !a.mpvReady() {
		t.Skip("mpv not ready on this host")
	}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "Test Track", Duration: 240000}})

	tt := ui.NewTester(func(c *ui.Context) {
		a.playerBar(c)
	}, 1200, 200)

	if a.eqOpen {
		t.Error("expected eqOpen to be false initially")
	}

	if err := tt.Click("图形均衡器 (EQ)"); err != nil {
		t.Fatalf("expected to click equalizer button: %v", err)
	}

	if !a.eqOpen {
		t.Error("expected eqOpen to be true after clicking equalizer button")
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

// Tests that E-AC-3 passthrough is not forced on the auto/default output.
func TestAudioSpdifDisabledForAutoDevice(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = true
	a.settings.MPVAudioSpdif = true
	a.settings.MPVAudioDevice = "auto"
	a.syncMPVSettings()

	if cfg := a.pl.MPVConfig(); cfg.AudioSpdif != "" {
		t.Fatalf("got AudioSpdif %q for auto device, want empty PCM-decoding path", cfg.AudioSpdif)
	}
}
