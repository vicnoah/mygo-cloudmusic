package netease

import (
	"errors"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

// A NetEase login from an older build restores its cookies and sign-out deletes the old record.
func TestNetEaseRestoresLegacySessionCookies(t *testing.T) {
	t.Setenv("MYGO_ENV", "development")
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	path := filepath.Join(dir, "session")
	if err := os.WriteFile(path, []byte("MUSIC_U=music-session; __csrf=csrf-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldJar := httpClient.Jar
	defer func() { httpClient.Jar = oldJar }()
	httpClient.Jar, _ = cookiejar.New(nil)
	if !SessionConfigured() || !RestoreSession() {
		t.Fatal("legacy NetEase session was not restored")
	}
	cookies := make(map[string]string)
	for _, cookie := range httpClient.Jar.Cookies(siteURL) {
		cookies[cookie.Name] = cookie.Value
	}
	if cookies["MUSIC_U"] != "music-session" || cookies["__csrf"] != "csrf-token" {
		t.Fatal("restored jar did not contain both login cookies")
	}
	ClearSession()
	if SessionConfigured() || len(httpClient.Jar.Cookies(siteURL)) != 0 {
		t.Fatal("sign-out retained the NetEase session")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy file after sign-out: %v", err)
	}
}
