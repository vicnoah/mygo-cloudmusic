package fnmusic

import backend "waygu-music/internal/plugins/fnmusic/internal/client"

type platformPreferences struct {
	Config           connectionConfig  `json:"config,omitempty"`
	Sorts            map[string]string `json:"sorts,omitempty"`
	TranscodeMode    transcodeMode     `json:"transcodeMode,omitempty"`
	TranscodeBitrate int               `json:"transcodeBitrate,omitempty"`
	LibrarySort      string            `json:"librarySort,omitempty"`
}

type connectionConfig struct {
	Address    string `json:"address,omitempty"`
	Username   string `json:"username,omitempty"`
	PassHash   string `json:"-"`
	AccessCode string `json:"-"`
	DeviceID   string `json:"deviceID,omitempty"`
	Mode       string `json:"mode,omitempty"`
	HTTPS      bool   `json:"https,omitempty"`
}

type transcodeMode = backend.TranscodeMode

const (
	transcodeAuto   = backend.TranscodeAuto
	transcodeNative = backend.TranscodeNative
	transcodeAlways = backend.TranscodeAlways
)

type transcodeSettings struct {
	Mode    transcodeMode
	Bitrate int
}
