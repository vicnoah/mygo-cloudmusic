package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/fnmusic"
)

// fnmusicSettings is persisted separately from the upstream NetEase
// settings. The active platform selects which settings page and provider are
// used; NetEase's existing top-level fields stay unchanged for compatibility
// with the upstream app.
type fnmusicSettings struct {
	Config           fnmusic.Config        `json:"config,omitempty"`
	TranscodeMode    fnmusic.TranscodeMode `json:"transcodeMode,omitempty"`
	TranscodeBitrate int                   `json:"transcodeBitrate,omitempty"`
	LibrarySort      string                `json:"librarySort,omitempty"`
}

// settings are the listener's choices, kept in settings.json in the user
// data directory.
type settings struct {
	// Notch shows the quick player around the notch: the listener turns
	// it on.
	Notch bool `json:"notch"`
	// NotchActivity shows the cover and the music's levels beside the
	// notch while music plays, which hides the menu bar there.
	NotchActivity bool `json:"notchActivity"`
	// NotchVirtual keeps a notch of the app's own showing at the top of a
	// display without one; else it shows only while the pointer asks.
	NotchVirtual bool `json:"notchVirtual"`
	// NotchGreeted says the notch showed itself, once, as the first song
	// played (greet).
	NotchGreeted bool `json:"notchGreeted"`
	// Quality is the level of the streams: standard, higher, exhigh,
	// lossless or hires.
	Quality string `json:"quality"`
	// FnMusic keeps Feiniu-only settings in its own platform namespace.
	FnMusic fnmusicSettings `json:"fnmusic,omitempty"`
	// Appearance is light, dark, or system to follow the system.
	Appearance string `json:"appearance"`
	// Smoke makes the progress bars comets trailing smoke as music plays,
	// for a few percent of a core: the listener turns it on.
	Smoke bool `json:"smoke"`
	// LyricsStage shows lyrics on the stage, Folia's style, in Now
	// Playing, rather than scrolling.
	LyricsStage bool `json:"lyricsStage"`
	// StageStyle is how the stage sets its lyrics (stageStyles): empty for
	// Folia's classic, or the id of another of its visualizers.
	StageStyle string `json:"stageStyle,omitempty"`
	// StageSparks throws sparks off each word on the stage as it is sung,
	// whatever its style.
	StageSparks bool `json:"stageSparks,omitempty"`
	// Volume is where the volume was left, from 0 to 1.
	Volume float64 `json:"volume"`
	// Normalize evens out the loudness of tracks, by what NetEase
	// measured of each.
	Normalize bool `json:"normalize"`
	// HotComments shows a song's hot comments in its quiet parts, in
	// place of the lyrics (quietComment): the listener turns it on.
	HotComments bool `json:"hotComments"`
	// Fog fills Now Playing's backdrop with fog of the cover's color,
	// rising and swelling with the bass: an experiment the listener turns
	// on.
	Fog bool `json:"fog"`
	// AppIcon is where the app's icon shows on macOS (appIcons); elsewhere
	// it is always in the notification area.
	AppIcon string `json:"appIcon"`
	// Captions writes down episodes as they play, for their captions,
	// once the listener has downloaded the model.
	Captions bool `json:"captions"`
	// CacheMB is how many megabytes the songs kept on disk may take; 0
	// keeps none.
	CacheMB int `json:"cacheMB"`
	// SearchHistory keeps submitted keywords, most recent first.
	SearchHistory []string `json:"searchHistory,omitempty"`
	// Source is the active platform: sourceNetease or sourceFn. It is persisted
	// as platform while retaining the existing runtime name for now.
	Source string `json:"platform"`
	// These aliases keep the current player and connection code small while
	// the platform-specific settings page is being split out. They are never
	// serialized; save/load synchronize them with FnMusic.
	Fn                 fnmusic.Config        `json:"-"`
	FnTranscodeMode    fnmusic.TranscodeMode `json:"-"`
	FnTranscodeBitrate int                   `json:"-"`

	path string // where they were loaded from, and are saved to
}

func defaultSettings() settings {
	s := settings{Source: sourceNetease, NotchActivity: true, Quality: "exhigh", Appearance: "system", Volume: 0.8, Normalize: true, AppIcon: "dock-and-menubar", CacheMB: 2048, Captions: true}
	s.FnMusic.TranscodeMode = fnmusic.TranscodeAuto
	s.syncFnMusic()
	return s
}

func (s *settings) syncFnMusic() {
	s.Fn = s.FnMusic.Config
	s.FnTranscodeMode = s.FnMusic.TranscodeMode
	s.FnTranscodeBitrate = s.FnMusic.TranscodeBitrate
}

func (s *settings) syncFnMusicAliases() {
	s.FnMusic.Config = s.Fn
	s.FnMusic.TranscodeMode = s.FnTranscodeMode
	s.FnMusic.TranscodeBitrate = s.FnTranscodeBitrate
}

func settingsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// oldName is what the app was called before it took its bundle's name, and
// kept its data under.
const oldName = "网易云音乐"

// moveOldData moves the user data and the song cache kept under oldName to
// the directories of the app's name, unless those hold something already:
// the login, settings and where playback was left go on. It must run before
// anything reads them.
func moveOldData() {
	for _, name := range []mygo.PathName{mygo.PathUserData, mygo.PathCache} {
		dir, err := mygo.App.Path(name)
		if err != nil {
			continue
		}
		moveDir(filepath.Join(filepath.Dir(dir), oldName), dir)
	}
}

// moveDir moves the directory old to dir when dir is empty, as Path made
// it, and old exists.
func moveDir(old, dir string) {
	if old == dir {
		return
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) > 0 {
		return
	}
	if _, err := os.Stat(old); err != nil {
		return
	}
	if os.Remove(dir) == nil {
		if err := os.Rename(old, dir); err != nil {
			os.MkdirAll(dir, 0o700)
		}
	}
}

// loadSettings reads the settings, or returns the defaults.
func loadSettings() settings {
	s := defaultSettings()
	if path, err := settingsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &s)
		}
		s.path = path
	}
	if s.Source == "" {
		s.Source = sourceNetease
	}
	if s.FnMusic.TranscodeMode != fnmusic.TranscodeAuto && s.FnMusic.TranscodeMode != fnmusic.TranscodeNative && s.FnMusic.TranscodeMode != fnmusic.TranscodeAlways {
		s.FnMusic.TranscodeMode = fnmusic.TranscodeAuto
	}
	if !fnmusic.ValidTranscodeBitrate(s.FnMusic.TranscodeBitrate) {
		s.FnMusic.TranscodeBitrate = 0
	}
	s.syncFnMusic()
	return s
}

// save keeps the settings where they were loaded from; defaults that were
// never loaded, as in tests, are not saved.
func (s settings) save() error {
	s.syncFnMusicAliases()
	path := s.path
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// applyAppearance makes the windows light, dark, or follow the system.
func (s settings) applyAppearance() {
	switch s.Appearance {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// qualityLevel is the stream level NetEase names a quality by.
var qualityLevels = []struct{ level, label string }{
	{"standard", "标准（128k）"},
	{"higher", "较高（192k）"},
	{"exhigh", "极高（320k）"},
	{"lossless", "无损（FLAC）"},
	{"hires", "Hi-Res"},
}

var fnTranscodeModes = []struct {
	mode  fnmusic.TranscodeMode
	label string
}{
	{fnmusic.TranscodeAuto, "自动转码"},
	{fnmusic.TranscodeNative, "仅播放原始格式"},
	{fnmusic.TranscodeAlways, "始终转码"},
}

func fnTranscodeModeLabel(mode fnmusic.TranscodeMode) string {
	for _, item := range fnTranscodeModes {
		if item.mode == mode {
			return item.label
		}
	}
	return fnTranscodeModes[0].label
}

func fnTranscodeBitrateLabel(bitrate int) string {
	if bitrate == 0 {
		return "自动（飞牛默认）"
	}
	return fmt.Sprintf("%d kbps", bitrate)
}

// cacheLimits are the sizes the song cache may take, in megabytes.
var cacheLimits = []struct {
	mb    int
	label string
}{
	{0, "关闭"},
	{1024, "1 GB"},
	{2048, "2 GB"},
	{5120, "5 GB"},
	{10240, "10 GB"},
}
