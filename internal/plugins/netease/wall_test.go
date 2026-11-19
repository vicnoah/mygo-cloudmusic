package netease

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/plugins/netease/internal/client"
)

// wallApp is an app showing the wall of a queue of n songs, the at-th
// playing.
func wallApp(t *testing.T, n, at int) (*testApp, *ui.Tester) {
	a := newTestApp()
	a.settings = defaultSettings()
	var tracks []netease.Track
	for i := range n {
		tracks = append(tracks, netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌%03d", i), Duration: 200000})
	}
	a.pl.Cue(tracks)
	if at > 0 {
		a.pl.Jump(at)
	}
	a.pl.SetLyrics(nil)
	a.nowPlaying, a.npPane = true, paneWall
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	return a, tt
}

// find returns where the cover of the song named s shows, failing the
// test if it is not built.
func find(t *testing.T, tt *ui.Tester, s string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("no cover of %s", s)
	}
	return r
}

// A click on a cover plays its song; a drag that starts on one moves the
// wall and plays nothing.
func TestWallPlaysTheCoverClicked(t *testing.T) {
	a, tt := wallApp(t, 300, 0)
	r := find(t, tt, "歌003")
	drag(tt, r, -300, 200*time.Millisecond)
	if got := a.pl.Current().Name; got != "歌000" {
		t.Fatalf("a drag played %s", got)
	}
	if moved := find(t, tt, "歌003"); moved.X > r.X-280 || moved.X < r.X-320 {
		t.Fatalf("dragged by 300, the cover went from %v to %v", r.X, moved.X)
	}
	if err := tt.Click("歌005"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.pl.Current().Name; got != "歌005" {
		t.Errorf("playing %s, want 歌005", got)
	}
}

// While the song plays, the rows sway, and a click plays the cover where
// it shows, not where it would rest: near its edge, which is far from
// where its row rests.
func TestWallPlaysTheCoverSwaying(t *testing.T) {
	a, tt := wallApp(t, 300, 0)
	a.pl.Hold(0) // playing
	for range 8 {
		tt.Advance(time.Second)
		tt.Frame()
		r := find(t, tt, "歌005")
		sway := r.X - (a.wallState.shown.X + a.wallState.pan.X + 5*wallPitch)
		if sway > -12 && sway < 12 {
			continue
		}
		x := r.X + 4 // the edge its row leads with
		if sway > 0 {
			x = r.X + r.W - 4
		}
		tt.ClickAt(x, r.Y+r.H/2)
		tt.Frame()
		if got := a.pl.Current().Name; got != "歌005" {
			t.Fatalf("clicked at its edge, swayed %v, played %s", sway, got)
		}
		return
	}
	t.Fatal("the rows never swayed")
}

// Thrown, the wall glides on past where the pointer let it go, and stops
// at its edge rather than leaving it.
func TestWallGlidesAndStops(t *testing.T) {
	a, tt := wallApp(t, 300, 0)
	ws := &a.wallState
	from := ws.shown.X
	r := find(t, tt, "歌003")
	drag(tt, r, -240, 0)
	tt.Advance(2 * time.Second)
	tt.Frame()
	if ws.shown.X > from-480 {
		t.Errorf("thrown 240 DIPs, the wall went from %v to %v", from, ws.shown.X)
	}
	// Thrown hard the other way, it stops with its first column in view.
	drag(tt, r, 3000, 0)
	tt.Advance(2 * time.Second)
	tt.Frame()
	if first := find(t, tt, "歌000"); first.X < 0 || first.X > 100 {
		t.Errorf("the first column stopped at %v", first.X)
	}
}

// Two fingers on the trackpad move the wall with them, at once.
func TestWallFollowsTheTrackpad(t *testing.T) {
	_, tt := wallApp(t, 300, 0)
	r := find(t, tt, "歌003")
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 180, 60)
	tt.Frame()
	moved := find(t, tt, "歌003")
	if dx, dy := moved.X-r.X, moved.Y-r.Y; dx > -170 || dx < -190 || dy > -50 || dy < -70 {
		t.Errorf("scrolled by (180, 60), the cover moved by (%v, %v)", dx, dy)
	}
}

// The wall opens on the song playing, however far into the queue, and
// goes to the next one played when it is out of view.
func TestWallShowsTheSongPlaying(t *testing.T) {
	a, tt := wallApp(t, 300, 250)
	in := func(s string) bool {
		r, ok := tt.Find(s)
		return ok && r.X >= 0 && r.X+r.W <= 1200 && r.Y >= 0 && r.Y+r.H <= 800
	}
	if !in("歌250") {
		t.Fatal("the wall opened away from the song playing")
	}
	a.pl.Jump(20)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if !in("歌020") {
		t.Error("the wall stayed away from the song played next")
	}
}

// With NM_FRAMES set, saves the wall of a long queue and a short one.
func TestWallFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, n := range []int{300, 7} {
		a, tt := wallApp(t, n, min(n-1, 40))
		colors := []ui.Color{ui.RGB(220, 120, 160), ui.RGB(90, 140, 220), ui.RGB(240, 200, 110), ui.RGB(120, 200, 170), ui.RGB(170, 120, 230)}
		for i := range a.pl.Queue() {
			url := fmt.Sprintf("art%d", i)
			a.pl.Queue()[i].Album.PicURL = url
			a.hostTest.artwork[netease.Sized(url, 240)] = &fixtureArtwork{bmp: solidBitmap(colors[i%len(colors)]), tint: colors[i%len(colors)]}
		}
		tt.Advance(time.Second)
		tt.Frame()
		save := func(name string) {
			f, _ := os.Create(filepath.Join(dir, name))
			png.Encode(f, tt.Image())
			f.Close()
		}
		save(fmt.Sprintf("wall-%d.png", n))
		// Scrolled up halfway into the frost.
		tt.Scroll(600, 600, 0, 70)
		tt.Frame()
		save(fmt.Sprintf("wall-%d-up.png", n))
	}
}

// solidBitmap is a cover of col, lighter toward its foot.
func solidBitmap(col ui.Color) *ui.Bitmap {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		c := col.Mix(ui.RGB(255, 255, 255), float32(y)/64)
		for x := range 32 {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return ui.NewBitmap(img)
}
