package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// New plugins run through the normal startup and switch flow without sharing a press or saved settings.
func TestPluginMainWindowRoot(t *testing.T) {
	a := newTestApp()
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(dir, "settings.json")
	a.settings.Platform = "uninstalled"
	clicks := map[pluginapi.ID]int{}
	registrations := []pluginapi.ID{"first", "second"}
	a.platformRegistrations = []pluginapi.Registration{{
		Info: pluginapi.Info{ID: "incomplete"}, Start: func(pluginapi.Startup) pluginapi.Adapter { return testPluginAdapter{} },
	}}
	var firstScope context.Context
	for _, id := range registrations {
		a.platformRegistrations = append(a.platformRegistrations, pluginapi.Registration{
			Info: pluginapi.Info{ID: id, Name: string(id)},
			Start: func(start pluginapi.Startup) pluginapi.Adapter {
				start.RegisterLifecycle(pluginapi.Lifecycle{Activate: func(ctx context.Context, _ pluginapi.PlaybackContinuity) {
					if id == "first" {
						firstScope = ctx
					}
				}})
				start.RegisterWindow(pluginapi.MainWindow, pluginapi.Window{Root: func(c *ui.Context) {
					if toast := start.Host.TakeToast(); toast != "" && id == "second" {
						t.Errorf("the new root received the previous plugin's toast: %q", toast)
					}
					if ui.Button(c.Key("play"), "Play").Clicked() {
						clicks[id]++
						if err := start.Settings.Save(map[string]string{"lastClicked": string(id)}); err != nil {
							t.Fatal(err)
						}
					}
				}})
				return testPluginAdapter{}
			},
		})
	}
	if !a.initPlatformState() || a.platformID != "first" {
		t.Fatalf("startup platform = %q, want first launchable plugin", a.platformID)
	}
	a.activatePlatformView()
	t.Cleanup(func() { a.deactivatePlatformView(false) })
	tt := ui.NewTester(a.view, 400, 240)
	tt.Frame()
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	button, ok := tt.Find("Play")
	if !ok {
		t.Fatal("play button disappeared")
	}
	x, y := button.X+button.W/2, button.Y+button.H/2
	tt.Press(x, y)
	tt.Frame()
	(platformHost{a: a}).Toast("song added to the previous platform")
	a.setPlatform("second")
	tt.Frame()
	tt.Release(x, y)
	tt.Frame()
	if clicks["second"] != 0 {
		t.Fatal("releasing a press on the first plugin activated the second plugin's button")
	}
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if firstScope == nil || firstScope.Err() != context.Canceled {
		t.Fatal("switching did not cancel the first plugin's activation")
	}
	a.setPlatform("incomplete")
	if a.platformID != "second" {
		t.Fatalf("switching to a plugin without a root displaced the active plugin: %q", a.platformID)
	}
	saved := loadSettings()
	if saved.Platform != "second" {
		t.Fatalf("saved platform = %q, want second", saved.Platform)
	}
	for _, id := range []pluginapi.ID{"first", "second"} {
		if got := clicks[id]; got != 1 {
			t.Fatalf("root %q handled %d clicks", id, got)
		}
		var namespace map[string]string
		if err := json.Unmarshal(saved.Plugins[id], &namespace); err != nil || namespace["lastClicked"] != string(id) {
			t.Fatalf("plugin %q settings did not round trip: %v, %v", id, namespace, err)
		}
	}
}

type testPluginAdapter struct{}

func (testPluginAdapter) Configured() bool                       { return true }
func (testPluginAdapter) OwnsTrack(*pluginapi.Track) bool        { return false }
func (testPluginAdapter) PlaybackMedia() pluginapi.PlaybackMedia { return pluginapi.PlaybackMedia{} }
func (testPluginAdapter) FetchArtwork(context.Context, string, int) ([]byte, bool, error) {
	return nil, false, nil
}
