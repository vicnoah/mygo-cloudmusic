package fnmusic

import (
	"context"
	"encoding/base64"
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
	if err := os.WriteFile(filepath.Join(dir, string(ID)+"-credentials"), []byte(`{"passHash":"saved-fn-hash","accessCode":"saved-nas-code"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	playlists := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		legacyFeiniuServer(w, r)
		if r.URL.Path == "/music/api/v1/playlist/list" {
			select {
			case playlists <- struct{}{}:
			default:
			}
		}
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
	select {
	case <-playlists:
	case <-eventCtx.Done():
		t.Fatal("startup did not request the saved account's playlists")
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

// A delayed logout must not delete the credentials saved by a new connection.
func TestReconnectWhilePreviousLogoutIsPending(t *testing.T) {
	t.Setenv("MYGO_ENV", "development")
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	logoutStarted := make(chan struct{}, 1)
	finishLogout := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/music/api/v1/user/password-login":
			var account struct{ Username string }
			if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"userToken": account.Username}})
		case "/music/api/v1/user/logout":
			select {
			case logoutStarted <- struct{}{}:
			default:
			}
			select {
			case <-finishLogout:
			case <-ctx.Done():
			}
			w.WriteHeader(http.StatusOK)
		default:
			_, _ = io.WriteString(w, `{"code":0,"data":{}}`)
		}
	}))
	defer server.Close()
	defer cancel()

	h := plugintest.NewHost(ID)
	a := h.Start(Registration(), nil).(*Adapter)
	first, leave := context.WithCancel(ctx)
	defer leave()
	h.Lifecycle.Activate(first, nil)
	old := connectionConfig{Address: server.URL, Username: "old-account", PassHash: "old-secret"}
	if err := a.runtime.Connect(first, old); err != nil {
		t.Fatal(err)
	}
	logoutDone := make(chan struct{})
	go func() {
		_ = a.runtime.Disconnect(context.Background())
		close(logoutDone)
	}()
	select {
	case <-logoutStarted:
	case <-ctx.Done():
		t.Fatal("the old account did not reach the logout endpoint")
	}

	leave()
	h.Lifecycle.Deactivate(true)
	h.Lifecycle.Activate(ctx, nil)
	next := connectionConfig{Address: server.URL, Username: "new-account", PassHash: "new-secret"}
	if err := a.runtime.Connect(ctx, next); err != nil {
		t.Fatal(err)
	}
	close(finishLogout)
	select {
	case <-logoutDone:
	case <-ctx.Done():
		t.Fatal("the old logout did not finish")
	}

	var persisted platformPreferences
	if err := h.Preferences.Load(&persisted); err != nil {
		t.Fatal(err)
	}
	restarted := plugintest.NewHost(ID).Start(Registration(), &persisted).(*Adapter)
	if cfg := restarted.runtime.Config(); !restarted.Configured() || cfg.Username != next.Username || cfg.PassHash != next.PassHash {
		t.Fatal("completing the old logout erased the new connection's saved account")
	}
	if !a.runtime.Connected() || a.runtime.Config().Username != next.Username {
		t.Fatal("the new connection did not survive the old logout")
	}
}

func legacyFeiniuServer(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-access-code") != base64.StdEncoding.EncodeToString([]byte("saved-nas-code")) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/access_code_verify":
		w.WriteHeader(http.StatusOK)
	case "/music/api/v1/sys/config":
		io.WriteString(w, `{"code":0,"data":{}}`)
	case "/music/api/v1/user/password-login":
		var account map[string]string
		if json.NewDecoder(r.Body).Decode(&account) != nil || account["username"] != "saved-user" || account["password"] != "saved-fn-hash" {
			io.WriteString(w, `{"code":120003}`)
			return
		}
		io.WriteString(w, `{"code":0,"data":{"userToken":"fn-session"}}`)
	default:
		io.WriteString(w, `{"code":0,"data":{}}`)
	}
}
