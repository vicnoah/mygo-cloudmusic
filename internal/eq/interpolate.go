package eq

import "math"

// Interpolate converts a curve between the two band layouts, by linear
// interpolation in dB over log frequency, as Supersonic does. Inputs of
// the wrong length come back as a flat curve of the target type.
func Interpolate(gains []float64, from, to string) []float64 {
	from, to = TypeOf(from), TypeOf(to)
	if from == to {
		return append([]float64(nil), gains...)
	}
	if len(gains) != BandCount(from) {
		return make([]float64, BandCount(to))
	}
	if from == TypeISO10 {
		return interpolate(gains, freqs10, freqs15)
	}
	return interpolate(gains, freqs15, freqs10)
}

// interpolate maps gains at src frequencies onto dst frequencies.
func interpolate(gains, src, dst []float64) []float64 {
	out := make([]float64, len(dst))
	for i, f := range dst {
		lo, hi := surrounding(src, f)
		switch {
		case lo == hi:
			out[i] = gains[lo]
		case lo < 0:
			// Below the lowest band: extend the slope of the first two.
			slope := (gains[1] - gains[0]) / math.Log(src[1]/src[0])
			out[i] = gains[0] + slope*math.Log(f/src[0])
		case hi < 0:
			out[i] = gains[len(gains)-1]
		default:
			t := math.Log(f/src[lo]) / math.Log(src[hi]/src[lo])
			out[i] = gains[lo] + t*(gains[hi]-gains[lo])
		}
	}
	return out
}

// surrounding returns the indices of the bands around f: the same index
// twice for a match, (-1, 0) below all, or (last, -1) above all.
func surrounding(freqs []float64, f float64) (int, int) {
	const tolerance = 0.01
	if f < freqs[0]-tolerance {
		return -1, 0
	}
	if f > freqs[len(freqs)-1]+tolerance {
		return len(freqs) - 1, -1
	}
	for i := 0; i < len(freqs)-1; i++ {
		if math.Abs(f-freqs[i]) < tolerance {
			return i, i
		}
		if f > freqs[i] && f < freqs[i+1] {
			return i, i + 1
		}
	}
	if math.Abs(f-freqs[len(freqs)-1]) < tolerance {
		return len(freqs) - 1, len(freqs) - 1
	}
	return len(freqs) - 1, -1
}
