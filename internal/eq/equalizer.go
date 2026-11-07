// Package eq holds the graphic equalizer's curves and settings, its
// presets, and the AutoEQ headphone profiles.
package eq

import (
	"fmt"
	"math"
	"strings"
)

// The equalizer's two curves: 10 bands an octave apart, and 15 bands two
// thirds of an octave apart, as Supersonic's ISO10Band and ISO15Band.
const (
	TypeISO10 = "ISO10Band"
	TypeISO15 = "ISO15Band"
)

var (
	freqs10  = []float64{31.25, 62.5, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}
	freqs15  = []float64{25, 40, 63, 100, 160, 250, 400, 630, 1000, 1600, 2500, 4000, 6300, 10000, 16000}
	labels10 = []string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}
	labels15 = []string{"25", "40", "63", "100", "160", "250", "400", "630", "1k", "1.6k", "2.5k", "4k", "6.3k", "10k", "16k"}
)

// TypeOf returns typ if it is a known curve, and ISO15Band otherwise.
func TypeOf(typ string) string {
	if typ == TypeISO10 {
		return TypeISO10
	}
	return TypeISO15
}

// BandCount returns how many bands typ has.
func BandCount(typ string) int {
	if TypeOf(typ) == TypeISO10 {
		return len(freqs10)
	}
	return len(freqs15)
}

// Frequencies returns the center frequencies of typ, in Hz.
func Frequencies(typ string) []float64 {
	if TypeOf(typ) == TypeISO10 {
		return append([]float64(nil), freqs10...)
	}
	return append([]float64(nil), freqs15...)
}

// Labels returns the band labels of typ, for the interface.
func Labels(typ string) []string {
	if TypeOf(typ) == TypeISO10 {
		return append([]string(nil), labels10...)
	}
	return append([]string(nil), labels15...)
}

// Width returns the width of each band of typ, in octaves.
func Width(typ string) float64 {
	if TypeOf(typ) == TypeISO10 {
		return 1
	}
	return 2. / 3
}

// BuildEqualizerFilter returns the MPV audio filter for the curve: the
// preamp and the bands whose gain is not zero, as Supersonic builds it.
func BuildEqualizerFilter(preamp float64, gains []float64, typ string) string {
	var filters []string
	if math.Abs(preamp) >= 0.01 {
		filters = append(filters, fmt.Sprintf("volume=volume=%.1fdB", preamp))
	}
	freqs := Frequencies(typ)
	width := Width(typ)
	for i, f := range freqs {
		if i >= len(gains) {
			break
		}
		g := gains[i]
		if math.Abs(g) >= 0.02 {
			filters = append(filters, fmt.Sprintf("equalizer=f=%d:g=%.2f:t=o:w=%.2f", int(math.Round(f)), g, width))
		}
	}
	return strings.Join(filters, ",")
}
