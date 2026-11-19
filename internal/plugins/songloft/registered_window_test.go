package songloft

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/internal/plugintest"
)

// Clicking a Songloft library row queues the track at its original position.
func TestSongloftLibraryRowPlays(t *testing.T) {
	h, home := songloftTestHome(t)
	tracks := []pluginapi.Track{
		{ID: 17, Name: "Songloft first track", Source: "songloft", Duration: 80000},
		{ID: 23, Name: "Songloft second track", Source: "songloft", Duration: 95000},
	}
	list := home.State().Catalog().ListForRoute(0, "library", "")
	list.Pages.Land(0, tracks, len(tracks))

	tt := ui.NewTester(h.Windows[pluginapi.MainWindow].Root, 1200, 800)
	tt.Frame()
	if err := tt.Click(tracks[1].Name); err != nil {
		t.Fatal(err)
	}
	if tr := h.Engine.Current(); tr == nil || tr.ID != tracks[1].ID {
		t.Fatalf("playing %+v, want track %d", tr, tracks[1].ID)
	}
	queue := h.Engine.Queue()
	if len(queue) != 2 || queue[0].ID != tracks[0].ID || queue[1].ID != tracks[1].ID {
		t.Fatalf("queue = %+v", queue)
	}
}

// The window shortcut focuses Songloft's own field so typing and Enter open the query.
func TestSongloftSearchShortcut(t *testing.T) {
	h, home := songloftTestHome(t)
	list := home.State().Catalog().ListForRoute(0, "library", "")
	list.Pages.Land(0, nil, 0)

	tt := ui.NewTester(h.Windows[pluginapi.MainWindow].Root, 1200, 800)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("Blue Moon")
	if got := strings.TrimSpace(*home.SearchText()); got != "Blue Moon" {
		t.Fatalf("search field contains %q", got)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if route := home.Route(); route.Page != PageSearch || route.Query != "Blue Moon" {
		t.Fatalf("search opened %+v", route)
	}
}

// The native New Playlist action opens Songloft's playlist form.
func TestSongloftNativeNewPlaylist(t *testing.T) {
	h, home := songloftTestHome(t)
	list := home.State().Catalog().ListForRoute(0, "library", "")
	list.Pages.Land(0, nil, 0)
	tt := ui.NewTester(h.Windows[pluginapi.MainWindow].Root, 1200, 800)
	tt.Frame()

	menu := h.Menus[pluginapi.ApplicationMenu]()
	for _, group := range menu.Items() {
		for _, item := range group.Submenu {
			if item.Label != "新建歌单…" {
				continue
			}
			item.Click(item, nil)
			tt.Frame()
			if _, ok := tt.Find("歌单名称"); !ok {
				t.Fatal("New Playlist did not open Songloft's playlist form")
			}
			return
		}
	}
	t.Fatal("native New Playlist action is missing")
}

// Turning on Songloft's quick player from its first-play greeting keeps it on.
func TestSongloftQuickPlayerGreeting(t *testing.T) {
	h := plugintest.NewHost(ID)
	var saved pluginapi.QuickPlayerPreferences
	h.QuickPanel = &pluginapi.MenuBarPanel{
		Preferences:     func() pluginapi.QuickPlayerPreferences { return saved },
		SavePreferences: func(p pluginapi.QuickPlayerPreferences) { saved = p },
	}
	h.Start(Registration(), nil)
	panel := h.Windows[pluginapi.QuickPlayerWindow]
	h.Engine.Cue([]pluginapi.Track{{ID: 17, Name: "Blue Moon", Source: "songloft", Duration: 80000}})
	panel.Updated()
	if saved.Greeted {
		t.Fatal("quick player greeted before playback")
	}

	h.AttachSilence(t, false)
	panel.Updated()
	tt := ui.NewTester(panel.Root, panel.Options.Width, panel.Options.Height)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("开启刘海播放器"); err != nil {
		t.Fatal(err)
	}
	if !saved.Enabled || !saved.Greeted {
		t.Fatalf("quick player preferences after enabling from its greeting: %+v", saved)
	}
}

// Toggling smoke in Songloft settings applies the shared presentation preference.
func TestSongloftAppliesSmokePreference(t *testing.T) {
	h := plugintest.Start(t, Registration())
	saves := 0
	h.Save = func() { saves++ }
	tt := ui.NewTester(h.Windows[pluginapi.SettingsWindow].Root, 560, 940)
	tt.Frame()
	label, ok := tt.Find("进度条烟雾特效")
	if !ok {
		t.Fatal("missing progress effect setting")
	}
	tt.ClickAt(498, label.Y+label.H)
	tt.Frame()
	if !h.Desktop.Preferences.Smoke || saves != 1 {
		t.Fatal("the settings switch did not apply and save the smoke preference")
	}
}

func songloftTestHome(t *testing.T) (*plugintest.Host, *Home) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"test","expires_in":3600}`)
	})
	mux.HandleFunc("GET /api/v1/playlists", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"playlists":[],"total":0}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	h := plugintest.NewHost(ID)
	home := h.Start(Registration(), nil).(*Adapter).Home()
	h.Activate(t)
	state := home.State()
	cfg := state.Config()
	cfg.Address = server.URL
	cfg.Username = "test"
	cfg.Password = "test"
	if err := state.Connect(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Disconnect() })
	return h, home
}
