// Package av3a decodes NetEase's Audio Vivid streams, which are AVS3-P3
// audio in an MP4 container. libmpv/FFmpeg do not decode AV3A, so the
// decoder ships as the `molan-av3a` helper built from MOLAN-AV3A: it takes
// the MP4 and writes a multichannel WAV that MPV can play.
package av3a

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// ErrUnavailable means the helper is not installed for this platform.
var ErrUnavailable = errors.New("av3a: decoder is not installed")

// BinaryPath returns the helper path: MYGO_AV3A_BINARY, the app's resources,
// or a resources directory beside the executable during development.
func BinaryPath() string {
	if p := strings.TrimSpace(os.Getenv("MYGO_AV3A_BINARY")); p != "" {
		return p
	}
	candidates := make([]string, 0, 3)
	// The bundled helper is a macOS arm64 binary. On other targets it must
	// not be treated as usable; set MYGO_AV3A_BINARY to a native build.
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		if dir, err := mygo.App.Path(mygo.PathResources); err == nil {
			candidates = append(candidates, filepath.Join(dir, "molan-av3a"))
		}
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(dir, "..", "Resources", "molan-av3a"),
				filepath.Join(dir, "resources", "molan-av3a"),
			)
		}
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// Ready reports whether the decoder helper is installed.
func Ready() bool { return BinaryPath() != "" }

// Decode converts an AV3A MP4 to a WAV at output. The helper is stateful
// per stream; this starts a fresh process, so a seek does not inherit an
// earlier decoder's history.
func Decode(ctx context.Context, input, output string) error {
	bin := BinaryPath()
	if bin == "" {
		return ErrUnavailable
	}
	cmd := exec.CommandContext(ctx, bin, "decode", input, output)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("av3a: decode: %w", err)
		}
		return fmt.Errorf("av3a: decode: %w: %s", err, msg)
	}
	if fi, err := os.Stat(output); err != nil || fi.Size() == 0 {
		return fmt.Errorf("av3a: decoder produced no output")
	}
	return nil
}

// Info returns the helper's report for an input, for diagnostics.
func Info(ctx context.Context, input string) (string, error) {
	bin := BinaryPath()
	if bin == "" {
		return "", ErrUnavailable
	}
	out, err := exec.CommandContext(ctx, bin, "info", input).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("av3a: info: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
