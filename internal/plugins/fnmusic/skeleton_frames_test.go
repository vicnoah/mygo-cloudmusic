package fnmusic

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"waygu-music/internal/plugins/internal/plugintest"

	"github.com/egoist/mygo/ui"
)

var fnmusicSkeletonScenes = []struct {
	name  string
	route Route
}{
	{"library", Route{Page: PageLibrary}},
	{"favorites", FavoritesRoute()},
	{"albums", Route{Page: PageAlbums}},
	{"artists", Route{Page: PageArtists}},
	{"artist-songs", ArtistSongsRoute(42, "artist")},
	{"artist-albums", Route{Page: PageArtistAlbums, ID: 42, Query: "artist"}},
	{"genres", Route{Page: PageGenres}},
	{"search", Route{Page: PageSearch, Query: "春日影"}},
	{"folders", Route{Page: PageFolders}},
	{"folder", Route{Page: PageFolders, Query: "/music"}},
	{"roam", Route{Page: PageRoam}},
}

// With NM_FRAMES set, saves every Feiniu loading layout without making a
// network request, in the light and the dark.
func TestFnmusicSkeletonFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range fnmusicSkeletonScenes {
		for _, dark := range []bool{false, true} {
			h := plugintest.NewHost(ID)
			settings := &platformPreferences{Config: connectionConfig{
				Address: "127.0.0.1:1", Username: "test", PassHash: "test",
			}}
			adapter := NewAdapter(h, settings)
			home := adapter.Home()
			home.ResetNavigation(s.route)
			view := func(c *ui.Context) {
				home.Content(c)
			}
			tt := ui.NewTester(view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(450 * time.Millisecond)
			tt.Frame()
			name := "fnmusic-" + s.name
			if dark {
				name += "-dark"
			}
			f, _ := os.Create(filepath.Join(dir, "skeleton-"+name+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
