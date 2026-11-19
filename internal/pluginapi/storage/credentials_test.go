package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

// A plugin can replace and remove its development credential without changing another instance's record.
func TestInstanceCredentialIsolation(t *testing.T) {
	t.Setenv("MYGO_ENV", "development")
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	one := For("library").Credential("account")
	two := For("Library").Credential("account")
	if err := one.Write([]byte("first account")); err != nil {
		t.Fatal(err)
	}
	if err := two.Write([]byte("another account")); err != nil {
		t.Fatal(err)
	}
	if err := one.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if got, err := one.Read(); err != nil || string(got) != "new" {
		t.Fatalf("replacement returned %q, %v", got, err)
	}
	if err := one.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := one.Read(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading the deleted credential returned %v", err)
	}
	if got, err := two.Read(); err != nil || string(got) != "another account" {
		t.Fatalf("other instance returned %q, %v", got, err)
	}
}

// A legacy credential can read and replace the file created by an older build.
func TestLegacyCredentialFile(t *testing.T) {
	t.Setenv("MYGO_ENV", "development")
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	legacy := filepath.Join(dir, "session")
	if err := os.WriteFile(legacy, []byte("previous session"), 0o600); err != nil {
		t.Fatal(err)
	}
	cred := For("legacy-service").LegacyCredential("session", "session")
	if got, err := cred.Read(); err != nil || string(got) != "previous session" {
		t.Fatalf("legacy session returned %q, %v", got, err)
	}
	if err := cred.Write([]byte("new session")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(legacy); err != nil || string(got) != "new session" {
		t.Fatalf("old filename contains %q, %v", got, err)
	}
}
