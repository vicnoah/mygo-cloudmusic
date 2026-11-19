package songloft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/internal/plugintest"
)

// Activation reconnects with the existing private credential and omits it from saved preferences.
func TestLegacyCredentialReconnectsAtStartup(t *testing.T) {
	t.Setenv("MYGO_ENV", "development")
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	if err := os.WriteFile(filepath.Join(dir, string(ID)+"-credentials"), []byte(`{"password":"saved-songloft-password"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		legacySongloftServer(w, r)

	}))
	t.Cleanup(server.Close)
	h := plugintest.NewHost(ID)
	prefs := &platformPreferences{Config: connectionConfig{Address: server.URL, Username: "saved-user"}}
	type snapshot struct {
		data []byte
		err  error
	}
	saved := make(chan snapshot, 1)
	h.Save = func() {
		var data json.RawMessage
		err := h.Preferences.Load(&data)
		select {
		case saved <- snapshot{data, err}:
		default:
		}
	}
	if adapter := h.Start(Registration(), prefs); !adapter.Configured() {
		t.Fatal("did not load the saved account")
	}
	ctx := h.Activate(t)
	eventCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	h.Lifecycle.Ready(pluginapi.Activation{Context: eventCtx, ColdStart: true})
	var persistedSettings snapshot
	select {
	case persistedSettings = <-saved:
	case <-eventCtx.Done():
		t.Fatal("startup did not reconnect with the saved credential")
	}

	if persistedSettings.err != nil {
		t.Fatal(persistedSettings.err)
	}
	var persisted struct{ Config map[string]any }
	if err := json.Unmarshal(persistedSettings.data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Config["address"] != server.URL || persisted.Config["username"] != "saved-user" {
		t.Fatalf("persisted connection did not match the saved account: %s", persistedSettings.data)
	}
	for _, secret := range []string{"passHash", "accessCode", "password"} {
		if _, ok := persisted.Config[secret]; ok {
			t.Fatalf("%s was written into the public preferences", secret)
		}
	}
}

func legacySongloftServer(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/auth/login" {
		var account map[string]string
		if json.NewDecoder(r.Body).Decode(&account) != nil || account["username"] != "saved-user" || account["password"] != "saved-songloft-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `{"access_token":"songloft-session","expires_in":3600}`)
		return
	}
	io.WriteString(w, `{}`)
}
