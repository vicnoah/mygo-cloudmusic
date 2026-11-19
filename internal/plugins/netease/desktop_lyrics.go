package netease

// The desktop lyrics window keeps its controls independent from Now Playing.

import (
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi/lyrics"
	"waygu-music/internal/plugins/netease/internal/client"
)

const (
	desktopLyricsWidth  = 900
	desktopLyricsHeight = 160
)

func (a *app) desktopLyricsView(c *ui.Context) {
	a.desktopLyrics = a.host.DesktopLyricsState()
	c.Root().Background(ui.Transparent)
	if a.pl.Playing() {
		// A steady 30 FPS is enough for word fill and stops immediately when
		// playback is paused.
		c.After(33 * time.Millisecond)
	}
	ui.Box(c).Size(desktopLyricsWidth, desktopLyricsHeight).Children(func() {
		a.desktopLyricsLyrics(c)
		if !a.desktopLyrics.Locked {
			// Unlocking makes the lyric surface a native drag area. The toolbar
			// is a child above it, so its buttons continue to receive clicks.
			ui.Box(c.Key("drag")).Absolute().Top(46).Left(0).Right(0).Bottom(0).DragWindow()
			if a.desktopLyrics.PointerInside {
				a.desktopLyricsControls(c)
			}
		}
	})
}

func (a *app) desktopLyricsLyrics(c *ui.Context) {
	tr := a.pl.Current()
	if tr == nil {
		return
	}
	lines, loaded := a.pl.Lyrics()
	pos := a.pl.Position()
	cur := lyrics.Index(lines, pos)
	if a.desktopLyricsTrackID != tr.ID {
		a.desktopLyricsTrackID = tr.ID
		a.desktopLyricsShownAt = -1
		a.desktopLyricsPreviousAt = -1
		a.desktopLyricsScrollGeneration = 0
	}
	// Keep this layer pass-through so only the toolbar captures mouse input.
	ui.Column(c.Key("lyrics")).Absolute().Top(0).Left(0).Size(desktopLyricsWidth, desktopLyricsHeight).
		Justify(ui.End).AlignItems(ui.Center).Padding(0, 0, 2, 0).Gap(4).PassThrough().Children(func() {
		switch {
		case !loaded:
			desktopLyricsMessage(c, tr.Name, 28, 800)
		case len(lines) == 0:
			desktopLyricsMessage(c, desktopLyricsTrackLabel(tr), 26, 700)
		case cur < 0:
			desktopLyricsMessage(c, lines[0].Text, 28, 800)
		default:
			a.desktopLyricsLine(c, lines, cur, pos)
		}
	})
}

func (a *app) desktopLyricsControls(c *ui.Context) {
	fill := ui.RGBA(24, 24, 28, 0.82)
	fg := ui.RGBA(255, 255, 255, 0.9)
	muted := ui.RGBA(255, 255, 255, 0.68)
	ui.Row(c.Key("toolbar")).Absolute().Top(8).Left((desktopLyricsWidth-360)/2).Height(38).PaddingX(8).
		Gap(1).AlignItems(ui.Center).Radius(19).Background(fill).Border(0.5, ui.RGBA(255, 255, 255, 0.12)).Children(func() {
		lockIcon, lockLabel := iconLock, "解锁桌面歌词"
		if !a.desktopLyrics.Locked {
			lockIcon, lockLabel = iconUnlock, "锁定桌面歌词"
		}
		if iconButton(c.Key("lock"), lockIcon, 17, fg, lockLabel).Clicked() {
			a.host.ToggleDesktopLyricsLock()
		}
		if iconButton(c.Key("prev"), iconPrev, 17, fg, "上一首").Clicked() {
			a.pl.Prev()
		}
		if playButton(c.Key("play"), a.pl.Playing(), 17, fg).Clicked() {
			a.pl.TogglePlay()
		}
		if iconButton(c.Key("next"), iconNext, 17, fg, "下一首").Clicked() {
			a.pl.Next()
		}
		translation := muted
		if a.desktopLyrics.ShowTranslation {
			translation = fg
		}
		if desktopLyricsTextButton(c.Key("translation"), "译", translation, "切换翻译").Clicked() {
			a.desktopLyrics.ShowTranslation = !a.desktopLyrics.ShowTranslation
			a.host.SetDesktopLyricsStyle(a.desktopLyrics.ShowTranslation, a.desktopLyrics.FontSize, a.desktopLyrics.Color)
		}
		if desktopLyricsTextButton(c.Key("font-minus"), "A-", fg, "减小字号").Clicked() {
			a.desktopLyrics.FontSize = max(22, a.desktopLyrics.FontSize-2)
			a.host.SetDesktopLyricsStyle(a.desktopLyrics.ShowTranslation, a.desktopLyrics.FontSize, a.desktopLyrics.Color)
		}
		if desktopLyricsTextButton(c.Key("font-plus"), "A+", fg, "增大字号").Clicked() {
			a.desktopLyrics.FontSize = min(40, a.desktopLyrics.FontSize+2)
			a.host.SetDesktopLyricsStyle(a.desktopLyrics.ShowTranslation, a.desktopLyrics.FontSize, a.desktopLyrics.Color)
		}
		if desktopLyricsColorButton(c.Key("color"), a.desktopLyricsAccent(), "切换歌词颜色").Clicked() {
			a.desktopLyrics.Color = (a.desktopLyrics.Color + 1) % len(desktopLyricsColors)
			a.host.SetDesktopLyricsStyle(a.desktopLyrics.ShowTranslation, a.desktopLyrics.FontSize, a.desktopLyrics.Color)
		}
		if iconButton(c.Key("close"), iconX, 17, muted, "关闭桌面歌词").Clicked() {
			a.host.HideDesktopLyrics()
		}
	})
}

func desktopLyricsMessage(c *ui.Context, text string, fontSize float32, weight int) {
	lineHeight := fontSize * 1.2
	ui.Box(c).Width(desktopLyricsWidth).Height(lineHeight + 4).Children(func() {
		ui.Text(c, text).Absolute().Top(2).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(weight).FixedLineHeight(lineHeight).TextColor(ui.RGBA(0, 0, 0, 0.72)).TextAlign(ui.Center).SingleLine().Label("桌面歌词提示阴影")
		ui.Text(c, text).Absolute().Top(0).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(weight).FixedLineHeight(lineHeight).TextColor(ui.RGBA(255, 255, 255, 0.96)).TextAlign(ui.Center).SingleLine().Label("桌面歌词提示")
	})
}

func desktopLyricsTextButton(c *ui.Context, text string, color ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Center().Radius(999).Label(label).TextColor(color)
	if b.Hovered() {
		b.Background(ui.RGBA(128, 128, 128, 0.14))
	}
	b.Children(func() { ui.Text(c, text).FontSize(13).FontWeight(600).SingleLine() })
	return b
}

func desktopLyricsColorButton(c *ui.Context, color ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Center().Radius(999).Label(label)
	if b.Hovered() {
		b.Background(ui.RGBA(128, 128, 128, 0.14))
	}
	b.Children(func() { ui.Box(c).Size(12, 12).Radius(999).Background(color) })
	return b
}

func (a *app) desktopLyricsAccent() ui.Color {
	if a.desktopLyrics.Color == 0 {
		if a.pal.accent.A != 0 {
			return a.pal.accent
		}
		return ui.RGB(255, 116, 80)
	}
	return desktopLyricsColors[a.desktopLyrics.Color%len(desktopLyricsColors)]
}

var desktopLyricsColors = []ui.Color{
	ui.RGB(53, 197, 240),
	ui.RGB(255, 209, 102),
	ui.RGB(255, 107, 157),
	ui.RGB(95, 215, 135),
	ui.RGB(120, 170, 255),
	ui.RGB(180, 120, 255),
	ui.RGB(255, 145, 85),
	ui.RGB(245, 245, 245),
}

func desktopLyricsTrackLabel(tr *netease.Track) string {
	if artists := tr.ArtistNames(); artists != "" {
		return tr.Name + " - " + artists
	}
	return tr.Name
}

func (a *app) desktopLyricsLine(c *ui.Context, lines []lyrics.Line, cur int, pos time.Duration) {
	fontSize := a.desktopLyrics.FontSize
	if fontSize <= 0 {
		fontSize = 30
	}
	// Give both rows the same fixed text line height and a few DIPs of room
	// for the shadow. The extra room keeps the glyph bottoms inside the clip
	// while one pair moves past the next.
	textLineHeight := fontSize * 1.2
	rowHeight := textLineHeight + 4
	pairHeight := rowHeight*2 + 4
	if a.desktopLyricsShownAt != cur {
		if a.desktopLyricsShownAt >= 0 && a.desktopLyricsShownAt < len(lines) {
			a.desktopLyricsPreviousAt = a.desktopLyricsShownAt
			a.desktopLyricsScrollGeneration++
		}
		a.desktopLyricsShownAt = cur
	}
	stage := ui.Box(c.Key("lyric-stage")).Width(desktopLyricsWidth).Height(pairHeight).Clip()
	if a.desktopLyricsScrollGeneration == 0 {
		stage.Children(func() {
			a.desktopLyricsPair(c, lines[cur], pos, fontSize, textLineHeight, rowHeight, true)
		})
		return
	}
	// Keep the previous pair just above the current pair while the viewport
	// animates upward. The previous pair is never left visible after settling.
	previous := lines[max(0, a.desktopLyricsPreviousAt)]
	progress := stage.AnimateTo("line-scroll", float32(a.desktopLyricsScrollGeneration), ui.Animation{Duration: 260 * time.Millisecond})
	offset := -(progress - float32(a.desktopLyricsScrollGeneration-1)) * pairHeight
	stage.Children(func() {
		ui.Column(c).Translate(0, offset).Children(func() {
			a.desktopLyricsPair(c, previous, previous.End, fontSize, textLineHeight, rowHeight, false)
			a.desktopLyricsPair(c, lines[cur], pos, fontSize, textLineHeight, rowHeight, true)
		})
	})
}

func (a *app) desktopLyricsPair(c *ui.Context, l lyrics.Line, pos time.Duration, fontSize, textLineHeight, rowHeight float32, live bool) {
	ui.Column(c).Width(desktopLyricsWidth).Height(rowHeight*2 + 4).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(desktopLyricsWidth).Height(rowHeight).Children(func() {
			// Keep the shadow inside the original-line box. An absolute
			// translation shadow here would be positioned against the whole pair
			// and cover the original lyric.
			if !live || len(l.Words) == 0 {
				ui.Text(c, l.Text).Absolute().Top(2).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(800).FixedLineHeight(textLineHeight).TextColor(ui.RGBA(0, 0, 0, 0.72)).TextAlign(ui.Center).SingleLine().Label("桌面歌词原文阴影")
				ui.Text(c, l.Text).Absolute().Top(0).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(800).FixedLineHeight(textLineHeight).TextColor(ui.RGBA(255, 255, 255, 0.96)).TextAlign(ui.Center).SingleLine().Label("桌面歌词原文")
				return
			}
			spans := a.desktopLyricsWordSpans(l.Words, pos, fontSize)
			// Derive the shadow from the already-built foreground spans. Keeping
			// the exact same runs is important: a space belongs to the preceding
			// word in YRC, and rebuilding the runs independently can move only the
			// shadow when the lyric contains spaces.
			shadowSpans := desktopLyricsShadowSpans(spans)
			ui.RichText(c, shadowSpans...).Absolute().Top(2).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(800).FixedLineHeight(textLineHeight).TextAlign(ui.Center).SingleLine().Label("桌面歌词原文阴影")
			ui.RichText(c, spans...).Absolute().Top(0).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(800).FixedLineHeight(textLineHeight).TextAlign(ui.Center).SingleLine().Label("桌面歌词原文")
		})
		if a.desktopLyrics.ShowTranslation && l.Translation != "" {
			ui.Box(c).Width(desktopLyricsWidth).Height(rowHeight).Children(func() {
				accent := a.desktopLyricsAccent()
				ui.Text(c, l.Translation).Absolute().Top(2).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(600).FixedLineHeight(textLineHeight).TextColor(ui.RGBA(0, 0, 0, 0.72)).TextAlign(ui.Center).SingleLine().Label("桌面歌词译文阴影")
				ui.Text(c, l.Translation).Absolute().Top(0).Left(0).Width(desktopLyricsWidth).FontSize(fontSize).FontWeight(600).FixedLineHeight(textLineHeight).TextColor(accent.Alpha(0.96)).TextAlign(ui.Center).SingleLine().Label("桌面歌词译文")
			})
		} else {
			ui.Box(c).Width(desktopLyricsWidth).Height(rowHeight)
		}
	})
}

func (a *app) desktopLyricsWordSpans(words []lyrics.Word, pos time.Duration, fontSize float32) []ui.Span {
	spans := make([]ui.Span, 0, len(words))
	for _, w := range words {
		color := ui.RGBA(255, 255, 255, 0.96)
		if pos >= w.End {
			color = a.desktopLyricsAccent()
		}
		spans = append(spans, ui.Span{Text: w.Text, Size: fontSize, Weight: 800, Color: color})
	}
	return spans
}

func desktopLyricsShadowSpans(foreground []ui.Span) []ui.Span {
	shadow := make([]ui.Span, len(foreground))
	for i, span := range foreground {
		span.Color = ui.RGBA(0, 0, 0, 0.72)
		shadow[i] = span
	}
	return shadow
}
