// Package speech transcribes speech on the device, for podcasts' captions:
// Silero's voice activity detection cuts the audio into what is said, and
// Qwen3-ASR, through sherpa-onnx, writes each of those down, with
// punctuation. Nothing leaves the device but the model's download.
package speech

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"time"
)

// SampleRate is the rate of the audio the models hear.
const SampleRate = 16000

// A Segment is what is said between two pauses, and when.
type Segment struct {
	Start, End time.Duration
	Text       string
}

// A Source is audio to transcribe: interleaved samples of its channels, at
// its rate.
type Source interface {
	SampleRate() int
	Channels() int
	Read(dst []float32) (int, error)
}

// A Recognizer transcribes audio, one Transcribe at a time. It holds the
// model, about a gigabyte of memory, until it is closed.
type Recognizer struct {
	rec, vad uintptr
}

// NewRecognizer loads the model and the libraries Download put in root.
func NewRecognizer(root string) (*Recognizer, error) {
	return newRecognizer(filepath.Join(root, ModelName), libDir(root))
}

func newRecognizer(dir, res string) (*Recognizer, error) {
	if err := load(res); err != nil {
		return nil, err
	}
	var cs cstrings
	cfg := cOfflineRecognizerConfig{
		feat:           cFeatureConfig{sampleRate: SampleRate, featureDim: 80},
		decodingMethod: cs.of("greedy_search"),
	}
	m := &cfg.model
	// Half the cores, as many as four: the rest stay for the app and
	// the rest of the system, and more gain little.
	m.numThreads = int32(min(max(runtime.NumCPU()/2, 1), 4))
	m.provider = cs.of("cpu")
	m.qwen3ASR = cQwen3ASR{
		convFrontend: cs.of(filepath.Join(dir, "conv_frontend.onnx")),
		encoder:      cs.of(filepath.Join(dir, "encoder.int8.onnx")),
		decoder:      cs.of(filepath.Join(dir, "decoder.int8.onnx")),
		tokenizer:    cs.of(filepath.Join(dir, "tokenizer")),
		// A segment is at most maxSpeech long: some 150 characters of
		// fast talk, with their punctuation.
		maxTotalLen:  768,
		maxNewTokens: 256,
		temperature:  1e-6,
		topP:         0.8,
		seed:         42,
	}
	r := &Recognizer{}
	r.rec = sherpa.createRecognizer(&cfg)
	if r.rec == 0 {
		return nil, errors.New("speech: the model would not load")
	}
	vc := cVadModelConfig{
		silero: cSileroVad{
			model:      cs.of(filepath.Join(res, "silero_vad.onnx")),
			threshold:  0.5,
			minSilence: 0.5,
			minSpeech:  0.25,
			windowSize: 512,
			maxSpeech:  maxSpeech,
		},
		sampleRate: SampleRate,
		numThreads: 1,
		provider:   cs.of("cpu"),
	}
	r.vad = sherpa.createVad(&vc, 2*maxSpeech)
	runtime.KeepAlive(cs)
	if r.vad == 0 {
		sherpa.destroyRecognizer(r.rec)
		return nil, errors.New("speech: the voice activity detector would not load")
	}
	return r, nil
}

// maxSpeech is how long, in seconds, a segment goes on before the
// detector cuts it at a softer pause.
const maxSpeech = 20

// Close lets the model go.
func (r *Recognizer) Close() {
	if r.vad != 0 {
		sherpa.destroyVad(r.vad)
	}
	if r.rec != 0 {
		sherpa.destroyRecognizer(r.rec)
	}
	r.rec, r.vad = 0, 0
}

// Transcribe transcribes src, which is from into the episode, up to until
// into it, or to its end if until is 0, or until ctx ends. It calls said
// with each segment as it is written down, and heard with how far all is
// written down, now and then.
func (r *Recognizer) Transcribe(ctx context.Context, src Source, from, until time.Duration, said func(Segment), heard func(time.Duration)) error {
	sherpa.vadReset(r.vad)
	rs := newResampler(src.SampleRate(), src.Channels())
	in := make([]float32, 8192*max(src.Channels(), 1))
	var out []float32
	var fed int64 // samples given to the detector
	limit := int64(-1)
	if until > from {
		limit = int64((until - from).Seconds() * SampleRate)
	}
	at := func(n int64) time.Duration { return from + time.Duration(n)*time.Second/SampleRate }
	// How far all is written down: what was fed, but for what the
	// detector holds, said or not.
	var done time.Duration
	lastHeard := from
	drain := func() error {
		for sherpa.vadEmpty(r.vad) == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			seg := sherpa.vadFront(r.vad)
			start, n := seg.start, int64(seg.n)
			text := r.decode(seg)
			sherpa.destroySeg(seg)
			sherpa.vadPop(r.vad)
			s := Segment{Start: at(start), End: at(start + n), Text: text}
			done = s.End
			if s.Text != "" {
				said(s)
			}
		}
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := src.Read(in)
		out = rs.push(out[:0], in[:n])
		end := limit >= 0 && fed+int64(len(out)) >= limit
		if end {
			out = out[:limit-fed]
		}
		if len(out) > 0 {
			sherpa.vadAccept(r.vad, &out[0], int32(len(out)))
			fed += int64(len(out))
		}
		if e := drain(); e != nil {
			return e
		}
		if done > lastHeard+2*time.Second {
			lastHeard = done
			heard(done)
		}
		if end || err == io.EOF {
			sherpa.vadFlush(r.vad)
			if e := drain(); e != nil {
				return e
			}
			heard(at(fed))
			return nil
		}
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		// Silence: all is written down as far as it went.
		if sherpa.vadEmpty(r.vad) != 0 && sherpa.vadDetected(r.vad) == 0 {
			done = at(fed)
		}
	}
}

// decode writes down what is said in seg.
func (r *Recognizer) decode(seg *cSpeechSegment) string {
	if seg.n <= 0 {
		return ""
	}
	stream := sherpa.createStream(r.rec)
	defer sherpa.destroyStream(stream)
	sherpa.acceptWaveform(stream, SampleRate, seg.samples, seg.n)
	sherpa.decode(r.rec, stream)
	res := sherpa.result(stream)
	if res == nil {
		return ""
	}
	defer sherpa.destroyResult(res)
	return goString(res.text)
}
