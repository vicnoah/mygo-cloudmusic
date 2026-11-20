package app

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"

	"github.com/waygu-music-plugin/sdk/bridge"
)

type pcmFixture struct {
	ctx context.Context
	at  int64
}

func (*pcmFixture) SampleRate() int { return 44100 }
func (*pcmFixture) Channels() int   { return 2 }
func (*pcmFixture) Close() error    { return nil }

func (a *pcmFixture) SetPosition(frame int64) error {
	a.at = frame
	return a.ctx.Err()
}

func (a *pcmFixture) Read(p []float32) (int, error) {
	if err := a.ctx.Err(); err != nil {
		return 0, err
	}
	for i := range p {
		p[i] = float32(a.at*2 + int64(i))
	}
	a.at += int64(len(p) / 2)
	return len(p), nil
}

// The PCM data channel preserves seek and sample framing and reports a revoked reader.
func TestPluginAudioSeeksReadsAndReportsRevocation(t *testing.T) {
	host, child := net.Pipe()
	ctx, revoke := context.WithCancel(context.Background())
	defer revoke()
	go bridge.ServeAudio(func() (net.Conn, error) { return host, nil }, &pcmFixture{ctx: ctx})
	audio := bridge.NewAudio(bridge.PluginAudioDescription{Rate: 44100, Channels: 2}, child)
	defer audio.Close()
	if err := audio.SetPosition(7); err != nil {
		t.Fatal(err)
	}
	p := make([]float32, 4)
	if n, err := audio.Read(p); err != nil || n != len(p) || !slices.Equal(p, []float32{14, 15, 16, 17}) {
		t.Fatalf("PCM data = %v (%d, %v)", p, n, err)
	}
	revoke()
	if n, err := audio.Read(p); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("revoked stream = %d, %v", n, err)
	}
}
