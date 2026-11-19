package speech

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withModel makes the model files with these contents, served by the
// sources for the test, and a library, lib, gzipped, served at libURL.
func withModel(t *testing.T, files map[string][]byte, libURL string, sources ...func(modelFile) string) {
	oldFiles, oldSources, oldNatives, oldURL := modelFiles, modelSources, natives, nativeURL
	t.Cleanup(func() { modelFiles, modelSources, natives, nativeURL = oldFiles, oldSources, oldNatives, oldURL })
	modelFiles = nil
	for _, path := range []string{"model.onnx", "tokenizer/vocab.json"} {
		sum := sha256.Sum256(files[path])
		modelFiles = append(modelFiles, modelFile{path: path, size: int64(len(files[path])), sha256: hex.EncodeToString(sum[:]), from: "repo/" + path})
	}
	modelSources = sources
	gz := gzipped(lib)
	sum, gzSum := sha256.Sum256(lib), sha256.Sum256(gz)
	f := nativeFile{name: "libtest.dylib", asset: "lib.gz", size: int64(len(lib)), gzSize: int64(len(gz)), sha256: hex.EncodeToString(sum[:]), gzSHA256: hex.EncodeToString(gzSum[:])}
	natives = func() []nativeFile { return []nativeFile{f} }
	nativeURL = func(nativeFile) string { return libURL }
}

var lib = bytes.Repeat([]byte("\xcf\xfa\xed\xfe code "), 1000)

func gzipped(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

func serve(t *testing.T, h http.HandlerFunc) func(modelFile) string {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return func(f modelFile) string { return srv.URL + "/" + f.from }
}

// A source that fails gives way to the next, and a file that stopped
// halfway goes on from there, asking only for the rest.
func TestDownloadResumes(t *testing.T) {
	files := map[string][]byte{
		"model.onnx":           bytes.Repeat([]byte("onnx"), 50000),
		"tokenizer/vocab.json": []byte(`{"你":1}`),
	}
	var ranges []string
	libURL := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(gzipped(lib)) })(modelFile{from: "lib.gz"})
	withModel(t, files, libURL,
		serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusBadGateway) }),
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			ranges = append(ranges, r.Header.Get("Range"))
			b := files[strings.TrimPrefix(r.URL.Path, "/repo/")]
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b))
		}))
	root := t.TempDir()
	dir := filepath.Join(root, ModelName)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "model.onnx.part"), files["model.onnx"][:120000], 0o644)
	var last, total int64
	if err := Download(context.Background(), root, func(d, t int64) { last, total = d, t }); err != nil {
		t.Fatal(err)
	}
	if !Installed(root) {
		t.Fatal("not installed")
	}
	if got, _ := os.ReadFile(filepath.Join(libDir(root), "libtest.dylib")); !bytes.Equal(got, lib) {
		t.Error("the library differs")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "model.onnx")); !bytes.Equal(got, files["model.onnx"]) {
		t.Error("model.onnx differs")
	}
	if ranges[0] != "bytes=120000-" || ranges[1] != "" {
		t.Errorf("asked for %q", ranges)
	}
	if last != total || total != 200009+int64(len(gzipped(lib))) {
		t.Errorf("progress %d of %d", last, total)
	}
}

// A file that comes in other than the model's is thrown away, not kept
// as it, nor gone on from.
func TestDownloadChecksDigest(t *testing.T) {
	files := map[string][]byte{"model.onnx": []byte("model"), "tokenizer/vocab.json": []byte("{}")}
	evil := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("evil!")) })
	withModel(t, files, evil(modelFile{from: "lib.gz"}), evil)
	dir := t.TempDir()
	err := Download(context.Background(), dir, func(int64, int64) {})
	if !errors.Is(err, errDigest) {
		t.Fatalf("err = %v", err)
	}
	for _, name := range []string{"libtest.dylib", "libtest.dylib.gz", "libtest.dylib.gz.part"} {
		if _, err := os.Stat(filepath.Join(libDir(dir), name)); err == nil {
			t.Errorf("%s kept", name)
		}
	}
	if Installed(dir) {
		t.Error("installed")
	}
}

// A library changed after it downloaded, as by whatever else could write
// there, does not load into the app.
func TestChangedLibraryRefused(t *testing.T) {
	withModel(t, map[string][]byte{}, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "libtest.dylib")
	os.WriteFile(path, lib, 0o755)
	if err := verifyLibs(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, append(lib, "evil"...), 0o755)
	if verifyLibs(dir) == nil {
		t.Error("a changed library loads")
	}
}
