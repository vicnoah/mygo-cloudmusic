package subsonic

import (
	"encoding/json"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/storage"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
)

func credentials(id pluginapi.ID) storage.Credential {
	if id == "" {
		id = ID
	}
	return storage.For(id).LegacyCredential("credentials", "")
}

type subsonicCredentials struct {
	Password string `json:"password"`
}

func LoadCredentials(id pluginapi.ID, cfg *backend.Config) {
	if cfg == nil {
		return
	}
	b, err := credentials(id).Read()
	var sec subsonicCredentials
	if err != nil || json.Unmarshal(b, &sec) != nil {
		return
	}
	cfg.Password = sec.Password
}

func SaveCredentials(id pluginapi.ID, cfg backend.Config) error {
	if cfg == (backend.Config{}) {
		return nil
	}
	b, _ := json.Marshal(subsonicCredentials{Password: cfg.Password})
	return credentials(id).Write(b)
}

func ClearCredentials(id pluginapi.ID) { _ = credentials(id).Delete() }
