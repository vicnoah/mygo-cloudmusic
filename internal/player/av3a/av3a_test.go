package av3a

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The helper protocol is one command: decode <mp4> <wav>. The wrapper must
// report a missing output rather than let MPV load a zero byte file.
func TestDecodeWithHelper(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	script := "#!/bin/sh\nprintf 'RIFF' > \"$3\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYGO_AV3A_BINARY", helper)
	out := filepath.Join(dir, "out.wav")
	if err := Decode(context.Background(), "in.mp4", out); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("output: %v %v", fi, err)
	}
}
