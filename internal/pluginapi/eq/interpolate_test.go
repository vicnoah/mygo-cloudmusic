package eq

import "testing"

// A flat curve stays flat when it changes band layouts.
func TestInterpolateFlat(t *testing.T) {
	up := Interpolate(make([]float64, 10), TypeISO10, TypeISO15)
	if len(up) != 15 {
		t.Fatalf("got %d bands, want 15", len(up))
	}
	for i, g := range up {
		if g != 0 {
			t.Errorf("band %d: got %v, want 0", i, g)
		}
	}
	down := Interpolate(up, TypeISO15, TypeISO10)
	if len(down) != 10 {
		t.Fatalf("got %d bands, want 10", len(down))
	}
	for i, g := range down {
		if g != 0 {
			t.Errorf("band %d: got %v, want 0", i, g)
		}
	}
}

// A curve of the wrong length becomes flat rather than panicking.
func TestInterpolateWrongLength(t *testing.T) {
	out := Interpolate([]float64{1, 2, 3}, TypeISO10, TypeISO15)
	if len(out) != 15 {
		t.Fatalf("got %d bands, want 15", len(out))
	}
	for i, g := range out {
		if g != 0 {
			t.Errorf("band %d: got %v, want 0", i, g)
		}
	}
}

// The lowest band carries across the layouts, and the highest stays.
func TestInterpolateKeepsEnds(t *testing.T) {
	gains := make([]float64, 10)
	gains[0], gains[1], gains[9] = 6, 6, -6
	out := Interpolate(gains, TypeISO10, TypeISO15)
	if out[0] != 6 {
		t.Errorf("lowest band: got %v, want 6", out[0])
	}
	if out[14] != -6 {
		t.Errorf("highest band: got %v, want -6", out[14])
	}
}
