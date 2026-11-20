package builtin

import "testing"

// A fresh host can construct Feiniu and exposes no other built-in platform.
func TestOnlyFeiniuIsBundled(t *testing.T) {
	entries := Platforms()
	if len(entries) != 1 || entries[0].ID != "fnmusic" || entries[0].Start == nil {
		t.Fatalf("unexpected built-in platform registrations: %+v", entries)
	}
}
