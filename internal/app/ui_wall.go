package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The wall lays the queue out as a wall of its covers, under the song
// playing: dragged, it moves every way and, thrown, glides on; swiped
// across the trackpad, it follows the fingers; a click on a cover plays
// it. Where the wall meets the song above, its covers fade into a frost
// as they go under it.

// The wall's covers, the gaps between them, and how far one is from the
// next.
const (
	wallTile  = 112
	wallGap   = 14
	wallPitch = wallTile + wallGap
)

// wallGlide is how the wall glides on once thrown, and to the song
// playing.
var wallGlide = ui.Animation{Spring: ui.SpringOptions{Duration: 800 * time.Millisecond}}

// wallState is the wall as the pointer and the trackpad move it.
type wallState struct {
	off   ui.Point // where Draggable rests it
	pan   ui.Point // where the trackpad moved it, besides
	shown ui.Point // where Draggable showed it in the last frame
	// The song the wall last brought into view, 0 until it shows.
	track int64
}

// wallLayout is how the wall lays out its covers in an area: as many
// columns as rows, and the offsets it rests at, within bounds.
type wallLayout struct {
	cols, rows int
	bounds     ui.Rect
}

// layWall lays n covers out in an area w×h, keeping the first row top
// down from the area's top, out of the frost, and the last foot up from
// its foot, above the controls. A long queue makes a wall wider than tall,
// and wider than the area, which the pointer then moves along both axes;
// a short one is centered, as wide as it needs.
func layWall(n int, w, h, top, foot float32) wallLayout {
	const pad = 48
	fit := max(1, int((w-2*pad+wallGap)/wallPitch))
	cols := max(min(n, fit), int(math.Ceil(math.Sqrt(float64(n)*2.5))))
	cols = max(1, min(cols, n))
	rows := (n + cols - 1) / cols
	cw, ch := float32(cols)*wallPitch-wallGap, float32(rows)*wallPitch-wallGap
	b := ui.Rect{X: w - pad - cw, W: cw + 2*pad - w, Y: h - foot - ch, H: ch + top + foot - h}
	if b.W < 0 {
		b.X, b.W = (w-cw)/2, 0
	}
	if b.H < 0 {
		b.Y, b.H = top, 0
	}
	return wallLayout{cols, rows, b}
}

// clamp returns p held within the offsets the wall rests at.
func (l wallLayout) clamp(p ui.Point) ui.Point {
	b := l.bounds
	return ui.Point{X: max(b.X, min(p.X, b.X+b.W)), Y: max(b.Y, min(p.Y, b.Y+b.H))}
}

// cell returns where the cover of the i-th song is on the wall.
func (l wallLayout) cell(i int) ui.Point {
	return ui.Point{X: float32(i%l.cols) * wallPitch, Y: float32(i/l.cols) * wallPitch}
}

// centre returns the offset that brings the i-th song's cover to the
// middle of the area w×h between top and foot, as near as the wall goes.
func (l wallLayout) centre(i int, w, h, top, foot float32) ui.Point {
	at := l.cell(i)
	return l.clamp(ui.Point{X: w/2 - at.X - wallTile/2, Y: (top+h-foot)/2 - at.Y - wallTile/2})
}

// shows reports whether the i-th song's cover is wholly in the area w×h
// between top and foot, with the wall at off.
func (l wallLayout) shows(i int, off ui.Point, w, h, top, foot float32) bool {
	at := l.cell(i)
	x, y := off.X+at.X, off.Y+at.Y
	return x >= 0 && x+wallTile <= w && y >= top && y+wallTile <= h-foot
}

// wallPane fills Now Playing with the wall, the song playing over it and
// the controls along its foot.
func (a *app) wallPane(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32, pointerIn bool) {
	// The song starts under the tabs, which are over its left.
	top := topBar(c) + npTabsHeight
	songH := min(max(h*0.36, 200), 320)
	// The wall goes on under the foot of the song, into the frost; at
	// rest, its first row is clear of the frost and its last of the
	// controls' blur, which only covers going by pass under.
	const frost, foot = 72, 112
	wallY := top + songH - frost/2
	a.wall(c, wallY, w, h-wallY, frost, foot, frost)
	// Up into the frost, covers blur more and more and fade into the
	// backdrop, painted again over them, opaque by the frost's top: there
	// it shows what shows above it, so that no edge does.
	ui.Box(c.Key("wall-frost")).Absolute().Top(wallY).Left(0).Right(0).Height(frost).PassThrough().
		Material(glass.Blur{Radius: 16, Mask: &ui.LinearGradient{From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 180}}).
		Draw(func(p *ui.Painter, r ui.Rect) {
			a.backdropOver(p, ui.Rect{W: w, H: h}, r, func(t float32) float32 {
				return 1 - smoothstep(0.1, 0.85, t)
			})
		})
	// Down under the controls, they blur and fade most of the way, for
	// the controls to read over them.
	ui.Box(c.Key("wall-foot")).Absolute().Left(0).Right(0).Bottom(0).Height(foot).PassThrough().
		Material(glass.Blur{Radius: 14, Mask: &ui.LinearGradient{From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 0}}).
		Draw(func(p *ui.Painter, r ui.Rect) {
			a.backdropOver(p, ui.Rect{W: w, H: h}, r, func(t float32) float32 { return 0.7 * smoothstep(0, 0.9, t) })
		})
	a.wallSong(c, tr, art, top, songH)
	a.wallBar(c, accent)
	reveal(npTabsRow(c, w), pointerIn).Children(func() { a.paneTabs(c) })
}

// wallSong is the song playing, over the wall: its cover, which breathes
// as Now Playing's does, its name, and the line of its lyrics sung, with
// the next.
func (a *app) wallSong(c *ui.Context, tr *netease.Track, art *artwork, top, h float32) {
	pl := a.pl
	ui.Row(c.Key("wall-song")).Absolute().Top(top).Left(0).Right(0).Height(h).Padding(0, 72).Gap(36).
		Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		// Not Now Playing's cover, which the pointer tilts and flings: here
		// the wall is what the pointer moves.
		size := h - 48
		cov := ui.Box(c.Key(coverKey{tr.ID, 0})).Size(size, size).Radius(16).Shadow(0, 24, 50, -12, ui.RGBA(0, 0, 0, 0.55))
		scale := float32(1)
		if !pl.Playing() {
			scale = 0.92
		}
		cov.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}})
		in := artIn(cov, bitmap(art))
		if in < 1 {
			cov.Background(white.Alpha(0.08))
		}
		cov.Children(func() { artImage(c, bitmap(art), 16, in) })
		ui.Column(c).Width(440).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			if r := pl.Radio(); r != "" {
				a.radioChip(c, r)
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tr.Name).FontSize(26).FontWeight(800).LetterSpacing(-0.4).SingleLine().Shrink(1).MinWidth(0)
				a.likeButton(c.Key("like"), tr, 20, white.Alpha(0.7))
				a.songMenu(c.Key("more"), tr, 20, white.Alpha(0.7)).Margin(0, -8, 0, 0)
			})
			a.artistLinks(c, tr, white.Alpha(0.6), white, true).FontSize(14).SingleLine()
			lines, _ := pl.Lyrics()
			if len(lines) == 0 {
				return
			}
			pos := pl.Position()
			cur := lyrics.Index(lines, pos)
			// Redraw as the next line begins.
			if pl.Playing() && cur+1 < len(lines) {
				c.After(lines[cur+1].At - pos + 20*time.Millisecond)
			}
			sung := ""
			if cur >= 0 {
				sung = lines[cur].Text
			}
			ui.Text(c, sung).FontSize(17).FontWeight(700).TextColor(white.Alpha(0.85)).MaxLines(2).Margin(14, 0, 0, 0)
			if cur+1 < len(lines) {
				ui.Text(c, lines[cur+1].Text).FontSize(14).FontWeight(600).TextColor(white.Alpha(0.38)).SingleLine()
			}
		})
	})
}

// wall lays the queue's covers out in an area from y down to the
// window's foot, w×h, its first row top down from the area's top and its
// last foot above its foot, frost the height of the frost over its top.
func (a *app) wall(c *ui.Context, y, w, h, top, foot, frost float32) {
	pl, ws := a.pl, &a.wallState
	queue := pl.Queue()
	l := layWall(len(queue), w, h, top, foot)
	playing := -1
	if order, at := pl.PlayOrder(); at >= 0 && at < len(order) {
		playing = order[at]
	}
	// Shown, the wall brings the song playing to its middle; after, it
	// glides to the next as little as shows it, unless held.
	if tr := pl.Current(); tr != nil && tr.ID != ws.track && playing >= 0 {
		if ws.track == 0 {
			ws.pan, ws.off = ui.Point{}, l.centre(playing, w, h, top, foot)
		} else if !l.shows(playing, addPoint(ws.shown, ws.pan), w, h, top, foot) {
			ws.off = subPoint(l.centre(playing, w, h, top, foot), ws.pan)
		}
		ws.track = tr.ID
	}

	// Covers go only into the frost, never past it over the song: the area
	// clips them, standing still as they move in it.
	area := ui.Box(c.Key("wall")).Absolute().Top(y).Left(0).Right(0).Bottom(0).Clip()
	// The trackpad and the wheel move it at once: the fingers carry it,
	// and their momentum, as far as it goes.
	area.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll {
			return false
		}
		ws.off = ws.shown // a glide stops where it shows
		at := addPoint(ws.shown, ws.pan)
		ws.pan = subPoint(l.clamp(ui.Point{X: at.X - ev.DX, Y: at.Y - ev.DY}), ws.shown)
		return true
	})
	area.Children(func() {
		bounds := l.bounds
		bounds.X -= ws.pan.X
		bounds.Y -= ws.pan.Y
		plane := ui.Box(c.Key("wall-plane")).Absolute().Top(0).Left(0).
			Size(float32(l.cols)*wallPitch, float32(l.rows)*wallPitch).
			Translate(ws.pan.X, ws.pan.Y).
			Draggable(&ws.off, ui.DraggableOptions{Bounds: &bounds, Snap: func(p ui.Point) ui.Point { return p }, Animation: wallGlide})
		ws.shown = plane.DragOffset()
		at := addPoint(ws.shown, ws.pan)
		// The wall is another view of the same lazy queue. When its last
		// rows approach the viewport, grow the queue before the listener
		// reaches the hard edge.
		lastRow := int(math.Floor(float64((h - at.Y) / wallPitch)))
		if lastRow >= l.rows-2 {
			pl.Prefetch()
		}
		// The rows sway while the song plays, and settle while it is paused.
		swaying := pl.Playing() && !c.Preferences().ReduceMotion
		now := c.Now()
		if swaying {
			_, turn := rowSwayTarget(0, now)
			c.After(turn)
		}
		// The covers take no press of their own, which would keep it from
		// dragging the wall: the wall finds the one under the pointer, in
		// the row under it, where that row showed in the last frame. Not one
		// under the controls, nor deep in the frost, nor one a control over
		// it takes the pointer from.
		pointRow, pointX := -1, float32(0)
		if px, py, over := plane.PointerPosition(); over && plane.Hovered() && !plane.Dragging() && px >= 0 && py >= 0 &&
			at.Y+py > frost/2 && at.Y+py < h-foot && py-float32(int(py/wallPitch))*wallPitch < wallTile {
			pointRow, pointX = int(py/wallPitch), px
		}
		planeX := plane.Bounds().X
		hover := -1
		pressed := plane.Pressed()
		// Only the covers in view, or nearly, as far as the rows sway.
		c0 := max(0, int(math.Floor(float64((-at.X-wallSway)/wallPitch))))
		c1 := min(l.cols-1, int(math.Floor(float64((w-at.X+wallSway)/wallPitch))))
		r0 := max(0, int(math.Floor(float64(-at.Y/wallPitch))))
		r1 := min(l.rows-1, int(math.Floor(float64((h-at.Y)/wallPitch))))
		plane.Children(func() {
			for r := r0; r <= r1; r++ {
				// Under the frost's opaque top, a row is not built.
				if at.Y+float32(r)*wallPitch+wallTile < frost*0.1 {
					continue
				}
				// A row sways as a pose, in frames that only paint.
				row := ui.Box(c.Key(wallRow(r))).Absolute().Top(float32(r)*wallPitch).Left(0).
					Size(float32(l.cols)*wallPitch, wallTile)
				if swaying {
					to, _ := rowSwayTarget(r, now)
					row.AnimatePose(ui.Pose{X: to}, ui.Animation{Duration: wallSwayHalf, Ease: ui.EaseInOut})
				} else {
					row.AnimatePose(ui.Pose{}, ui.Animation{Spring: ui.SpringOptions{Duration: 1200 * time.Millisecond}})
				}
				if r == pointRow {
					x := pointX
					if b := row.Bounds(); b.W > 0 {
						x -= b.X - planeX // how far it had swayed
					}
					col := int(math.Floor(float64(x / wallPitch)))
					if i := r*l.cols + col; x >= 0 && col < l.cols && i < len(queue) && x-float32(col)*wallPitch < wallTile {
						hover = i
						plane.Cursor(ui.CursorPointer)
					}
				}
				row.Children(func() {
					for col := c0; col <= c1; col++ {
						i := r*l.cols + col
						if i >= len(queue) {
							break
						}
						lift := float32(1)
						switch {
						case i == playing:
							lift = 1.08
						case i == hover && pressed:
							lift = 0.96
						case i == hover:
							lift = 1.04
						}
						a.wallTile(c, &queue[i], i, float32(col)*wallPitch, i == playing, i == hover, lift)
					}
				})
			}
		})
		if plane.Clicked() && hover >= 0 && hover != playing && queue[hover].Playable() {
			pl.Jump(hover)
		}
	})
}

// wallRow keys a row of the wall.
type wallRow int

// How far the rows sway across, either way, and how long one takes to
// sway from one side to the other.
const (
	wallSway     = 36
	wallSwayHalf = 7 * time.Second
)

// rowSwayTarget returns the side row r sways to at now, rows next to each
// other going opposite ways, and when it turns for the other.
func rowSwayTarget(r int, now time.Time) (float32, time.Duration) {
	half := int64(wallSwayHalf)
	k := now.UnixNano() / half
	to := float32(wallSway)
	if (k+int64(r))%2 != 0 {
		to = -to
	}
	return to, time.Duration(half - now.UnixNano()%half)
}

// wallTile is the cover of the i-th song of the queue, tr, x along its
// row, scaled to lift: the song playing stands out,
// ringed, and the one under the pointer says what it is.
func (a *app) wallTile(c *ui.Context, tr *netease.Track, i int, x float32, playing, hovered bool, lift float32) {
	tile := ui.Box(c.Key(i)).Absolute().Left(x).Top(0).Size(wallTile, wallTile).Radius(12).
		Label(tr.Name).ContextMenu(a.songItems(tr, 0))
	tile.AnimatePose(ui.Pose{Scale: lift}, ui.Animation{Spring: ui.SpringOptions{Duration: 400 * time.Millisecond, Bounce: 0.2}})
	if !tr.Playable() {
		tile.Opacity(0.4)
	}
	if playing {
		tile.Shadow(0, 14, 30, -8, ui.RGBA(0, 0, 0, 0.6))
	}
	tile.Children(func() {
		a.cover(c, tr.Album.PicURL, 240, wallTile, 12)
		if hovered {
			// Its name and artists over a shade along its foot, on the
			// cover, so that they move with it.
			ui.Column(c.Key("caption")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(12).Padding(8).Gap(1).
				Justify(ui.End).PassThrough().TextColor(white).
				Gradient(ui.RGBA(0, 0, 0, 0.78), ui.Transparent, 0).
				Children(func() {
					ui.Text(c, tr.Name).FontSize(12).FontWeight(700).SingleLine()
					if names := tr.ArtistNames(); names != "" {
						ui.Text(c, names).FontSize(10.5).TextColor(white.Alpha(0.7)).SingleLine()
					}
				})
		}
		if playing {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(12).Border(2, white.Alpha(0.9)).PassThrough()
		}
	})
}

// wallBar is the controls along the foot of the wall: where the song is,
// and the transport between how far it has gone and how far it has left.
func (a *app) wallBar(c *ui.Context, accent ui.Color) {
	pl := a.pl
	ui.Column(c.Key("wall-bar")).Absolute().Left(0).Right(0).Bottom(0).Padding(0, 48, 24, 48).Gap(10).Children(func() {
		a.scrubber(c.Key("wall-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 14, accent.Mix(white, 0.35))
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				ui.Text(c, clock(pl.Position())).FontSize(11.5).FontFeatures("tnum").TextColor(white.Alpha(0.5))
			})
			a.transport(c, white, white.Alpha(0.45), white, 22)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).Children(func() {
				ui.Text(c, "-"+clock(pl.Duration()-pl.Position())).FontSize(11.5).FontFeatures("tnum").TextColor(white.Alpha(0.5))
			})
		})
	})
}

func addPoint(p, q ui.Point) ui.Point { return ui.Point{X: p.X + q.X, Y: p.Y + q.Y} }

func subPoint(p, q ui.Point) ui.Point { return ui.Point{X: p.X - q.X, Y: p.Y - q.Y} }

// smoothstep goes from 0 at a to 1 at b, easing in and out.
func smoothstep(a, b, t float32) float32 {
	t = clamp01((t - a) / (b - a))
	return t * t * (3 - 2*t)
}
