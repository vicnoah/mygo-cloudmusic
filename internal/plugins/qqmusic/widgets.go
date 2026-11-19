package qqmusic

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

// actionPill uses the same palette as the Songloft window and settings.
func actionPill(c *ui.Context, key string, icon *ui.SVG, label string, primary bool) ui.Element {
	p := paletteFor(c.Theme())
	if primary {
		return pillButton(c.Key(key), icon, label, p.accent, p.onAccent)
	}
	return pillButton(c.Key(key), icon, label, p.hover.Over(p.bg), p.text)
}

func bone(c *ui.Context, radius float32) ui.Element {
	return pageShimmer(c).bone(c, radius)
}

func textBone(c *ui.Context, size, lineHeight float32) ui.Element {
	return pageShimmer(c).text(c, size, lineHeight)
}

// pillButton is a rounded button with an icon and a label.
func pillButton(c *ui.Context, icon *ui.SVG, label string, bg, fg ui.Color) ui.Element {
	b := ui.ButtonBase(c).Height(36).PaddingX(18).Gap(8).Radius(999).AlignItems(ui.Center).TextColor(fg)
	fill := bg
	if b.Hovered() {
		fill = bg.Mix(ui.RGB(255, 255, 255), 0.12)
	}
	b.Background(b.AnimateColor("bg", fill, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.96
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14)
		}
		ui.Text(c, label).FontSize(14).FontWeight(600).SingleLine()
	})
	return b
}

// shimmer is how the blocks of a skeleton look.
type shimmer struct {
	base, sheen ui.Color
	span        float32 // the width the band crosses: the window's
	still       bool    // the desktop asks for less motion: no band
}

const (
	shimmerPeriod = 1600 * time.Millisecond
	shimmerSweep  = 1100 * time.Millisecond // of the period, the band crossing; then it rests
	shimmerBand   = 260                     // DIPs
)

// pageShimmer is the shimmer of the pages, on their background.
func pageShimmer(c *ui.Context) shimmer {
	s := shimmer{base: ui.RGBA(0, 0, 0, 0.06), sheen: ui.RGBA(0, 0, 0, 0.022)}
	if c.Theme().Dark {
		s.base, s.sheen = ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.13)
	}
	return s.in(c)
}

func (s shimmer) in(c *ui.Context) shimmer {
	s.span, _ = c.Size()
	s.still = c.Preferences().ReduceMotion
	return s
}

// paint fills r, its corners rounded by radius, as a block of the skeleton.
func (s shimmer) paint(p *ui.Painter, r ui.Rect, radius float32) {
	if s.still || r.W <= 0 {
		p.Fill(r, s.base, radius)
		return
	}
	at := time.Duration(p.Now().UnixNano() % int64(shimmerPeriod))
	if at >= shimmerSweep {
		p.Fill(r, s.base, radius)
		p.After(shimmerPeriod - at)
		return
	}
	p.AnimationFrameRate(60)
	t := float32(at) / float32(shimmerSweep)
	x := -shimmerBand/2 + t*(s.span+shimmerBand)
	if x+shimmerBand/2 < r.X || x-shimmerBand/2 > r.X+r.W {
		p.Fill(r, s.base, radius)
		return
	}
	// The band's place along the block, which it may lie partly beyond.
	along := func(x float32) float32 { return (x - r.X) / r.W }
	p.FillGradient(r, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
		{Color: s.base, At: along(x - shimmerBand/2)},
		{Color: s.sheen, At: along(x)},
		{Color: s.base, At: along(x + shimmerBand/2)},
	}}, radius)
}

// bone is a block of the skeleton, as big as it is laid out.
func (s shimmer) bone(c *ui.Context, radius float32) ui.Element {
	return ui.Box(c).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) { s.paint(p, r, radius) })
}

// text is a line of text at size, as tall as the line and as wide as it
// is laid out, its block the height of the letters.
func (s shimmer) text(c *ui.Context, size, lineHeight float32) ui.Element {
	return ui.Box(c).Shrink(0).Height(size * lineHeight).Draw(func(p *ui.Painter, r ui.Rect) {
		h := size * 0.78
		s.paint(p, ui.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h}, h*0.32)
	})
}

type palette struct {
	dark      bool
	bg        ui.Color // the content's background
	sidebar   ui.Color // under the sidebar where no material shows
	text      ui.Color
	muted     ui.Color
	faint     ui.Color
	hairline  ui.Color
	hover     ui.Color
	press     ui.Color
	accent    ui.Color
	like      ui.Color // hearts, which stay red on a platform with another accent
	onAccent  ui.Color
	cardShade ui.Color
}

func paletteFor(t *ui.Theme) palette {
	if t.Dark {
		return palette{
			dark:      true,
			bg:        ui.Hex("#141416"),
			sidebar:   ui.Hex("#1c1c1f"),
			text:      ui.Hex("#f5f5f7"),
			muted:     ui.RGBA(235, 235, 245, 0.56),
			faint:     ui.RGBA(235, 235, 245, 0.3),
			hairline:  ui.RGBA(255, 255, 255, 0.07),
			hover:     ui.RGBA(255, 255, 255, 0.06),
			press:     ui.RGBA(255, 255, 255, 0.1),
			accent:    ui.Hex("#30a46c"),
			like:      ui.Oklch(0.66, 0.2, 22),
			onAccent:  ui.Hex("#ffffff"),
			cardShade: ui.RGBA(0, 0, 0, 0.45),
		}
	}
	return palette{
		bg:        ui.Hex("#ffffff"),
		sidebar:   ui.Hex("#f4f4f6"),
		text:      ui.Hex("#1d1d1f"),
		muted:     ui.RGBA(60, 60, 67, 0.62),
		faint:     ui.RGBA(60, 60, 67, 0.32),
		hairline:  ui.RGBA(0, 0, 0, 0.07),
		hover:     ui.RGBA(0, 0, 0, 0.04),
		press:     ui.RGBA(0, 0, 0, 0.08),
		accent:    ui.Hex("#30a46c"),
		like:      ui.Oklch(0.6, 0.21, 24),
		onAccent:  ui.Hex("#ffffff"),
		cardShade: ui.RGBA(0, 0, 0, 0.16),
	}
}

func iconButton(c *ui.Context, icon *ui.SVG, size float32, color ui.Color, label string) ui.Element {
	b := roundButton(c, size, color, label)
	b.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return b
}

func roundButton(c *ui.Context, size float32, color ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Size(size+16, size+16).Center().Radius(999).Label(label).TextColor(color)
	bg := ui.Transparent
	if b.Hovered() {
		bg = ui.RGBA(128, 128, 128, 0.14)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.88
	}
	b.AnimatePose(pose, ui.Snappy)
	return b
}

func (h *Home) cover(c *ui.Context, url string, px int, size, radius float32) ui.Element {
	p := paletteFor(c.Theme())
	box := ui.Box(c).Size(size, size).Radius(radius).Center()
	bmp, _ := h.host.Artwork(url, px)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(p.hover)
	}
	box.Children(func() {
		if in < 1 {
			ui.Icon(c, iconNote).Size(size*0.3, size*0.3).TextColor(p.faint).Opacity(1 - in)
		}
		artImage(c, bmp, radius, in)
	})
	return box
}

func artIn(el ui.Element, bmp *ui.Bitmap) float32 {
	if bmp == nil {
		return el.Jump("art", 0)
	}
	return el.AnimateTo("art", 1, artFade)
}

func artImage(c *ui.Context, bmp *ui.Bitmap, radius, in float32) {
	if bmp == nil || in <= 0 {
		return
	}
	img := ui.Image(c, bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Radius(radius)
	if in < 1 {
		img.Opacity(in)
	}
}

func segmented(c *ui.Context, selected *int, labels ...string) bool {
	t := c.Theme()
	const height, inset, radius = 24, 2, 6
	width := float32(0)
	for _, l := range labels {
		w, _ := c.MeasureText(0, ui.Span{Text: l, Size: 13})
		width = max(width, w+26)
	}
	track, capsule, divider := ui.RGBA(0, 0, 0, 0.06), ui.RGB(255, 255, 255), ui.RGBA(0, 0, 0, 0.13)
	if t.Dark {
		track, capsule, divider = ui.RGBA(255, 255, 255, 0.08), ui.RGBA(255, 255, 255, 0.24), ui.RGBA(255, 255, 255, 0.13)
	}
	before := *selected
	parts := ui.SegmentedBase(c, selected, len(labels))
	at := parts.Track.AnimateTo("at", float32(*selected), ui.Animation{Spring: ui.SpringOptions{Duration: 280 * time.Millisecond, Bounce: 0.08}})
	n := len(labels)
	dark := t.Dark
	parts.Track.Height(height).Padding(inset).Radius(radius).Background(track).Draw(func(p *ui.Painter, r ui.Rect) {
		// Hairlines between segments, but beside the capsule.
		for i := 1; i < n; i++ {
			if d := float32(i) - at; d > -1 && d < 2 {
				continue
			}
			x := r.X + inset + float32(i)*width
			p.Fill(ui.Rect{X: x - 0.5, Y: r.Y + 6, W: 1, H: r.H - 12}, divider, 0)
		}
		pill := ui.Rect{X: r.X + inset + at*width, Y: r.Y + inset, W: width, H: r.H - 2*inset}
		if !dark {
			p.Shadow(pill, radius-1, 0, 0.5, 1, 0, ui.RGBA(0, 0, 0, 0.2))
			p.Shadow(pill, radius-1, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06))
		}
		p.Fill(pill, capsule, radius-1)
	}).Children(func() {
		for i, l := range labels {
			seg := parts.Segment(i).Width(width).Height(height - 2*inset).Center().Radius(radius - 1)
			if seg.Pressed() && i != *selected {
				seg.Background(divider)
			}
			seg.Children(func() { ui.Text(c, l).FontSize(13).TextColor(t.Text).SingleLine() })
		}
	})
	return *selected != before
}

func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

var artFade = ui.Animation{Duration: 320 * time.Millisecond}

func (h *Home) coverAtSize(c *ui.Context, url string, size, radius float32) ui.Element {
	px := int(size * 3)
	if size <= 40 {
		px = 80
	}
	return h.cover(c, url, px, size, radius)
}

func playButton(c *ui.Context, playing bool, size float32, color ui.Color) ui.Element {
	b := roundButton(c, size, color, "播放/暂停")
	b.Children(func() { playPause(c, b, playing, size) })
	return b
}

func playPause(c *ui.Context, b ui.Element, playing bool, size float32) {
	to := float32(0)
	if playing {
		to = 1
	}
	v := b.AnimateTo("playing", to, ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.3}})
	ui.Box(c).Size(size, size).Children(func() {
		for _, icon := range []struct {
			svg   *ui.SVG
			shown float32
		}{{iconPlaySolid, 1 - v}, {iconPause, v}} {
			if icon.shown <= 0.01 {
				continue
			}
			s := 0.4 + 0.6*icon.shown
			ui.Icon(c, icon.svg).Absolute().Top(0).Left(0).Size(size, size).Scale(s, s).Opacity(min(icon.shown, 1))
		}
	})
}

func npShimmer(c *ui.Context) shimmer {
	return shimmer{base: white.Alpha(0.1), sheen: white.Alpha(0.2)}.in(c)
}

func lyricsSkeleton(c *ui.Context, s shimmer) {
	ui.Column(c).Gap(6).Padding(0, 8, 0, 0).Children(func() {
		for i := range 7 {
			ui.Box(c.Key(i)).Padding(10, 12).Children(func() {
				s.text(c, 30, 1.25).WidthPercent(boneWidth(i+1) * 0.9)
			})
		}
	})
}

func stageSkeleton(c *ui.Context, s shimmer, f, w float32) {
	ui.Row(c).Wrap().Justify(ui.Center).AlignItems(ui.Center).Padding(0, w*0.1).Gap(f * 0.35).Children(func() {
		for i, n := range []float32{2.2, 1.3, 3.1, 1.8} {
			s.bone(c.Key(i), f*0.22).Size(f*n, f*0.85)
		}
	})
}

func edgeShown(el ui.Element, under bool) float32 {
	target := float32(0)
	if under {
		target = 1
	}
	return el.AnimateTo("shown", target, ui.Animation{Duration: 180 * time.Millisecond})
}

func (h *Home) moreButton(c *ui.Context, key string, menu func(*ui.Menu)) ui.Element {
	b := iconButton(c.Key(key), iconMore, 18, paletteFor(c.Theme()).muted, "更多")
	b.Menu(menu)
	return b
}
