package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/desktop"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/eq"
	"github.com/waygu-music-plugin/sdk/pluginapi/storage"
)

type desktopLyricsSettings = pluginapi.DesktopLyricsPreferences

// settings are the listener's choices, kept in settings.json in the user
// data directory.
type settings struct {
	platformSettings
	DesktopLyricsColor    string `json:"desktopLyricsColor,omitempty"`
	StatusLyrics          bool   `json:"statusLyrics,omitempty"`
	TaskbarLyricsFontSize int    `json:"taskbarLyricsFontSize,omitempty"`
	LyricsFontFamily      string `json:"lyricsFontFamily,omitempty"`
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
	// Fog fills Now Playing's backdrop with fog of the cover's color,
	// rising and swelling with the bass: an experiment the listener turns
	// on.
	Fog bool `json:"fog"`
	// AppIcon is where the app's icon shows on macOS (appIcons); elsewhere
	// it is always in the notification area.
	AppIcon string `json:"appIcon"`
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

	// EQ is the graphic equalizer, under a key of its own: settings files
	// from before the 15-band equalizer keep their old equalizer under
	// other keys, which are ignored, so it starts again from default.
	EQ eq.Settings `json:"eq"`

	path string // where they were loaded from, and are saved to
}

func defaultSettings() settings {
	return settings{
		NotchActivity: true,
		Quality:       "exhigh",
		Appearance:    "system",
		Volume:        0.8,
		Normalize:     true,
		AppIcon:       "dock-and-menubar",
		CacheMB:       2048,
		EQ:            eq.Default(),
	}
}

func settingsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// oldNames are what the app was called before it took its bundle's name,
// and kept its data under, newest first. It was "网易云音乐" before it was
// "MyGo 云音乐"; the first builds under the new name wrote "肥牛音乐" on
// Windows and macOS, and "wagyu-music" on Linux, before the folders were
// pinned to "Wagyu Music". A listener who came from any of them keeps their
// login, settings and where playback was left.
var oldNames = []string{"肥牛音乐", "wagyu-music", "MyGo 云音乐", "网易云音乐"}

// moveOldData moves the user data and the song cache kept under the old
// names to the directories of the app's name, unless those hold something
// already: the login, settings and where playback was left go on. It must
// run before anything reads them.
func moveOldData() {
	for _, name := range []mygo.PathName{mygo.PathUserData, mygo.PathCache} {
		dir, err := mygo.App.Path(name)
		if err != nil {
			continue
		}
		for _, old := range oldNames {
			moveDir(filepath.Join(filepath.Dir(dir), old), dir)
		}
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
func loadSettings(registrations ...pluginapi.Registration) settings {
	s := defaultSettings()
	if path, err := settingsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil {
			file := struct {
				*settings
				LegacyFont string  `json:"taskbarLyricsFontFamily"`
				SharedFont *string `json:"lyricsFontFamily"`
			}{settings: &s}
			json.Unmarshal(b, &file)
			s.LyricsFontFamily = file.LegacyFont
			if file.SharedFont != nil {
				s.LyricsFontFamily = *file.SharedFont
			}
			s.loadLegacyNamespaces(b, registrations)
		}
		s.path = path
	}
	s.rememberLegacySubsonicSession()
	s.EQ.Normalize()
	return s
}

// save keeps the settings where they were loaded from; defaults that were
// never loaded, as in tests, are not saved.
func (s settings) save() error {
	path := s.path
	if path == "" {
		return nil
	}
	b, err := s.marshalSettings()
	if err != nil {
		return err
	}
	return storage.WriteDataFile(path, b)
}

func (s settings) marshalSettings() ([]byte, error) {
	if len(s.legacyNamespaces) == 0 {
		return json.MarshalIndent(s, "", "  ")
	}
	body, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	for key, raw := range s.legacyNamespaces {
		fields[key] = raw
	}
	return json.MarshalIndent(fields, "", "  ")
}

func (s settings) applyAppearance() { desktop.ApplyAppearance(s.Appearance) }
