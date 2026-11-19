package subsonic

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/eq"
)

type playbackView struct {
	pal                           palette
	settings                      pluginapi.Presentation
	volume                        float64
	volumeOpen                    bool
	mutedFrom                     float64
	barScrub                      scrubState
	npScrub                       scrubState
	nowPlaying                    bool
	npOpen                        bool
	npChangedAt                   time.Time
	npPane                        int
	npPaneTop                     float32
	lyricScroll                   ui.ScrollState
	lyricGlide                    lyricGlide
	lyricAt                       int
	lyricTrack                    int64
	queueList                     ui.ListState
	queueAt                       int
	backdropCache                 backdropCache
	npArt                         pluginapi.DecodedArtwork
	npCoverState                  coverState
	wallState                     wallState
	castOpen                      bool
	eqOpen                        bool
	eqUI                          eqUIState
	eqSettings                    eq.Settings
	desktopLyrics                 pluginapi.DesktopLyricsState
	desktopLyricsTrackID          int64
	desktopLyricsShownAt          int
	desktopLyricsPreviousAt       int
	desktopLyricsScrollGeneration int
}

func (h *Home) closePlaybackView() {
	if h.npOpen {
		h.host.SetMainWindowButtonsTheme(mygo.ThemeSystem)
	}
	h.npOpen, h.volumeOpen, h.castOpen, h.eqOpen = false, false, false, false
	h.nowPlaying = false
	h.eqUI.saveOpen, h.eqUI.deleteOpen, h.eqUI.autoeqOpen = false, false, false
	h.eqUI.autoeqLoading, h.eqUI.autoeqBusy = false, ""
}

func (h *Home) mainWindow(c *ui.Context) {
	pl := h.host.Playback()
	if pl == nil {
		return
	}
	h.settings = h.host.Presentation()
	h.volume = h.host.Volume()
	if h.host.EqualizerAvailable() {
		h.eqSettings = h.host.EqualizerSettings()
	}

	theme := *c.Theme()
	h.pal = paletteFor(&theme)
	p := h.pal
	theme.Accent, theme.AccentText, theme.Selection = p.accent, p.onAccent, p.accent.Alpha(0.3)
	c.SetTheme(&theme)
	h.shortcuts(c, pl)

	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	root := ui.Box(c).Fill()
	if !c.Focused(h.searchInput) {
		root.OnShortcut(0, ui.KeySpace, pl.TogglePlay)
		c.OnShortcut(0, ui.KeySpace, pl.TogglePlay)
	}
	open, settled := h.npState(c)
	root.Children(func() {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			if open && settled {
				return
			}
			side := ui.Column(c).Width(sidebarWidth)
			if !c.Vibrancy() {
				side.Background(p.sidebar)
			}
			side.Children(func() {
				top := float32(14)
				if c.TitleBar().Left > 0 {
					top = topBar(c) + 4
				}
				ui.Column(c).Fill().Padding(top, 14, 14, 14).DragWindow().Children(func() {
					h.sidebar(c)
				})
			})
			ui.Box(c.Key("main")).Grow(1).Background(p.bg).BorderWidth(0, 0, 0, 1).BorderColor(p.hairline).Children(func() {
				h.content(c)
				h.pageTop(c)
				h.playerBar(c)
			})
		})
		h.nowPlayingLayer(c, open, settled)
	})
	if message := h.host.TakeToast(); message != "" {
		c.Toast(message)
	}
}

func (h *Home) shortcuts(c *ui.Context, pl pluginapi.PlaybackControls) {
	back := func() {
		if h.nowPlaying {
			h.nowPlaying = false
			return
		}
		h.Back()
		h.host.Invalidate()
	}
	forward := func() {
		h.Forward()
		h.host.Invalidate()
	}
	c.OnShortcut(0, ui.KeyBack, back)
	c.OnShortcut(0, ui.KeyForward, forward)
	c.OnShortcut(ui.Cmd, ui.KeyBracketLeft, back)
	c.OnShortcut(ui.Cmd, ui.KeyBracketRight, forward)
	c.OnShortcut(ui.Cmd, ui.KeyF, func() {
		h.nowPlaying = false
		h.searchInput.Focus(c)
	})
	if h.nowPlaying {
		c.OnShortcut(0, ui.KeyEscape, func() { h.nowPlaying = false })
		if pl.Current() != nil {
			for i, tab := range h.npTabs() {
				c.OnShortcut(ui.Cmd, ui.Key1+ui.Key(i), func() { h.choosePane(tab.pane) })
			}
		}
	}
}

func (h *Home) pageTop(c *ui.Context) {
	top, _ := h.insets(c)
	p := h.pal
	edge := ui.Box(c.Key("platform-top-edge")).Absolute().Top(0).Left(0).Right(0).Height(top).
		DragWindow().Material(glass.ScrollEdge{Hard: true, Background: p.bg})
	edge.Opacity(edgeShown(edge, h.underTop))
	if !h.CanBack() && !h.CanForward() {
		return
	}
	ui.Row(c).Absolute().Top(0).Left(20).Height(topBar(c)).AlignItems(ui.Center).Children(func() {
		back := iconButton(c.Key("platform-back"), iconBack, 18, p.muted, "返回").Disabled(!h.CanBack())
		if !h.CanBack() {
			back.Opacity(0.35)
		}
		if back.Clicked() {
			h.Back()
			h.host.Invalidate()
		}
		if h.CanForward() && iconButton(c.Key("platform-forward"), iconForward, 18, p.muted, "前进").Clicked() {
			h.Forward()
			h.host.Invalidate()
		}
	})
}

func (h *Home) savePresentation() {
	s := h.settings
	h.host.SetLyricsPresentation(s.LyricsStage, s.StageStyle, s.StageSparks)
}

func bitmap(art pluginapi.DecodedArtwork) *ui.Bitmap {
	if art == nil {
		return nil
	}
	return art.Bitmap()
}

func (h *Home) songMenu(c *ui.Context, tr *pluginapi.Track, size float32, color ui.Color) ui.Element {
	button := iconButton(c, iconMore, size, color, "更多")
	button.Menu(h.songItems(tr, 0))
	return button
}

func (h *Home) songItems(tr *pluginapi.Track, _ int64) func(*ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item("复制歌名").Chosen() {
			h.host.CopyText(tr.Name)
		}
		if m.Item("复制歌名和歌手").Chosen() {
			h.host.CopyText(tr.Name + " - " + tr.ArtistNames())
		}
		m.Separator()
		if m.Item("复制链接").Chosen() {
			h.host.CopyText("")
		}
		linked := tr.Album.ID != 0
		for _, artist := range tr.Artists {
			linked = linked || artist.ID != 0
		}
		if linked {
			m.Separator()
		}
		for _, artist := range tr.Artists {
			if artist.ID != 0 && m.Item("查看歌手："+artist.Name).Chosen() {
				h.showRoute(Route{Page: "artist", Kind: "artist", ID: artist.ID})
			}
		}
		if tr.Album.ID != 0 && m.Item("查看专辑："+tr.Album.Name).Chosen() {
			h.showRoute(Route{Page: "album", Kind: "album", ID: tr.Album.ID})
		}
	}
}

const heartRadio = "heart"

func radioLabel(name string) string {
	switch name {
	case heartRadio:
		return "心动模式"
	case "fm:DEFAULT":
		return "私人 FM · 推荐"
	case "fm:FAMILIAR":
		return "私人 FM · 熟悉"
	case "fm:EXPLORE":
		return "私人 FM · 探索"
	default:
		return "私人 FM"
	}
}

func (h *Home) exitRadio() {
	pl := h.host.Playback()
	if name := pl.Radio(); name != "" {
		pl.EndRadio()
		label := "私人 FM"
		if name == heartRadio {
			label = "心动模式"
		}
		h.host.Toast("已退出" + label)
	}
}
