package speech

import "math"

// A resampler turns interleaved audio of a few channels at one rate into
// mono at SampleRate, as the models hear it: averaged, then filtered of
// what is above half the new rate, which would alias, by a windowed sinc.
// It keeps what it needs of one chunk for the next, so chunks resample as
// the whole would.
type resampler struct {
	ch   int
	step float64 // the input's samples to each of the output's
	fc   float64 // the cutoff, in cycles per input sample
	half int     // the filter's half-width, in input samples
	buf  []float32
	base int64   // the input sample buf[0] is
	t    float64 // where the next output sample is, in input samples
}

func newResampler(rate, channels int) *resampler {
	step := float64(rate) / SampleRate
	r := &resampler{ch: max(channels, 1), step: step}
	// Just under the output's Nyquist frequency, with eight of its
	// periods on either side.
	r.fc = 0.45 / max(step, 1)
	r.half = int(math.Ceil(8 * max(step, 1)))
	return r
}

// push resamples in, appending what it can of the output to out.
func (r *resampler) push(out, in []float32) []float32 {
	for i := 0; i+r.ch <= len(in); i += r.ch {
		var sum float32
		for c := range r.ch {
			sum += in[i+c]
		}
		r.buf = append(r.buf, sum/float32(r.ch))
	}
	for {
		center := int64(math.Floor(r.t))
		if center+int64(r.half) >= r.base+int64(len(r.buf)) {
			break
		}
		var acc, norm float64
		for j := center - int64(r.half) + 1; j <= center+int64(r.half); j++ {
			x := r.t - float64(j)
			w := r.tap(x)
			norm += w
			if k := j - r.base; k >= 0 {
				acc += w * float64(r.buf[k])
			}
		}
		out = append(out, float32(acc/norm))
		r.t += r.step
	}
	// Drop what no output to come reaches.
	if drop := int64(math.Floor(r.t)) - int64(r.half) - r.base; drop > 0 {
		drop = min(drop, int64(len(r.buf)))
		r.buf = r.buf[:copy(r.buf, r.buf[drop:])]
		r.base += drop
	}
	return out
}

// tap is the filter's weight x input samples from where it samples.
func (r *resampler) tap(x float64) float64 {
	n := float64(r.half)
	if x <= -n || x >= n {
		return 0
	}
	s := 2 * r.fc
	if x != 0 {
		s = math.Sin(2*math.Pi*r.fc*x) / (math.Pi * x)
	}
	// Blackman's window.
	p := math.Pi * (x/n + 1)
	return s * (0.42 - 0.5*math.Cos(p) + 0.08*math.Cos(2*p))
}
