package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/eq"
	"netease-music-mygo/internal/netease"
)

// The equalizer has no button while MPV does not play.
func TestEqualizerHiddenWithoutMPV(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = false
	a.pl.SetUseMPV(false)
	a.pl.Cue([]netease.Track{{ID: 1, Name: "Test Track", Duration: 240000}})
	tt := ui.NewTester(func(c *ui.Context) {
		a.playerBar(c)
	}, 1200, 200)
	tt.Frame()
	if _, ok := tt.Find("图形均衡器 (EQ)"); ok {
		t.Error("the equalizer button shows without MPV")
	}
}

// Turning MPV off closes the equalizer and its dialogs.
func TestEqualizerClosesWhenMPVOff(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = true
	a.pl.SetUseMPV(true)
	if !a.mpvReady() {
		t.Skip("mpv not ready on this host")
	}
	a.eqOpen, a.eqUI.autoeqOpen = true, true
	a.settings.UseMPV = false
	a.syncMPVSettings()
	if a.eqOpen || a.eqUI.autoeqOpen {
		t.Error("the equalizer stayed open after MPV was turned off")
	}
}

// The equalizer's save button opens the preset name dialog.
func TestEqualizerSaveDialogOpens(t *testing.T) {
	a := newTestApp()
	a.settings.UseMPV = true
	a.pl.SetUseMPV(true)
	if !a.mpvReady() {
		t.Skip("mpv not ready on this host")
	}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "Test Track", Duration: 240000}})
	tt := ui.NewTester(func(c *ui.Context) {
		a.playerBar(c)
	}, 1200, 600)
	tt.Frame()
	if err := tt.Click("图形均衡器 (EQ)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.eqUI.saveOpen {
		t.Error("the preset name dialog did not open")
	}
}

// Saving a preset from the dialog writes it to the preset folder.
func TestEqualizerSavesPreset(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	a := newTestApp()
	a.settings.UseMPV = true
	a.pl.SetUseMPV(true)
	if !a.mpvReady() {
		t.Skip("mpv not ready on this host")
	}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "Test Track", Duration: 240000}})
	tt := ui.NewTester(func(c *ui.Context) {
		a.playerBar(c)
	}, 1200, 600)
	tt.Frame()
	if err := tt.Click("图形均衡器 (EQ)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("我的预设")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.settings.EQ.Preset != "我的预设" {
		t.Errorf("preset %q, want 我的预设", a.settings.EQ.Preset)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "eq_presets"))
	if err != nil || len(entries) != 1 {
		t.Errorf("preset files %v, %v", entries, err)
	}
}

// The AutoEQ dialog lists the profiles it has fetched.
func TestEqualizerAutoEQDialogListsProfiles(t *testing.T) {
	a := newTestApp()
	a.pl.SetUseMPV(true)
	if !a.mpvReady() {
		t.Skip("mpv not ready on this host")
	}
	a.eqUI.autoeqMetas = []eq.Meta{{
		Name: "Test Headphone", Path: "src/over-ear/Test Headphone", Source: "src", Type: "over-ear",
	}}
	a.eqUI.autoeqOpen = true
	a.pl.Cue([]netease.Track{{ID: 1, Name: "Test Track", Duration: 240000}})
	tt := ui.NewTester(func(c *ui.Context) {
		a.playerBar(c)
	}, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("Test Headphone"); !ok {
		t.Error("the AutoEQ dialog did not list the profile")
	}
}
