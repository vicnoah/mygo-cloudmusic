package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
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
