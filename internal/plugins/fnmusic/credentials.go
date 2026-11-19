package fnmusic

import (
	"encoding/json"

	"waygu-music/internal/pluginapi/storage"
	backend "waygu-music/internal/plugins/fnmusic/internal/client"
)

var credentials = storage.For(ID).LegacyCredential("credentials", "")

type fnCredentials struct {
	PassHash   string `json:"passHash"`
	AccessCode string `json:"accessCode,omitempty"`
}

func LoadCredentials(cfg *backend.Config) {
	if cfg == nil {
		return
	}
	b, err := credentials.Read()
	var sec fnCredentials
	if err != nil || json.Unmarshal(b, &sec) != nil {
		return
	}
	cfg.PassHash, cfg.AccessCode = sec.PassHash, sec.AccessCode
}

func SaveCredentials(cfg backend.Config) error {
	if cfg == (backend.Config{}) {
		return nil
	}
	b, _ := json.Marshal(fnCredentials{PassHash: cfg.PassHash, AccessCode: cfg.AccessCode})
	return credentials.Write(b)
}

func DeleteCredentials() { _ = credentials.Delete() }
