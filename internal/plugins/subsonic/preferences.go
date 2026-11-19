package subsonic

type platformPreferences struct {
	Config           connectionConfig  `json:"config,omitempty"`
	Sorts            map[string]string `json:"sorts,omitempty"`
	TranscodeMode    transcodeMode     `json:"transcodeMode,omitempty"`
	TranscodeBitrate int               `json:"transcodeBitrate,omitempty"`
}

type connectionConfig struct {
	Address  string `json:"address,omitempty"`
	Username string `json:"username,omitempty"`
	PassHash string `json:"-"`
	HTTPS    bool   `json:"https,omitempty"`
}

type transcodeMode string

const (
	transcodeAuto   transcodeMode = "auto"
	transcodeNative transcodeMode = "native"
	transcodeAlways transcodeMode = "always"
)

type transcodeSettings struct {
	Mode    transcodeMode
	Bitrate int
}
