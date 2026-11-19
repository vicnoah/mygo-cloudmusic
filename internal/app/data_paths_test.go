package app

import (
	"github.com/egoist/mygo"
	"os"
	"path/filepath"
	"testing"
)

// The data kept under the app's old name moves to the directory of its
// name, which Path made empty, but not over data kept there already.
func TestMoveOldData(t *testing.T) {
	root := t.TempDir()
	old, dir := filepath.Join(root, "MyGo 云音乐"), filepath.Join(root, "Wagyu Music")
	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte("old"), 0o600)
	os.MkdirAll(dir, 0o700)
	moveDir(old, dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "old" {
		t.Fatalf("settings %q after moving", b)
	}

	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte("older"), 0o600)
	moveDir(old, dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "old" {
		t.Fatalf("settings %q, moved over what was there", b)
	}
}

// The newest of the old names wins when a listener has data under both:
// the rename chain must not bring back what the older one holds.
func TestMoveOldDataTakesNewestOldName(t *testing.T) {
	root, cacheRoot := t.TempDir(), t.TempDir()
	dir, cache := filepath.Join(root, "Wagyu Music"), filepath.Join(cacheRoot, "Wagyu Music")
	mygo.App.SetPath(mygo.PathUserData, dir)
	mygo.App.SetPath(mygo.PathCache, cache)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	defer mygo.App.SetPath(mygo.PathCache, "")
	os.MkdirAll(dir, 0o700)
	os.MkdirAll(cache, 0o700)
	for old, settings := range map[string]string{"MyGo 云音乐": "new", "网易云音乐": "old"} {
		os.MkdirAll(filepath.Join(root, old), 0o700)
		os.WriteFile(filepath.Join(root, old, "settings.json"), []byte(settings), 0o600)
	}
	moveOldData()
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "new" {
		t.Fatalf("settings %q, want the newest old name's", b)
	}
	if _, err := os.Stat(filepath.Join(root, "网易云音乐")); err != nil {
		t.Fatalf("the older data moved after the newest: %v", err)
	}
}
