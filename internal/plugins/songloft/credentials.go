package songloft

import (
	"encoding/json"

	"github.com/egoist/mygo"

	"waygu-music/internal/pluginapi/storage"
	backend "waygu-music/internal/plugins/songloft/internal/client"
)

var credentials = storage.For(ID).LegacyCredential("credentials", "")

type songloftCredentials struct {
	Password string `json:"password"`
}

func LoadCredentials(cfg *backend.Config) {
	if cfg == nil {
		return
	}
	var b []byte
	if !mygo.IsDev() {
		b, _ = credentials.Read()
	}
	if len(b) == 0 {
		b, _ = credentials.ReadFile()
	}
	var sec songloftCredentials
	if len(b) == 0 || json.Unmarshal(b, &sec) != nil {
		return
	}
	cfg.Password = sec.Password
}

func SaveCredentials(cfg backend.Config) error {
	if cfg == (backend.Config{}) {
		return nil
	}
	b, _ := json.Marshal(songloftCredentials{Password: cfg.Password})
	if !mygo.IsDev() {
		_ = credentials.Write(b)
	}
	_ = credentials.WriteFile(b)
	return nil
}

func ClearCredentials() error {
	if !mygo.IsDev() {
		_ = credentials.Delete()
	}
	_ = credentials.DeleteFile()
	return nil
}
