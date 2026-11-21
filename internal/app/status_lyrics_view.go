package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const taskbarLyricRowPadding = 1

type taskbarLyricStyle struct {
	original, translation string
	font                  string
	size, height          float32
}

type taskbarLyricRows struct {
	style                 taskbarLyricStyle
	original, translation *ui.TextLayout
}

func taskbarLyricsView(c *ui.Context, frame statusLyricsFrame, dark bool) {
	c.Root().Background(ui.Transparent)
	width, height := c.Size()
	color := ui.RGB(33, 33, 36)
	if dark {
		color = ui.RGB(245, 245, 247)
	}
	original, translation := frame.original, frame.translation
	if original == "" {
		original, translation = translation, ""
	}
	font := frame.font
	if font == "" {
		font = c.Theme().Font
	}
	style := taskbarLyricStyle{original, translation, font, float32(frame.fontSize), height}
	const padding, gap = 2, 7
	available := width - 2*padding
	coverSize := float32(0)
	if frame.cover != nil && frame.cover.bitmap != nil {
		coverSize = min(float32(32), max(1, height-4))
		available -= coverSize + gap
	}
	progress, wake := taskbarLyricsMotion(frame.timing, frame.position)
	scrolling := false
	row := ui.Row(c).Fill().PaddingX(padding).Gap(gap).AlignItems(ui.Center)
	rows := ui.Local(row, "lyric layouts", func() taskbarLyricRows { return taskbarLyricRows{} })
	if rows.style != style {
		*rows = shapeTaskbarLyricRows(style)
	}
	row.Children(func() {
		if coverSize > 0 {
			ui.Image(c, frame.cover.bitmap).Size(coverSize, coverSize).Shrink(0).Fit(ui.Cover).Radius(5)
		}
		if rows.original == nil || available <= 0 {
			return
		}
		ui.Column(c).Width(available).Shrink(0).Children(func() {
			scrolling = drawTaskbarLyricRow(c.Key("original"), rows.original, original, color, available, progress)
			if rows.translation != nil {
				translated := drawTaskbarLyricRow(c.Key("translation"), rows.translation, translation, color.Alpha(0.78), available, progress)
				scrolling = scrolling || translated
			}
		})
	})
	if frame.playing && scrolling && wake > 0 {
		c.After(wake)
	}
}

func shapeTaskbarLyricRows(style taskbarLyricStyle) taskbarLyricRows {
	rows := taskbarLyricRows{style: style}
	if style.original == "" {
		return rows
	}
	budget := style.height - 2*taskbarLyricRowPadding
	secondary := max(8, style.size-3)
	if style.translation != "" {
		budget -= 2 * taskbarLyricRowPadding
	}
	// Shaping accounts for the selected face and fallback glyphs. Fit both
	// real line heights before installing the scrolling viewport.
	for size := style.size; size >= 1 && budget > 0; {
		original := ui.ShapeText(style.original, ui.Font{Family: style.font, Size: size, Weight: 600}, 0)
		_, total := original.Size()
		var translation *ui.TextLayout
		if style.translation != "" {
			translation = ui.ShapeText(style.translation, ui.Font{Family: style.font, Size: secondary, Weight: 400}, 0)
			_, height := translation.Size()
			total += height
		}
		if total <= budget {
			rows.original, rows.translation = original, translation
			return rows
		}
		scale := min(0.95, budget/total)
		size, secondary = size*scale, secondary*scale
	}
	return rows
}

func drawTaskbarLyricRow(c *ui.Context, layout *ui.TextLayout, label string, color ui.Color, available, progress float32) bool {
	width, height := layout.Size()
	overflow := max(0, width-available)
	offset := max(0, (available-width)/2) - overflow*progress
	ui.Box(c).Size(available, height+2*taskbarLyricRowPadding).Label(label).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Clip(r, 0, func() {
			p.TextLayout(layout, r.X+offset, r.Y+taskbarLyricRowPadding, color)
		})
	})
	return overflow > 0
}

func taskbarLyricsMotion(timing statusLyricsTiming, position time.Duration) (float32, time.Duration) {
	if timing.end <= timing.start {
		return 0, 0
	}
	hold := min(350*time.Millisecond, (timing.end-timing.start)/8)
	start, end := timing.start+hold, timing.end-hold
	if position < start {
		return 0, start - position
	}
	if position >= end {
		return 1, 0
	}
	progress := float32(position-start) / float32(end-start)
	return progress, min(time.Second/30, end-position)
}
