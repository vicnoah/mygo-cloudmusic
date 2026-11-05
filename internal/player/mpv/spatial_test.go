package mpv

import (
	"strings"
	"testing"
)

// TestBuildSpatialFilter checks that only headphone mode installs a filter
// and that the two profiles produce different widths.
func TestBuildSpatialFilter(t *testing.T) {
	if got := BuildSpatialFilter("speakers", "balanced"); got != "" {
		t.Fatalf("speakers filter = %q, want empty", got)
	}
	if got := BuildSpatialFilter("stereo", "balanced"); got != "" {
		t.Fatalf("stereo filter = %q, want empty", got)
	}
	balanced := BuildSpatialFilter("headphones", "balanced")
	wide := BuildSpatialFilter("headphones", "wide")
	for _, f := range []string{balanced, wide} {
		if !strings.Contains(f, "crossfeed") {
			t.Fatalf("headphone filter = %q, want crossfeed", f)
		}
		if !strings.Contains(f, "volume=0.65") {
			t.Fatalf("headphone filter = %q, want output gain", f)
		}
	}
	if balanced == wide {
		t.Fatal("balanced and wide filters should differ")
	}
}
