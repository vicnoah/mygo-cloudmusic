package fnmusic

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
)

// With NM_FRAMES set, saves album cards whose covers are missing.
func TestFnmusicAlbumPlaceholderFrame(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	h := NewHome(NewState(), nopHost{})
	p := paging.New[pluginapi.Playlist](60, 0, nil, nil)
	p.Land(0, []pluginapi.Playlist{
		{ID: 1, Name: "无封面专辑"},
		{ID: 2, Name: "另一张无封面专辑"},
	}, 2)
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		h.pal = paletteFor(&t)
		h.renderCards(c, p, CardView{
			Title: "专辑",
			Header: func(c *ui.Context) {
				ui.Text(c, "专辑").FontSize(30).FontWeight(700).Margin(0, 0, 18, 0)
			},
			Placeholder: iconNote,
		})
	}, 1200, 800)
	tt.Frame()
	f, _ := os.Create(filepath.Join(dir, "loaded-fnmusic-album-placeholder.png"))
	png.Encode(f, tt.Image())
	f.Close()
}
