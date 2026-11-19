package eq

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseIndex(t *testing.T) {
	index := "- [Sennheiser HD 650](./oratory1990/over-ear/Sennheiser%20HD%20650)\n" +
		"- [索尼 MDR-7506](./crinacle/over-ear/Sony%20MDR-7506)\n"
	metas, err := parseIndex([]byte(index))
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("got %d profiles, want 2", len(metas))
	}
	if metas[0].Name != "Sennheiser HD 650" || metas[0].Source != "oratory1990" || metas[0].Type != "over-ear" {
		t.Errorf("first profile is %+v", metas[0])
	}
	if metas[1].Name != "索尼 MDR-7506" {
		t.Errorf("second profile is %+v", metas[1])
	}
}

func TestParseProfile(t *testing.T) {
	var b strings.Builder
	b.WriteString("Preamp: -6.2 dB\n")
	freqs := []int{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}
	for i, f := range freqs {
		gain := 5 - i
		fmt.Fprintf(&b, "Filter %d: ON PK Fc %d Hz Gain %d.0 dB Q 0.70\n", i+1, f, gain)
	}
	p, err := parseProfile(Meta{Name: "Test", Path: "src/over-ear/Test"}, []byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if p.Preamp != -6.2 {
		t.Errorf("preamp %v", p.Preamp)
	}
	if len(p.Bands) != 10 || p.Bands[0] != 5 || p.Bands[9] != -4 {
		t.Errorf("bands %v", p.Bands)
	}
}

func TestParseProfileNeedsTenBands(t *testing.T) {
	_, err := parseProfile(Meta{}, []byte("Preamp: 0 dB\nFilter 1: ON PK Fc 31 Hz Gain 1.0 dB Q 0.70\n"))
	if err == nil {
		t.Error("a profile with one band was accepted")
	}
}

func TestProfileURL(t *testing.T) {
	u, err := profileURL(Meta{Path: "oratory1990/over-ear/Sennheiser%20HD%20650"})
	if err != nil {
		t.Fatal(err)
	}
	want := autoEQBaseURL + "oratory1990/over-ear/Sennheiser%20HD%20650/Sennheiser%20HD%20650%20FixedBandEQ.txt"
	if u != want {
		t.Errorf("got %q, want %q", u, want)
	}
}
