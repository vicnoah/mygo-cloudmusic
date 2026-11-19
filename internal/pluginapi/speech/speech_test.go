package speech

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// wavSource reads 16-bit PCM from a WAV file, as a Source.
type wavSource struct {
	r        io.Reader
	rate, ch int
}

func openWAV(t *testing.T, path string) *wavSource {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	head := make([]byte, 12)
	io.ReadFull(f, head)
	s := &wavSource{r: f}
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(f, chunk[:]); err != nil {
			t.Fatal("wav: no data")
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		switch string(chunk[:4]) {
		case "fmt ":
			b := make([]byte, size)
			io.ReadFull(f, b)
			s.ch = int(binary.LittleEndian.Uint16(b[2:]))
			s.rate = int(binary.LittleEndian.Uint32(b[4:]))
		case "data":
			return s
		default:
			f.Seek(size, io.SeekCurrent)
		}
	}
}

func (s *wavSource) SampleRate() int { return s.rate }
func (s *wavSource) Channels() int   { return s.ch }
func (s *wavSource) Read(dst []float32) (int, error) {
	b := make([]byte, len(dst)*2)
	n, err := io.ReadFull(s.r, b)
	for i := range n / 2 {
		dst[i] = float32(int16(binary.LittleEndian.Uint16(b[2*i:]))) / (1 << 15)
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n / 2, err
}

// A recording transcribes to what is said in it, in segments that follow
// one another from where it starts in the episode. It needs a WAV of
// Chinese speech in SPEECH_WAV, and SPEECH_ROOT, which Download fills with
// what is missing of the model and the libraries, from the app's release;
// or else the libraries and Silero's VAD in SHERPA_ONNX_DIR and the model
// in SPEECH_MODEL.
func TestTranscribe(t *testing.T) {
	root, dir, wav := os.Getenv("SPEECH_ROOT"), os.Getenv("SPEECH_MODEL"), os.Getenv("SPEECH_WAV")
	var r *Recognizer
	var err error
	switch {
	case wav == "":
		t.Skip("set SPEECH_WAV, and SPEECH_ROOT or SHERPA_ONNX_DIR and SPEECH_MODEL")
	case root != "":
		if err := Download(context.Background(), root, func(int64, int64) {}); err != nil {
			t.Fatal(err)
		}
		r, err = NewRecognizer(root)
	case dir != "" && os.Getenv("SHERPA_ONNX_DIR") != "":
		r, err = newRecognizer(dir, os.Getenv("SHERPA_ONNX_DIR"))
	default:
		t.Skip("set SPEECH_ROOT, or SHERPA_ONNX_DIR and SPEECH_MODEL")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	from := 10 * time.Minute
	var segs []Segment
	var heard time.Duration
	start := time.Now()
	err = r.Transcribe(context.Background(), openWAV(t, wav), from, from+time.Minute, func(s Segment) {
		t.Logf("%v–%v %s", s.Start, s.End, s.Text)
		segs = append(segs, s)
	}, func(d time.Duration) { heard = d })
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("a minute in %v", time.Since(start))
	if len(segs) == 0 {
		t.Fatal("nothing transcribed")
	}
	for i, s := range segs {
		if s.Start < from || s.End <= s.Start || i > 0 && s.Start < segs[i-1].End {
			t.Errorf("segment %d at %v–%v", i, s.Start, s.End)
		}
	}
	if heard != from+time.Minute {
		t.Errorf("heard up to %v, want %v", heard, from+time.Minute)
	}
	var text strings.Builder
	for _, s := range segs {
		text.WriteString(s.Text)
	}
	if !strings.ContainsAny(text.String(), "，。？") {
		t.Errorf("no punctuation in %q", text.String())
	}
}
