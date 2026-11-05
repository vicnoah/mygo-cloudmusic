package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player/dlna"
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
