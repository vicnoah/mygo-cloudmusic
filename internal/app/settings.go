package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// multichannelLayout is the speaker whitelist MPV tries from 7.1.4 down to
// 5.1 and stereo, matching the channel order of the AV3A helper's WAV.
const multichannelLayout = "fl-fr-fc-lfe-bl-br-sl-sr-tfl-tfr-tbl-tbr,5.1(side),stereo"

// desktopLyricsSettings are the desktop lyric window's persistent choices.
// A pointer in settings keeps older settings files compatible: missing
// desktop-lyrics data gets the current defaults on first use.
type desktopLyricsSettings struct {
	Locked          bool    `json:"locked,omitempty"`
	ShowTranslation bool    `json:"showTranslation"`
	FontSize        float32 `json:"fontSize,omitempty"`
	Color           int     `json:"color,omitempty"`
	X               int     `json:"x,omitempty"`
	Y               int     `json:"y,omitempty"`
	PositionSet     bool    `json:"positionSet,omitempty"`
}

func defaultDesktopLyricsSettings() desktopLyricsSettings {
	return desktopLyricsSettings{ShowTranslation: true, FontSize: 30}
}

// settings are the listener's choices, kept in settings.json in the user
// data directory.
type settings struct {
	platformSettings
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
	// DesktopLyricsLocked remembers whether desktop lyrics should start
	// click-through when shown.
	DesktopLyricsLocked bool `json:"desktopLyricsLocked,omitempty"`
	// DesktopLyrics keeps the desktop lyric window's style and position.
	DesktopLyrics *desktopLyricsSettings `json:"desktopLyrics,omitempty"`
	// CacheMB is how many megabytes the songs kept on disk may take; 0
	// keeps none.
	CacheMB int `json:"cacheMB"`
	// SearchHistory keeps submitted keywords, most recent first.
	SearchHistory []string `json:"searchHistory,omitempty"`

	// UseMPV enables high-quality playback via libmpv when available.
	UseMPV bool `json:"useMPV,omitempty"`
	// MPVExclusive enables audio device exclusive mode in libmpv.
	MPVExclusive bool `json:"mpvExclusive,omitempty"`
	// MPVReplayGain configures replaygain normalization mode ("no", "album", "track").
	MPVReplayGain string `json:"mpvReplayGain,omitempty"`
	// MPVGapless enables gapless audio playback in libmpv.
	MPVGapless bool `json:"mpvGapless,omitempty"`
	// MPVAudioDevice specifies the selected output device name ("" or "auto" for system default).
	MPVAudioDevice string `json:"mpvAudioDevice,omitempty"`
	// MPVAudioChannels is MPV's output channel layout: "auto-safe",
	// "stereo", or an explicit speaker whitelist for multichannel output.
	// It is kept for settings written before SpatialMode existed.
	MPVAudioChannels string `json:"mpvAudioChannels,omitempty"`
	// MPVAudioSpdif attempts E-AC-3 passthrough when a specific HDMI/SPDIF
	// receiver is selected. Built-in speakers and "auto" stay on MPV's
	// normal decoder; the engine also falls back to PCM if a receiver
	// rejects compressed passthrough.
	MPVAudioSpdif bool `json:"mpvAudioSpdif,omitempty"`
	// SpatialMode is how multichannel streams are rendered: "auto",
	// "speakers", "headphones" or "stereo".
	SpatialMode string `json:"spatialMode,omitempty"`
	// HrtfProfile selects the headphone crossfeed width: "balanced" or
	// "wide". MPV builds without a SOFA/HRTF filter use crossfeed.
	HrtfProfile string `json:"hrtfProfile,omitempty"`

	// EqualizerEnabled toggles graphic EQ processing in MPV.
	EqualizerEnabled bool `json:"equalizerEnabled,omitempty"`
	// EqualizerPreset is the active preset ID ("flat", "vocal", "bass", etc.).
	EqualizerPreset string `json:"equalizerPreset,omitempty"`
	// EqualizerPreamp is the preamp gain in dB (-12 to +12).
	EqualizerPreamp float64 `json:"equalizerPreamp,omitempty"`
	// EqualizerGains stores the 10-band gains in dB.
	EqualizerGains [10]float64 `json:"equalizerGains,omitempty"`

	path string // where they were loaded from, and are saved to
}

func defaultSettings() settings {
	return settings{NotchActivity: true, Quality: "exhigh", Appearance: "system", Volume: 0.8, Normalize: true, AppIcon: "dock-and-menubar", CacheMB: 2048, Captions: true, EqualizerPreset: "flat", MPVAudioChannels: "auto-safe", SpatialMode: "auto", HrtfProfile: "balanced"}
}

// spatialMode returns the configured spatial mode, migrating the old
// channel setting when SpatialMode has not been written yet.
func (s settings) spatialMode() string {
	if s.SpatialMode != "" {
		return s.SpatialMode
	}
	switch s.MPVAudioChannels {
	case "stereo":
		return "stereo"
	case multichannelLayout:
		return "speakers"
	default:
		return "auto"
	}
}

func (s settings) hrtfProfile() string {
	if s.HrtfProfile == "wide" {
		return "wide"
	}
	return "balanced"
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
	return s
}

// save keeps the settings where they were loaded from; defaults that were
// never loaded, as in tests, are not saved.
func (s settings) save() error {
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
	{"jyeffect", "高清臻音（96kHz/24bit）"},
	{"sky", "沉浸环绕声（5.1）"},
	{"jymaster", "超清母带（192kHz/24bit）"},
	{"dolby", "杜比全景声（E-AC-3）"},
	{"vivid", "臻音全景声（Audio Vivid 7.1.4）"},
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
