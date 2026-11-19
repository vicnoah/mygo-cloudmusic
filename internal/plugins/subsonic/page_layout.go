package subsonic

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
	"waygu-music/internal/pluginapi"
)

const (
	sidebarWidth    = 232
	playerBarHeight = 84
	pagePad         = 40
	pageGap         = 12
	topEdgePad      = 6
	topBarHeight    = 40
	buttonsHeight   = topBarHeight + topEdgePad
)

func (h *Home) insets(c *ui.Context) (top, bottom float32) {
	top = topBar(c)
	if runtime.GOOS == "darwin" {
		// The edge reaches a little below the traffic lights' strip.
		top += topEdgePad
	}
	return top, playerBarHeight
}

func topBar(c *ui.Context) float32 {
	if runtime.GOOS != "darwin" {
		// The whole edge, which the buttons fill: what sits in the strip
		// centers on their glyphs.
		return buttonsHeight
	}
	return max(c.TitleBar().Height, topBarHeight)
}

func (h *Home) startMark(c *ui.Context) {
	b := ui.Box(c.Key("start-mark")).Height(0).Bounds()
	if b.W > 0 {
		top, _ := h.insets(c)
		h.underTop = b.Y < top+pageGap-0.5
	}
}

func (h *Home) endMark(c *ui.Context) {
	b := ui.Box(c.Key("end-mark")).Height(0).Bounds()
	if b.W > 0 {
		_, height := c.Size()
		_, bottom := h.insets(c)
		h.underBottom = b.Y > height-bottom+0.5
	}
}

func (h *Home) centered(c *ui.Context, err string, retry func()) {
	top, bottom := h.insets(c)
	ui.Column(c).Grow(1).Padding(top, 0, bottom, 0).Center().Gap(12).Children(func() {
		ui.Text(c, err).TextColor(paletteFor(c.Theme()).muted).FontSize(13)
		if retry != nil && ui.Button(c, "重试").Clicked() {
			retry()
		}
	})
}

func (h *Home) columns(c *ui.Context, min float32) int {
	w, _ := c.Size()
	inner := w - sidebarWidth - 2*pagePad
	return max(2, int((inner+20)/(min+20)))
}

func (h *Home) heading(c *ui.Context, title, sub string) {
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		if sub != "" {
			ui.Text(c, sub).FontSize(12).FontWeight(600).TextColor(paletteFor(c.Theme()).muted).LetterSpacing(0.4)
		}
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
	})
}

func (h *Home) section(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(34, 0, 14, 0)
}

func (h *Home) nowPlayingBars(c *ui.Context) {
	color := paletteFor(c.Theme()).accent
	ui.Box(c).Size(16, 14).Draw(func(p *ui.Painter, r ui.Rect) {
		lv, live := h.host.PlaybackLevels()
		h.rowBars.step(p.Now(), lv, live)
		drawBars(p, r, &h.rowBars, 4, color, 2.5)
		if live || h.rowBars.group(0, pluginapi.SpectrumBands) > 0.01 {
			p.AnimationFrame()
		}
	})
}

func (h *Home) cardWidth(c *ui.Context) (int, float32) {
	const gap = 20
	w, _ := c.Size()
	cols := h.columns(c, 168)
	inner := w - sidebarWidth - 2*pagePad
	return cols, (inner - gap*float32(cols-1)) / float32(cols)
}

type barState struct {
	h    [pluginapi.SpectrumBands]float32
	last time.Time
}

func (st *barState) step(now time.Time, target [pluginapi.SpectrumBands]float32, live bool) {
	dt := float32(now.Sub(st.last).Seconds())
	if st.last.IsZero() || dt > 0.2 {
		dt = 0.016
	}
	st.last = now
	for i := range st.h {
		tg := target[i]
		if !live {
			tg = 0
		}
		k := float32(18) // rises fast
		if tg < st.h[i] {
			k = 6 // falls slowly
		}
		st.h[i] += (tg - st.h[i]) * min(1, k*dt)
	}
}

// group returns the mean of bands [from, to).
func (st *barState) group(from, to int) float32 {
	var s float32
	for i := from; i < to; i++ {
		s += st.h[i]
	}
	return s / float32(to-from)
}

func drawBars(p *ui.Painter, r ui.Rect, st *barState, n int, color ui.Color, minH float32) {
	gap := r.W / float32(n) * 0.38
	w := (r.W - gap*float32(n-1)) / float32(n)
	per := pluginapi.SpectrumBands / n
	for i := range n {
		// Shape the response: low bands are loud already.
		v := st.group(i*per, (i+1)*per)
		v = v * v * 1.15
		h := max(minH, min(r.H, r.H*v))
		x := r.X + float32(i)*(w+gap)
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-h)/2, W: w, H: h}, color, w/2)
	}
}

func (h *Home) resetPageLayout() {
	h.trackList, h.platformCardList = ui.ListState{}, ui.ListState{}
	h.platformHeroSince = time.Time{}
	h.underTop, h.underBottom = false, false
	h.laidOutRoute = h.route
}
