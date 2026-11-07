package eq

import (
	"os"
	"path/filepath"
	"testing"
)

// Saved presets come back with their curve, and built-ins cannot be
// replaced or deleted.
func TestPresetStore(t *testing.T) {
	store := NewPresetStore(t.TempDir())
	builtins := store.List()
	if len(builtins) != 12 {
		t.Fatalf("got %d built-ins, want 12", len(builtins))
	}
	if !builtins[0].Builtin || builtins[0].Name != "Flat" {
		t.Errorf("first built-in is %+v", builtins[0])
	}

	bands := make([]float64, BandCount(TypeISO15))
	bands[0], bands[14] = 4, -3
	if err := store.Save(Preset{Name: "我的低音", Type: TypeISO15, Preamp: -2, Bands: bands}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range store.List() {
		if p.Name == "我的低音" {
			found = true
			if p.Builtin || p.Preamp != -2 || len(p.Bands) != len(bands) || p.Bands[0] != 4 {
				t.Errorf("saved preset came back as %+v", p)
			}
		}
	}
	if !found {
		t.Error("saved preset is not listed")
	}

	if err := store.Save(Preset{Name: "Rock", Type: TypeISO15, Bands: bands}); err == nil {
		t.Error("a built-in was replaced")
	}
	if err := store.Delete("Rock"); err == nil {
		t.Error("a built-in was deleted")
	}
	if err := store.Delete("我的低音"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, presetFile("我的低音"))); !os.IsNotExist(err) {
		t.Errorf("deleted preset file remains: %v", err)
	}
}
