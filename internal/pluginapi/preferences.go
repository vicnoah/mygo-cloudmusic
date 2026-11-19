package pluginapi

import "time"

// DesktopPreferences are application-wide settings used by the active plugin.
type DesktopPreferences struct {
	Appearance    string
	AppIcon       string
	CacheMB       int
	Notch         bool
	NotchActivity bool
	NotchVirtual  bool
	Smoke         bool
	Fog           bool

	UseMPV         bool
	MPVExclusive   bool
	MPVGapless     bool
	MPVAudioDevice string
}

type DesktopSettings struct {
	Preferences            DesktopPreferences
	CacheUsed              int64
	NotchAvailable         bool
	Updates                bool
	UpdateChecks           bool
	UpdateDownloads        bool
	LastUpdateCheck        time.Time
	DeveloperMode          bool
	VersionClicksRemaining int
}

type AudioDevice struct {
	Name        string
	Description string
}

type DecoderStatus struct {
	Ready       bool
	Downloaded  bool
	Downloading bool
	SystemPath  string
	Done, Total int64
	Devices     []AudioDevice
}

// DesktopServices reads and applies settings without drawing any controls.
type DesktopServices interface {
	DesktopSettings() DesktopSettings
	ApplyDesktopPreferences(DesktopPreferences)
	ClearAudioCache()
	DecoderStatus() DecoderStatus
	StartDecoderDownload()
	CancelDecoderDownload()
	DeleteDecoder()
	SetUpdateChecks(bool)
	SetUpdateDownloads(bool)
	CheckForUpdates()
	RecordVersionClick()
}
