package subsonic

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/subsonic"
)

func secretKey(id platform.ID) string {
	if id == "" {
		return "subsonic-credentials"
	}
	return string(id) + "-credentials"
}

type subsonicCredentials struct {
	Password string `json:"password"`
}

func credentialFile(id platform.ID) (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, secretKey(id)), nil
}

// LoadCredentials loads the stored password from secure storage or local dev file.
func LoadCredentials(id platform.ID, cfg *backend.Config) {
	if cfg == nil {
		return
	}
	var b []byte
	var err error
	if mygo.IsDev() {
		var path string
		if path, err = credentialFile(id); err == nil {
			b, err = os.ReadFile(path)
		}
	} else {
		b, err = mygo.Secrets.Get(secretKey(id))
	}
	var sec subsonicCredentials
	if err != nil || json.Unmarshal(b, &sec) != nil {
		return
	}
	cfg.Password = sec.Password
}

// SaveCredentials persists the password into secure storage or local dev file.
func SaveCredentials(id platform.ID, cfg backend.Config) error {
	if cfg == (backend.Config{}) {
		return nil
	}
	b, _ := json.Marshal(subsonicCredentials{Password: cfg.Password})
	if !mygo.IsDev() {
		return mygo.Secrets.Set(secretKey(id), b)
	}
	path, err := credentialFile(id)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

// ClearCredentials removes persisted credentials on logout/disconnect.
func ClearCredentials(id platform.ID) {
	if mygo.IsDev() {
		if path, err := credentialFile(id); err == nil {
			_ = os.Remove(path)
		}
		return
	}
	_ = mygo.Secrets.Delete(secretKey(id))
}
