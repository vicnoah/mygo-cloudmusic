package app

import (
	"bytes"
	"image"
	"image/png"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/artwork"
	"github.com/waygu-music-plugin/sdk/pluginapi/lyrics"
	"github.com/waygu-music-plugin/sdk/protocol"
	"golang.org/x/image/draw"
)

// statusLyrics uses the host's playback clock even when a plugin window is hidden.
type statusLyrics struct {
	title          string
	timer          *time.Timer
	tick           uint64
	native         statusLyricsNative
	track          *pluginapi.Track
	coverRequested bool
	cover          *statusLyricsCover
	trayCover      *statusLyricsCover
	trayText       string
	trayPages      []string
}

type statusLyricsCover struct {
	bitmap *ui.Bitmap
	png    []byte
}

type statusLyricsLine struct {
	original, translation string
}

type statusLyricsFrame struct {
	statusLyricsLine
	timing   statusLyricsTiming
	position time.Duration
	playing  bool
	cover    *statusLyricsCover
	fontSize int
	font     string
}

type statusLyricsTiming struct {
	start, end time.Duration
}

func statusLyricsAvailable() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin" || runtime.GOOS == "linux"
}

func taskbarLyricsFontSize(size int) int {
	if runtime.GOOS != "windows" {
		return 0
	}
	if size < pluginapi.TaskbarLyricsMinFontSize || size > pluginapi.TaskbarLyricsMaxFontSize {
		return pluginapi.TaskbarLyricsDefaultFontSize
	}
	return size
}

func (a *app) setTaskbarLyricsFontSize(size int) error {
	if size == 0 || taskbarLyricsFontSize(size) != size {
		return protocol.Failure{Code: "invalid-settings"}
	}
	if a.settings.TaskbarLyricsFontSize == size {
		return nil
	}
	next := a.settings
	next.TaskbarLyricsFontSize = size
	return a.saveLyricsAppearance(next)
}

func (a *app) setLyricsFontFamily(family string) error {
	if len(family) > 256 || !utf8.ValidString(family) || strings.ContainsFunc(family, unicode.IsControl) {
		return protocol.Failure{Code: "invalid-settings"}
	}
	if a.settings.LyricsFontFamily == family {
		return nil
	}
	next := a.settings
	next.LyricsFontFamily = family
	return a.saveLyricsAppearance(next)
}

func (a *app) saveLyricsAppearance(next settings) error {
	if err := next.save(); err != nil {
		return err
	}
	a.settings = next
	a.syncStatusLyrics()
	a.desktopTasksChanged()
	return nil
}

func (a *app) setStatusLyrics(enabled bool) error {
	if !statusLyricsAvailable() || a.settings.StatusLyrics == enabled {
		return nil
	}
	next := a.settings
	next.StatusLyrics = enabled
	if err := next.save(); err != nil {
		return err
	}
	a.settings = next
	if a.win != nil {
		a.applyTray()
		if a.tray != nil {
			a.tray.SetMenu(a.trayMenu())
		}
	}
	a.desktopTasksChanged()
	return nil
}

func (a *app) syncStatusLyrics() {
	s := &a.statusLyrics
	s.tick++
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	enabled := a.settings.StatusLyrics && statusLyricsAvailable() && !a.quitting && a.win != nil && a.pl != nil
	var track *pluginapi.Track
	var data []byte
	var line statusLyricsLine
	var timing statusLyricsTiming
	var position time.Duration
	var playing bool
	var after time.Duration
	if enabled {
		track, data = a.pl.Current(), a.pl.CurrentArtwork()
		if track != nil && !a.pl.Loading() {
			lines, loaded := a.pl.Lyrics()
			if loaded {
				position, playing = a.pl.Position(), a.pl.Playing()
				line, timing, after = statusLyricAt(lines, position)
				if duration := a.pl.Duration(); duration > timing.start {
					timing.end = min(timing.end, duration)
				}
			}
		}
	}
	a.syncStatusLyricsCover(track, data)
	if runtime.GOOS == "windows" {
		s.native.update(enabled, statusLyricsFrame{
			statusLyricsLine: line, cover: s.cover, fontSize: taskbarLyricsFontSize(a.settings.TaskbarLyricsFontSize),
			font: a.settings.LyricsFontFamily, timing: timing, position: position, playing: playing,
		}, a.pl.Position)
	} else {
		if s.trayText != line.original {
			s.trayText = line.original
			s.trayPages = statusLyricsTrayPages(line.original)
		}
		title, pageAfter := statusLyricsTrayPageAt(s.trayPages, timing, position)
		a.syncStatusLyricsTray(title)
		if pageAfter > 0 && (after <= 0 || pageAfter < after) {
			after = pageAfter
		}
	}
	if after > 0 && playing {
		tick := s.tick
		s.timer = time.AfterFunc(max(after, 25*time.Millisecond), func() {
			mygo.RunOnMain(func() {
				if s.tick == tick {
					a.syncStatusLyrics()
				}
			})
		})
	}
}

func (a *app) syncStatusLyricsCover(track *pluginapi.Track, data []byte) {
	s := &a.statusLyrics
	if s.track != track {
		s.track, s.coverRequested, s.cover = track, false, nil
	}
	if track == nil || len(data) == 0 || s.coverRequested {
		return
	}
	s.coverRequested = true
	go func() {
		cover := makeStatusLyricsCover(data)
		if cover != nil {
			a.post(func() {
				if s.track == track && !a.quitting {
					s.cover = cover
					a.syncStatusLyrics()
				}
			})
		}
	}()
}

func makeStatusLyricsCover(data []byte) *statusLyricsCover {
	img, err := artwork.Decode(data)
	if err != nil {
		return nil
	}
	src := img.Bounds()
	side := min(src.Dx(), src.Dy())
	if side == 0 {
		return nil
	}
	origin := src.Min.Add(image.Pt((src.Dx()-side)/2, (src.Dy()-side)/2))
	crop := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(side, side))}
	thumb := image.NewRGBA(image.Rect(0, 0, 64, 64))
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), img, crop, draw.Src, nil)
	if runtime.GOOS == "windows" {
		return &statusLyricsCover{bitmap: ui.NewBitmap(thumb)}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, thumb); err != nil {
		return nil
	}
	return &statusLyricsCover{png: buf.Bytes()}
}

func (a *app) syncStatusLyricsTray(title string) {
	s := &a.statusLyrics
	if a.tray != nil {
		if title != s.title {
			a.tray.SetTitle(title)
		}
		if s.cover != s.trayCover {
			data, template := trayArtwork()
			if s.cover != nil {
				data, template = s.cover.png, false
			}
			if a.tray.SetIcon(data, template) == nil {
				s.trayCover = s.cover
			}
		}
	}
	s.title = title
}

// statusLyricAt returns the current text, its scroll interval, and the delay to the next lyric.
func statusLyricAt(lines []lyrics.Line, pos time.Duration) (statusLyricsLine, statusLyricsTiming, time.Duration) {
	current := lyrics.Index(lines, pos)
	var line statusLyricsLine
	var timing statusLyricsTiming
	if current >= 0 {
		timing = statusLyricsTiming{start: lines[current].At, end: lines[current].End}
		if timing.end <= timing.start {
			if current+1 < len(lines) {
				timing.end = lines[current+1].At
			} else {
				timing.end = timing.start + 6*time.Second
			}
		}
		line.original = cleanStatusLyric(lines[current].Text)
		line.translation = cleanStatusLyric(lines[current].Translation)
		// Bilingual LRC files can put both rows at the same timestamp.
		if line.translation == "" && current > 0 && lines[current-1].At == lines[current].At &&
			(current == 1 || lines[current-2].At != lines[current].At) {
			first := cleanStatusLyric(lines[current-1].Text)
			if first != "" && first != line.original {
				translation := cleanStatusLyric(lines[current-1].Translation)
				if translation == "" {
					translation = line.original
				}
				line = statusLyricsLine{original: first, translation: translation}
			}
		}
	}
	if next := current + 1; next < len(lines) {
		if current >= 0 {
			timing.end = min(timing.end, lines[next].At)
		}
		return line, timing, lines[next].At - pos
	}
	return line, timing, 0
}

func cleanStatusLyric(text string) string {
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
}
