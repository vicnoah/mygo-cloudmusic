package songloft

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	backend "netease-music-mygo/internal/platform/songloft"
)

const secretKeyName = "songloft-credentials"

type songloftCredentials struct {
	Password string `json:"password"`
}

func credentialFile() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, secretKeyName), nil
}

// LoadCredentials loads the stored password from secure storage or local dev file.
func LoadCredentials(cfg *backend.Config) {
	if cfg == nil {
		return
	}
	var b []byte
	if !mygo.IsDev() {
		b, _ = mygo.Secrets.Get(secretKeyName)
	}
	if len(b) == 0 {
		if path, perr := credentialFile(); perr == nil {
			b, _ = os.ReadFile(path)
		}
	}
	var sec songloftCredentials
	if len(b) == 0 || json.Unmarshal(b, &sec) != nil {
		return
	}
	cfg.Password = sec.Password
}

// SaveCredentials persists the password into secure storage or local dev file.
func SaveCredentials(cfg backend.Config) error {
	if cfg == (backend.Config{}) {
		return nil
	}
	b, _ := json.Marshal(songloftCredentials{Password: cfg.Password})
	if !mygo.IsDev() {
		_ = mygo.Secrets.Set(secretKeyName, b)
	}
	if path, err := credentialFile(); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0700)
		_ = os.WriteFile(path, b, 0600)
	}
	return nil
}

// ClearCredentials removes persisted credentials on logout/disconnect.
func ClearCredentials() error {
	if !mygo.IsDev() {
		_ = mygo.Secrets.Delete(secretKeyName)
	}
	if path, err := credentialFile(); err == nil {
		_ = os.Remove(path)
	}
	return nil
}
