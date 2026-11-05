package mpv

import (
	"fmt"
	"math"
	"strings"
)

// Standard ISO 10-band center frequencies (Hz).
var ISO10Bands = [10]int{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// Standard ISO 10-band display labels.
var ISO10BandLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

// EqualizerPreset defines a named equalizer curve.
type EqualizerPreset struct {
	ID    string
	Name  string
	Gains [10]float64
}

// Built-in presets.
var Presets = []EqualizerPreset{
	{ID: "flat", Name: "原声直通", Gains: [10]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	{ID: "vocal", Name: "流行人声", Gains: [10]float64{-1, 0, 1, 2.5, 3.5, 3, 2, 1, 0, -1}},
	{ID: "bass", Name: "低音增强", Gains: [10]float64{5.5, 4.5, 3.5, 2, 0.5, 0, 0, 0, 0, 0}},
	{ID: "treble", Name: "清澈高音", Gains: [10]float64{0, 0, 0, 0, 0.5, 1.5, 3, 4.5, 5, 5.5}},
	{ID: "rock", Name: "摇滚", Gains: [10]float64{4.5, 3.5, 2, 0.5, -0.5, 0, 1.5, 3, 4, 4.5}},
	{ID: "classical", Name: "古典", Gains: [10]float64{4, 3, 2, 1.5, -0.5, -0.5, 0, 2, 3, 3.5}},
	{ID: "electronic", Name: "电子舞曲", Gains: [10]float64{4.5, 4, 1.5, 0, -1, 1, 0.5, 2, 4, 4.5}},
}

// FindPreset returns the preset matching id, or the flat preset.
func FindPreset(id string) (EqualizerPreset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Presets[0], false
}

// BuildEqualizerFilter constructs the FFmpeg audio filter string for MPV.
func BuildEqualizerFilter(preamp float64, gains [10]float64) string {
	var filters []string
	if math.Abs(preamp) >= 0.05 {
		filters = append(filters, fmt.Sprintf("volume=volume=%.1fdB", preamp))
	}
	for i, f := range ISO10Bands {
		g := gains[i]
		if math.Abs(g) >= 0.05 {
			filters = append(filters, fmt.Sprintf("equalizer=f=%d:g=%.1f:t=o:w=1.00", f, g))
		}
	}
	return strings.Join(filters, ",")
}
