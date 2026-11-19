package subsonic

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
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

type restoreAccountHost struct{ Host }

func (restoreAccountHost) Presentation() pluginapi.Presentation { return pluginapi.Presentation{} }
func (restoreAccountHost) Post(func())                          {}

// Each Subsonic entry point reconnects at startup using its own pre-plugin credential file.
func TestSubsonicRestoresLegacyProfileCredentials(t *testing.T) {
	t.Setenv("MYGO_ENV", "development")
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	registrations := Platforms()
	passwords := make(map[string]string, len(registrations))
	for _, entry := range registrations {
		username := string(entry.ID)
		password := "fixture-for-" + username
		passwords[username] = password
		data, err := json.Marshal(map[string]string{"password": password})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, username+"-credentials"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()
		password, found := passwords[query.Get("u")]
		salt := query.Get("s")
		token := fmt.Sprintf("%x", md5.Sum([]byte(password+salt)))
		if !found || salt == "" || query.Get("t") != token {
			io.WriteString(w, `<subsonic-response status="failed" version="1.16.1"><error code="40" message="Invalid credentials"/></subsonic-response>`)
			return
		}
		io.WriteString(w, `<subsonic-response status="ok" version="1.16.1"><scanStatus count="1"/><albumList2/><playlists/><starred2/></subsonic-response>`)
	}))
	defer server.Close()

	for _, registration := range registrations {
		t.Run(string(registration.ID), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			connected := make(chan struct{}, 1)
			var lifecycle pluginapi.Lifecycle
			store := plugintest.NewSettings(platformPreferences{
				Config:        connectionConfig{Address: server.URL, Username: string(registration.ID)},
				TranscodeMode: transcodeAlways, TranscodeBitrate: 192,
			}, func() { connected <- struct{}{} })
			adapter := registration.Start(pluginapi.Startup{
				Host: restoreAccountHost{}, Settings: store,
				RegisterWindow:    func(pluginapi.WindowID, pluginapi.Window) {},
				RegisterMenu:      func(pluginapi.MenuID, func() *mygo.Menu) {},
				RegisterLifecycle: func(hooks pluginapi.Lifecycle) { lifecycle = hooks },
			}).(*Adapter)
			if !adapter.Configured() {
				t.Fatal("entry point did not restore its saved account")
			}
			lifecycle.Activate(ctx, nil)
			lifecycle.Ready(pluginapi.Activation{Context: ctx, ColdStart: true})
			select {
			case <-connected:
				if !adapter.state.Connected() {
					t.Fatal("entry point persisted an account without establishing a connection")
				}
			case <-ctx.Done():
				t.Fatalf("entry point did not reconnect to its saved account: %v", adapter.state.Error())
			}
			if client := adapter.state.Client(); client == nil || client.TranscodeMode() != "always" || client.MaxBitRate() != 192 {
				t.Fatal("reconnecting did not apply the saved transcode policy to the client")
			}
			adapter.runtime.SetTranscode(transcodeSettings{Mode: transcodeNative, Bitrate: 128})
			var saved platformPreferences
			if err := store.Load(&saved); err != nil {
				t.Fatal(err)
			}
			if saved.Config.Username != string(registration.ID) || saved.TranscodeMode != transcodeNative || saved.TranscodeBitrate != 128 {
				t.Fatalf("changing policy did not save within the profile: %+v", saved)
			}
		})
	}
}
