package songloft

import (
	"netease-music-mygo/internal/platform"
)

// Profile configures branding and hints for Songloft.
type Profile struct {
	ID          platform.ID
	Name        string
	DefaultHint string
	Description string
	DefaultPort string
}

var DefaultProfile = Profile{
	ID:          platform.SongloftID,
	Name:        "Songloft",
	DefaultHint: "例如 http://192.168.1.10:58091",
	Description: "现代化自建音乐服务",
	DefaultPort: "58091",
}

// Config represents connection parameters for a Songloft instance.
type Config struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"-"`
	DeviceID string `json:"deviceID,omitempty"`
}
