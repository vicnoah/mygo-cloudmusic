package songloft

import (
	"waygu-music/internal/pluginapi"
)

// Profile configures branding and hints for Songloft.
type Profile struct {
	ID          pluginapi.ID
	Name        string
	DefaultHint string
	Description string
	DefaultPort string
}

var DefaultProfile = Profile{
	ID:          PlatformID,
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
