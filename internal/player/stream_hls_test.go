package player

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"
)

type mp3Pieces struct {
	pieces   [][]byte
	duration time.Duration
}

func (p mp3Pieces) SegmentCount() int                 { return len(p.pieces) }
func (p mp3Pieces) SegmentDuration(int) time.Duration { return p.duration }
func (p mp3Pieces) TotalDuration() time.Duration      { return time.Duration(len(p.pieces)) * p.duration }
func (p mp3Pieces) Close()                            {}
func (p mp3Pieces) FetchSegment(_ context.Context, i int) ([]byte, error) {
	return p.pieces[i], nil
}

// Seeking across HLS segments decodes the same samples as a continuous MP3.
func TestSegmentedMP3SeekDecodesAcrossPieces(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	const framesPerPiece = 30
	const samplesPerFrame = 1152
	view := downloadView{download: downloaded(b), ctx: context.Background()}
	start := id3Size(view)
	pieces := mp3Pieces{duration: time.Second * (framesPerPiece * samplesPerFrame) / 44100}
	for left, off, count := int(start), int(start), 0; off+4 < len(b); count++ {
		n := frameSize(b[off:])
		if n == 0 || n > len(b)-off {
			break
		}
		off += n
		if (count+1)%framesPerPiece == 0 {
			pieces.pieces = append(pieces.pieces, b[left:off])
			left = off
		}
	}
	if len(pieces.pieces) < 4 {
		t.Fatal("fixture does not have enough complete pieces")
	}
	full := decodeAll(t, b)
	s, err := openSegmentedStream(context.Background(), pieces)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Start just before a segment boundary so a single read spans two pieces.
	const target = 3*framesPerPiece*samplesPerFrame - 256
	if err := s.SetPosition(target); err != nil {
		t.Fatal(err)
	}
	got := make([]float32, 4096)
	n, err := s.Read(got)
	if err != nil || n != len(got) || !slices.Equal(got, full[2*target:2*target+n]) {
		t.Fatalf("cross-segment seek decoded %d samples at frame %d: %v", n, target, err)
	}
}
