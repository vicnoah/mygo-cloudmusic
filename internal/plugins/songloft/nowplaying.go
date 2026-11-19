package songloft

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/lyrics"
)

var white = ui.RGB(255, 255, 255)

var npSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 450 * time.Millisecond, Bounce: 0.08}}

// npSettle is how long Now Playing takes to come or go, its spring at rest.
const npSettle = 600 * time.Millisecond

// npState tracks Now Playing opening and closing, and reports whether it is
// open and whether it has come to rest, open or closed. Call it once a
// build, before the window's content.
func (view *Home) npState(c *ui.Context) (open, settled bool) {
	open = view.nowPlaying && view.host.Playback().Current() != nil
	now := c.Now()
	if open != view.npOpen {
		view.npOpen, view.npChangedAt = open, now
		// A build once it is at rest: to stop painting what it covers, or
		// itself once gone.
		c.After(npSettle)
		// The window buttons light over its dark backdrop, and as the
		// theme over the pages (Windows).
		buttons := mygo.ThemeSystem
		if open {
			buttons = mygo.ThemeDark
		}
		view.host.SetMainWindowButtonsTheme(buttons)
	}
	return open, now.Sub(view.npChangedAt) >= npSettle
}

// nowPlayingLayer holds Now Playing over the whole window, rising into
// view as it opens and sinking away as it closes. The layer moves itself,
// staying on top all the way, rather than leaving as an exit transition
// would, under the rest of the window: that would show it going over the
// sidebar's material while the page above it hid it at once. Its pose
// moves it in frames that only paint, building nothing.
func (view *Home) nowPlayingLayer(c *ui.Context, open, settled bool) {
	layer := ui.Box(c.Key("now-playing-layer")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	pose := ui.Pose{Y: 60, Fade: 1}
	if open {
		pose = ui.Pose{}
	}
	layer.AnimatePose(pose, npSpring)
	if !open && settled || view.host.Playback().Current() == nil {
		// Closed: nothing to paint or click.
		layer.Invisible()
		view.queueAt, view.wallState.track = -1, 0
		return
	}
	if !open {
		// On its way out, the window under it takes the clicks.
		layer.PassThrough()
	}
	layer.Children(func() { view.nowPlayingView(c) })
}

// nowPlayingView fills the window with what plays: its cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
func (view *Home) nowPlayingView(c *ui.Context) {
	pl := view.host.Playback()
	tr := pl.Current()
	w, h := c.Size()
	art := view.host.DecodedArtwork(tr.Album.PicURL, 600, true)
	// The colors stay the last song's while its cover loads, rather than
	// going gray between the two.
	shown := art
	if art == nil && tr.Album.PicURL != "" {
		shown = view.npArt
	}
	view.npArt = shown
	tint := ui.RGB(120, 120, 130)
	if shown != nil && !shown.Failed() {
		tint = shown.Tint()
	}
	accent := tint.Mix(white, 0.45)

	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.Hex("#0b0b0d")).TextColor(white)
	// The pointer is over the view whenever it is in the window.
	pointerIn := root.Hovered()
	root.Children(func() {
		view.backdrop(c, shown, tint)
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() { view.nowPlayingBody(c, tr, art, accent, w, h, pointerIn) })
	})
}

func (view *Home) nowPlayingBody(c *ui.Context, tr *pluginapi.Track, art pluginapi.DecodedArtwork, accent ui.Color, w, h float32, pointerIn bool) {
	pl := view.host.Playback()

	if view.npPane != paneQueue {
		view.queueAt = -1
	}
	if view.npPane != paneWall {
		view.wallState.track = 0
	}
	{
		// The top strip drags the window; the chevron closes the view,
		// showing only while the pointer is in the window, as the pane tabs do.
		ui.Row(c).Height(topBar(c)).Justify(ui.Center).AlignItems(ui.Center).DragWindow().Children(func() {
			reveal(ui.Box(c.Key("close-reveal")), pointerIn).Children(func() {
				if iconButton(c.Key("close"), iconDown, 22, white.Alpha(0.7), "关闭").Clicked() {
					view.nowPlaying = false
				}
			})
		})
		if view.npPane == paneStage {
			// The stage takes the whole view: the lyrics are the show, the
			// controls a slim bar at the foot.
			reveal(npTabsRow(c, w), pointerIn).Children(func() {
				view.paneTabs(c)
				view.stageStyleButton(c)
			})
			view.stage(c, tr, accent)
			view.stageBar(c, tr, art, accent)
			return
		}
		if view.npPane == paneWall {
			// The wall takes the whole view too, under the song playing.
			view.wallPane(c, tr, art, accent, w, h, pointerIn)
			return
		}

		side := float32(math.Min(float64(w)*0.36, 440))
		// The cover leaves room above it for the tabs, starting as far
		// under them as the wall's song does.
		coverSize := min(side, h-436)
		// The column is as wide as the cover, for the cover, what plays
		// and the tabs over it to start at one edge, but wide enough for
		// the controls.
		side = max(coverSize, 280)
		padX := npSideX(w)
		reveal(npTabsRow(c, w), pointerIn).Children(func() { view.paneTabs(c) })
		ui.Row(c).Grow(1).Padding(0, padX, 44, padX).Gap(72).AlignItems(ui.Stretch).Children(func() {
			var coverTop float32
			ui.Column(c).Width(side).Justify(ui.Center).Gap(0).Children(func() {
				coverTop = ui.Box(c.Key("cover-top")).Height(0).LayoutBounds().Y
				view.npCover(c, tr, art, coverSize)

				ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(36, 0, 18, 0).Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
						if r := pl.Radio(); r != "" {
							view.radioChip(c, r)
						}
						ui.Text(c, tr.Name).FontSize(22).FontWeight(700).LetterSpacing(-0.3).SingleLine()
						view.artistLinks(c, tr, white.Alpha(0.6), white, true).FontSize(15).SingleLine()
					})
					view.likeButton(c.Key("like"), tr, 22, white.Alpha(0.7))
					view.songMenu(c.Key("more"), tr, 22, white.Alpha(0.7)).Margin(0, -8, 0, 0)
				})
				view.scrubber(c.Key("np-scrub"), &view.npScrub, white.Alpha(0.18), white.Alpha(0.9), 16, accent.Mix(white, 0.35))
				// As tall with the quality's badge as without it: the badge goes
				// while the next song loads, and the column would move with it.
				ui.Row(c).Height(16).AlignItems(ui.Center).Children(func() {
					ui.Text(c, clock(pl.Position())).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
					ui.Spacer(c)
					qualityBadge(c, pl.Stream())
					ui.Spacer(c)
					status := "-" + clock(pl.Duration()-pl.Position())
					if pl.Loading() {
						status = "正在缓冲…"
					}
					ui.Text(c, status).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
				})
				ui.Box(c).Margin(14, 0, 18, 0).Children(func() {
					view.transport(c, white, white.Alpha(0.45), accent, 28)
				})
				ui.Row(c).Justify(ui.Center).Children(func() {
					view.volumeSlider(c, white.Alpha(0.7), white.Alpha(0.18), 180)
				})
			})

			pane := ui.Column(c.Key("pane")).Grow(1).Basis(0).MinWidth(0)
			pane.Children(func() {
				// The pane starts level with the cover beside it, but below the
				// tabs, which start level with the pane's top: they come and go
				// with the pointer, so they sit out of the flow, never over it.
				b := pane.LayoutBounds()
				if b.H > 0 && coverTop > 0 {
					if top := max(coverTop-b.Y, npTabsHeight+8); top != view.npPaneTop {
						view.npPaneTop = top
						c.AnimationFrame()
					}
				} else {
					c.AnimationFrame() // not laid out yet: the next frame places it
				}
				ui.Box(c).Height(view.npPaneTop)
				switch view.npPane {
				case paneLyrics:
					view.lyricsPane(c, tr, h)
				default:
					view.queuePane(c)
				}
			})
		})
	}
}

// npMaxWidth is how wide the cover and the pane beside it go, at most.
const npMaxWidth = 1280

// npSideX is how far in from the sides of a view w wide the cover and the
// pane beside it are: in a wide window they keep together in the middle,
// rather than the pane taking all the width the cover leaves.
func npSideX(w float32) float32 {
	return max(72, (w-npMaxWidth)/2)
}

// npTabsHeight is how tall the row of Now Playing's tabs is, for what is
// under it to start below it.
const npTabsHeight = 28

// npTabsRow is the row of Now Playing's tabs, in the same place for every
// pane, so that they stay put going from one to another: at the top, level
// with the cover's left. They start from the left, so that only the last,
// the comments, moves as its count changes.
func npTabsRow(c *ui.Context, w float32) ui.Element {
	return ui.Row(c.Key("tabs")).Absolute().Top(topBar(c)).Left(npSideX(w)).Height(npTabsHeight).Gap(6).AlignItems(ui.Center)
}

// reveal fades e in while the pointer is in the window, and out when it
// leaves.
func reveal(e ui.Element, shown bool) ui.Element {
	pose := ui.Pose{Fade: 1}
	if shown {
		pose = ui.Pose{}
	}
	return e.AnimatePose(pose, ui.Animation{Duration: 200 * time.Millisecond})
}

// lyricsPaneChosen is the pane of lyrics the listener chose last: what
// the app starts on, before the player is made. Now Playing shows an
// episode's in place of either.
func (view *Home) lyricsPaneChosen() int {
	if view.settings.LyricsStage {
		return paneStage
	}
	return paneLyrics
}

// The panes of Now Playing.
const (
	paneLyrics = iota
	paneQueue
	paneStage
	paneWall
)

// npTab is a tab of Now Playing: its pane, and what it says.
type npTab struct {
	pane  int
	label string
}

// npTabs returns the tabs of Now Playing, in order, for what plays: ⌘1
// chooses the first, ⌘2 the second, and on.
func (view *Home) npTabs() []npTab {
	return []npTab{{paneLyrics, "歌词"}, {paneStage, "律动"}, {paneQueue, "待播清单"}, {paneWall, "选歌"}}
}

func (view *Home) paneTabs(c *ui.Context) {
	for _, t := range view.npTabs() {
		view.paneTab(c, t.pane, t.label)
	}
}

// choosePane shows pane i in Now Playing, remembering lyrics or the stage,
// whichever was chosen, for the next song.
func (view *Home) choosePane(i int) {
	view.npPane = i
	if (i == paneLyrics || i == paneStage) && view.settings.LyricsStage != (i == paneStage) {
		view.settings.LyricsStage = i == paneStage
		view.savePresentation()
	}
}

// radioChip says which radio plays, over the title: a click on it, or on
// its cross, leaves the radio, playing on the song.
func (view *Home) radioChip(c *ui.Context, name string) {
	icon, exit := iconRadio, "退出私人 FM"
	if name == heartRadio {
		icon, exit = iconHeartFill, "退出心动模式"
	}
	chip := ui.ButtonBase(c.Key("radio-chip")).Gap(5).Padding(3, 6, 3, 8).Margin(0, 0, 4, -8).Radius(999).
		AlignItems(ui.Center).AlignSelf(ui.Start).FocusRing(false).Label(exit).Tooltip(exit)
	bg, fg, cross := ui.Transparent, white.Alpha(0.6), float32(0.5)
	if chip.Hovered() {
		bg, fg, cross = white.Alpha(0.12), white.Alpha(0.85), 1
	}
	chip.Background(chip.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	chip.Children(func() {
		ui.Icon(c, icon).Size(12, 12)
		ui.Text(c, radioLabel(name)).FontSize(11.5).FontWeight(700).LetterSpacing(0.3)
		ui.Icon(c, iconX).Size(11, 11).Opacity(cross)
	})
	if chip.Clicked() {
		view.exitRadio()
	}
}

// qualityBadge marks a lossless stream, as it plays.
func qualityBadge(c *ui.Context, st *pluginapi.Stream) {
	if st == nil || st.Level != "lossless" && st.Level != "hires" {
		return
	}
	label := "无损"
	if st.Level == "hires" {
		label = "Hi-Res"
	}
	ui.Text(c, label).FontSize(9.5).FontWeight(800).LetterSpacing(0.6).TextColor(ui.Hex("#f5d58a")).
		Padding(1, 6).Radius(4).Border(1, ui.Hex("#f5d58a").Alpha(0.55))
}

// stageBar is the slim bar along the foot of the stage: what plays, where
// it is, and the controls.
func (view *Home) stageBar(c *ui.Context, tr *pluginapi.Track, art pluginapi.DecodedArtwork, accent ui.Color) {
	pl := view.host.Playback()
	ui.Column(c).Padding(0, 48, 28, 48).Gap(10).Children(func() {
		view.scrubber(c.Key("stage-scrub"), &view.npScrub, white.Alpha(0.18), white.Alpha(0.9), 14, accent.Mix(white, 0.35))
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				if bmp := bitmap(art); bmp != nil {
					cov := ui.Box(c.Key(tr.ID)).Size(44, 44)
					in := artIn(cov, bmp)
					cov.Children(func() { artImage(c, bmp, 8, in) })
				}
				ui.Column(c).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, tr.Name).FontSize(14).FontWeight(700).SingleLine()
					view.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12).SingleLine()
				})
				view.likeButton(c.Key("like"), tr, 17, white.Alpha(0.6))
				view.songMenu(c.Key("more"), tr, 17, white.Alpha(0.6)).Margin(0, 0, 0, -10)
			})
			view.transport(c, white, white.Alpha(0.45), white, 22)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).Children(func() {
				ui.Text(c, clock(pl.Position())+" / "+clock(pl.Duration())).FontSize(11.5).FontFeatures("tnum").TextColor(white.Alpha(0.5))
			})
		})
	})
}

func (view *Home) paneTab(c *ui.Context, i int, label string) {
	b := ui.ButtonBase(c.Key(i)).Padding(6, 14).Radius(999).FocusRing(false).Label(label)
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if view.npPane == i {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		bg = white.Alpha(0.06)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg)
	})
	if b.Clicked() {
		view.choosePane(i)
	}
}

func (view *Home) lyricsPane(c *ui.Context, tr *pluginapi.Track, h float32) {
	pl := view.host.Playback()
	lines, loaded := pl.Lyrics()
	if len(lines) == 0 {
		col := ui.Column(c).Grow(1)
		if loaded {
			col.Center()
		}

		col.Children(func() {
			if !loaded {
				lyricsSkeleton(c, npShimmer(c))
				return
			}
			ui.Text(c, "纯音乐，请欣赏").FontSize(24).FontWeight(700).TextColor(white.Alpha(0.4))
		})

		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	// Redraw as the next line begins.
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if view.lyricTrack != tr.ID {
		view.lyricTrack, view.lyricAt = tr.ID, -2
		view.lyricScroll, view.lyricGlide = ui.ScrollState{}, lyricGlide{}
	}
	lag := view.lyricGlide.step(c, &view.lyricScroll)

	scroll := ui.Scroll(c.Key("lyrics")).Grow(1).TrackScroll(&view.lyricScroll).Padding(0, 8, h*0.5, 0)

	view.timedLines(c, scroll, lines, pos, cur, lag, linesStyle{size: 30, weight: 800, lineHeight: 1.25}, nil)
}

// linesStyle is how timedLines sets its lines: the size, weight and
// leading of their type; quiet nearly hides them, under a hot comment.
type linesStyle struct {
	size       float32
	weight     int
	lineHeight float32
}

// timedLines fills scroll with lines that follow the track playing at
// pos: the line at cur lit, and brought to a third of the way down once
// it changes, but none with cur -1 before the first; a click on one seeks
// to it. The lines are held back by lag, how far the glide to the line
// lit has still to go (lyricGlide). foot, if set, follows the lines.
func (view *Home) timedLines(c *ui.Context, scroll ui.Element, lines []lyrics.Line, pos time.Duration, cur int, lag float32, st linesStyle, foot func()) {
	pl := view.host.Playback()
	var active ui.Element
	scroll.Children(func() {
		ui.Column(c).Gap(6).Translate(0, -lag).Children(func() {
			for i, l := range lines {
				line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(10, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
				on := i == cur
				alpha := float32(0.28)
				if on {
					alpha = 1
				} else if line.Hovered() {
					alpha = 0.55
				}
				if line.Hovered() {
					line.Background(white.Alpha(0.06))
				}
				scale := float32(0.94)
				if on {
					scale = 1
				}
				line.TransformOrigin(0, 0.5).AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.1}})
				line.Children(func() {
					// The line fades as a pose, in frames that only paint, rather
					// than building the view at the display's rate as it does.
					text := ui.Column(c).AlignItems(ui.Start).Gap(6)
					text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: 380 * time.Millisecond})
					text.Children(func() {
						if on && len(l.Words) > 0 {
							// The line sung lights up word by word as it is sung.
							ui.RichText(c.Key("words")).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).Children(func() {
								for wi, wd := range l.Words {
									lit := clamp01(float32((pos - wd.Start).Seconds() / max(0.05, (wd.End-wd.Start).Seconds())))
									ui.Text(c.Key(wi), wd.Text).TextColor(white.Alpha(0.38 + 0.62*lit))
								}
							})
							if pl.Playing() {
								// Words brighten smoothly enough at 25 frames a second.
								c.After(40 * time.Millisecond)
							}
						} else {
							ui.Text(c, l.Text).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).TextColor(white)
						}
						if l.Translation != "" {
							ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(0.7))
						}
					})
				})
				if line.Clicked() {
					pl.Seek(l.At)
				}
				if on {
					active = line
				}
			}
		})
		if foot != nil {
			foot()
		}
	})
	// Bring the line sung to a third of the way down, once it changed.
	if cur != view.lyricAt {
		box := scroll.Bounds()
		target := float32(0)
		if active.Valid() {
			// Where the glide is going, without where it still is.
			if lb := active.LayoutBounds(); lb.H > 0 && box.H > 0 {
				target = lb.Y - box.Y + view.lyricScroll.Y - box.H*0.3
			} else {
				return // not laid out yet: the next frame scrolls
			}
		}
		view.lyricAt = cur
		view.lyricGlide.to(c, &view.lyricScroll, min(max(0, target), view.lyricScroll.MaxY))
	}
}

// lyricGlide scrolls the lyrics to the line sung, as a critically damped
// spring, as ScrollTo's would. ScrollTo moves the offset in every frame,
// and MyGo builds the view again in the frame after each that moved it,
// and its spring creeps over the last pixel for half a second: with a line
// following another every second or two, most of a song. So the offset
// goes to the line at once, in one frame, and the lines are held back by
// what the glide has still to go, its lag, which settles to nothing within
// half a pixel.
type lyricGlide struct {
	on bool
	// The lag t after start: (a + b·t)·e^(−ωt), in DIPs.
	a, b  float64
	start time.Time
	// The offset it goes to, set in the next frame (pending), with the lag
	// that holds the lines back from it: the layout of the frame that
	// chose it takes the offset too, the lines built without the lag.
	target  float32
	pending bool
	set     float32 // the offset it set
}

// lyricOmega is the spring's ω: a line or two away, it comes within half a
// pixel in about 800 ms.
const lyricOmega = 10

// at returns the lag at now, and how fast it changes, in DIPs a second.
func (g *lyricGlide) at(now time.Time) (lag, v float64) {
	if !g.on {
		return 0, 0
	}
	t := now.Sub(g.start).Seconds()
	e := math.Exp(-lyricOmega * t)
	return (g.a + g.b*t) * e, (g.b - lyricOmega*(g.a+g.b*t)) * e
}

// to scrolls s to target from the next frame, gliding from where it
// shows, at the speed it went if it was on its way.
func (g *lyricGlide) to(c *ui.Context, s *ui.ScrollState, target float32) {
	now := c.Now()
	lag, v := g.at(now)
	shown := float64(s.Y) + lag
	g.target, g.pending = target, true
	g.on = !c.Preferences().ReduceMotion
	g.start, g.a = now, shown-float64(target)
	g.b = v + lyricOmega*g.a
	c.AnimationFrame()
}

// step returns the lag of the lines in this frame, and asks for the next
// while there is one. The listener scrolling stops the glide where it
// shows.
func (g *lyricGlide) step(c *ui.Context, s *ui.ScrollState) float32 {
	if g.pending {
		s.Y, g.set, g.pending = g.target, g.target, false
	}
	if !g.on {
		return 0
	}
	lag, v := g.at(c.Now())
	if s.Y != g.set {
		// The listener took over: the offset takes what showed.
		s.Y, g.on = max(0, min(s.Y+float32(lag), s.MaxY)), false
		return 0
	}
	if math.Abs(lag) < 0.5 && math.Abs(v) < 20 {
		g.on = false
		return 0
	}
	c.AnimationFrame()
	return float32(lag)
}

// queuePane lists the whole queue in play order, the track playing marked
// among the others: a click plays one there, and the list stays as it is.
func (view *Home) queuePane(c *ui.Context) {
	pl := view.host.Playback()
	order, at := pl.PlayOrder()
	// Opened, it shows the track playing at its top; after, it follows it
	// as little as keeps it in view.
	if at >= 0 && at != view.queueAt {
		if view.queueAt < 0 {
			view.queueList.ScrollTo(at, ui.Start)
		} else {
			view.queueList.ScrollIntoView(at)
		}
		view.queueAt = at
	}
	view.queueList.Key = func(row int) any { return order[row] }
	ui.List(c.Key("queue"), &view.queueList, len(order), func(r int) {
		i := order[r]
		tr := &pl.Queue()[i]
		playing := r == at
		row := ui.ButtonBase(c.Key(i)).Height(58).PaddingX(10).Gap(14).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
			ContextMenu(view.songItems(tr, 0))
		switch {
		case playing:
			row.Background(white.Alpha(0.12))
		case row.Hovered():
			row.Background(white.Alpha(0.08))
		}
		if !tr.Playable() {
			row.Opacity(0.4)
		}
		row.Children(func() {
			view.cover(c, tr.Album.PicURL, 96, 42, 7)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(600).SingleLine()
				view.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12.5).SingleLine()
			})
			ui.Text(c, clock(tr.Length())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.4))
		})
		if row.Clicked() && tr.Playable() && !playing {
			pl.Jump(i)
		}
	}).Grow(1).Padding(8, 0, 40, 0).Gap(2)
}
