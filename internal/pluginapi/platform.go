package pluginapi

import (
	"context"

	"github.com/egoist/mygo/ui"
)

// Adapter describes the status and media that the host routes between plugins and services.
type Adapter interface {
	Configured() bool
	// FetchArtwork reports whether this plugin recognized the resource reference.
	FetchArtwork(ctx context.Context, ref string, size int) (data []byte, handled bool, err error)
	PlaybackMedia() PlaybackMedia
	OwnsTrack(*Track) bool
}

// Registration supplies the entry point for a platform instance.
type Registration struct {
	Info
	Start         func(Startup) Adapter
	DeveloperOnly bool
	// LegacySettingsKey keeps a former top-level namespace in the same place.
	LegacySettingsKey string
	// LegacySettingsFields keeps selected fields at the root of the old settings file.
	LegacySettingsFields []string
	// FallbackOnInvalidSwitch preserves the bundle's choice for unknown IDs.
	FallbackOnInvalidSwitch bool
}

// Info is a platform's identity as presented by plugin-owned selectors.
type Info struct {
	ID   ID
	Name string
	// Icon is supplied by the plugin for other plugins' platform selectors.
	Icon *ui.SVG
}

type PlayerPort interface {
	Current() *Track
	Playing() bool
	Loading() bool
	Err() string
	TogglePlay()
	Next()
	Queue() []Track
	PlayOrder() ([]int, int)
	PlayQueue([]Track, int)
	AppendQueue([]Track)
	PlayList([]Track, int, func(func([]Track)))
	PlayRadio(string, []Track, func(func([]Track)))
	EndRadio()
	Radio() string
	SetShuffle(bool)
}

// Services provides desktop and media operations to window-owning plugins.
type Services interface {
	DesktopServices
	DesktopPlaybackServices
	Post(func())
	Invalidate()
	CurrentPlatform() ID
	ShowMain()
	OpenSettings()
	CloseSettings()
	Toast(string)
	TakeToast() string
	Player() PlayerPort
	PlatformOptions() []Info
	Artwork(url string, size int) (*ui.Bitmap, bool)
	PlaybackLevels() ([SpectrumBands]float32, bool)
	CopyText(string)
	GlobalSearchHistory() []string
	RememberGlobalSearch(string)
	ClearSearchHistory()
	SwitchPlatform(ID)
	PlatformConfigured(ID) bool
}
