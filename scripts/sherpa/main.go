// Command sherpa packs sherpa-onnx's libraries, which write podcasts'
// captions on the device, for the app to download with the model: each
// platform's, signed for macOS with the app's Developer ID so that its
// hardened runtime loads them, and Silero's voice activity detector, which
// every platform shares. It writes them, gzipped, into a directory to
// upload as the assets of the release internal/speech downloads them from,
// and internal/speech/natives.go, which names them with their digests:
//
//	go run ./scripts/sherpa -sign "Developer ID Application: …" -out build/sherpa
//	gh release create sherpa-onnx-2.0.1 --prerelease --latest=false build/sherpa/*
//
// The upstream archives are checked against their digests, and kept in the
// user's cache between runs.
package main

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// version is the sherpa-onnx the app is written for: internal/speech
// mirrors the structs of its C API.
const version = "2.0.1"

// release is the tag of the app's release the files are uploaded to.
const release = "sherpa-onnx-" + version

const repo = "vicnoah/mygo-cloudmusic"

// An archive of sherpa-onnx's release, and the libraries the app takes
// from it, the C API's first.
type archive struct {
	name, sha256 string
	files        []string
}

var archives = map[string]archive{
	"darwin-arm64": {"osx-arm64-shared-no-tts-lib", "ac5594ad237db56ec4a6c4b3fc48700441b33a07bb71c235f35530a52a93f2e3",
		[]string{"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"}},
	"darwin-amd64": {"osx-x64-shared-no-tts-lib", "b0107b8166c4131245380bc9b984345c4ea8637c65807f09ca5f501c72926be5",
		[]string{"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"}},
	"linux-amd64": {"linux-x64-shared-no-tts-lib", "ce437357915491e6f2754b52bec7dcf62ff6fc6aaed9cdcd66f17807bbaef374",
		[]string{"libsherpa-onnx-c-api.so", "libonnxruntime.so"}},
	"linux-arm64": {"linux-aarch64-shared-cpu-lib", "15bd737eae467da32ec97e3740a329a535fd21e33caca4f327b0081451d11533",
		[]string{"libsherpa-onnx-c-api.so", "libonnxruntime.so"}},
	// Linked statically to the C runtime (MT): no Visual C++
	// Redistributable to install.
	"windows-amd64": {"win-x64-shared-MT-Release-no-tts-lib", "21da6a8b92d63ca02ad41dbfc120012343aaaec2e5e02b14819c6b4f38004509",
		[]string{"sherpa-onnx-c-api.dll", "onnxruntime.dll", "onnxruntime_providers_shared.dll"}},
	"windows-arm64": {"win-arm64-shared-MT-Release-no-tts-lib", "b3099bec60e6c3958275e90d0803d837cfe3ad6ed08528f29505d713b3a3e1bb",
		[]string{"sherpa-onnx-c-api.dll", "onnxruntime.dll", "onnxruntime_providers_shared.dll"}},
}

// The voice activity detector, Silero's, as sherpa-onnx publishes it.
const (
	vadURL    = "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx"
	vadSHA256 = "9e2449e1087496d8d4caba907f23e0bd3f78d91fa552479bb9c23ac09cbb1fd6"
)

func main() {
	sign := flag.String("sign", "", "the Developer ID that signs the macOS libraries (optional)")
	out := flag.String("out", "build/sherpa", "where to write the files to upload")
	flag.Parse()
	if err := run(*sign, *out); err != nil {
		fmt.Fprintln(os.Stderr, "sherpa:", err)
		os.Exit(1)
	}
}

// A packed file: its asset, and the file it unpacks to.
type packed struct {
	name, asset      string
	size, gzSize     int64
	sha256, gzSHA256 string
}

func run(identity, out string) error {
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	vad, err := fetch(vadURL, vadSHA256)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(vad)
	if err != nil {
		return err
	}
	vadFile, err := pack(out, "silero_vad.onnx", "silero_vad.onnx.gz", b)
	if err != nil {
		return err
	}
	libs := map[string][]packed{}
	platforms := make([]string, 0, len(archives))
	for p := range archives {
		platforms = append(platforms, p)
	}
	slices.Sort(platforms)
	for _, p := range platforms {
		a := archives[p]
		url := fmt.Sprintf("https://github.com/k2-fsa/sherpa-onnx/releases/download/v%s/sherpa-onnx-v%s-%s.tar.bz2", version, version, a.name)
		file, err := fetch(url, a.sha256)
		if err != nil {
			return err
		}
		tmp, err := os.MkdirTemp("", "sherpa-"+p)
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if err := extract(file, tmp, a.files); err != nil {
			return err
		}
		for _, name := range a.files {
			lib := filepath.Join(tmp, name)
			if strings.HasPrefix(p, "darwin-") && identity != "" {
				cmd := exec.Command("codesign", "--force", "--sign", identity, "--options", "runtime", "--timestamp", lib)
				if b, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("codesign %s/%s: %v\n%s", p, name, err, b)
				}
			}
			b, err := os.ReadFile(lib)
			if err != nil {
				return err
			}
			f, err := pack(out, name, p+"-"+name+".gz", b)
			if err != nil {
				return err
			}
			libs[p] = append(libs[p], f)
		}
		fmt.Printf("packed sherpa-onnx %s for %s\n", version, p)
	}
	return writeManifest(filepath.Join("internal", "speech", "natives.go"), platforms, libs, vadFile)
}

// pack gzips b into out as asset.
func pack(out, name, asset string, b []byte) (packed, error) {
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	w.Write(b)
	if err := w.Close(); err != nil {
		return packed{}, err
	}
	if err := os.WriteFile(filepath.Join(out, asset), gz.Bytes(), 0o644); err != nil {
		return packed{}, err
	}
	sum, gzSum := sha256.Sum256(b), sha256.Sum256(gz.Bytes())
	return packed{name: name, asset: asset, size: int64(len(b)), gzSize: int64(gz.Len()),
		sha256: hex.EncodeToString(sum[:]), gzSHA256: hex.EncodeToString(gzSum[:])}, nil
}

func writeManifest(file string, platforms []string, libs map[string][]packed, vad packed) error {
	var b strings.Builder
	lit := func(f packed) string {
		return fmt.Sprintf("{name: %q, asset: %q, size: %d, gzSize: %d, sha256: %q, gzSHA256: %q}", f.name, f.asset, f.size, f.gzSize, f.sha256, f.gzSHA256)
	}
	fmt.Fprintf(&b, "// Code generated by go run ./scripts/sherpa; DO NOT EDIT.\n\npackage speech\n\n")
	fmt.Fprintf(&b, "// libVersion is the sherpa-onnx whose libraries the app downloads.\nconst libVersion = %q\n\n", version)
	fmt.Fprintf(&b, "// libBase is where they download from: the assets of a release of the app.\nconst libBase = %q\n\n", "https://github.com/"+repo+"/releases/download/"+release+"/")
	fmt.Fprintf(&b, "// libFiles are the libraries of each platform, the C API's first.\nvar libFiles = map[string][]nativeFile{\n")
	for _, p := range platforms {
		fmt.Fprintf(&b, "\t%q: {\n", p)
		for _, f := range libs[p] {
			fmt.Fprintf(&b, "\t\t%s,\n", lit(f))
		}
		fmt.Fprintf(&b, "\t},\n")
	}
	fmt.Fprintf(&b, "}\n\n// vadFile is Silero's voice activity detector.\nvar vadFile = nativeFile%s\n", lit(vad)[0:])
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		return err
	}
	return os.WriteFile(file, src, 0o644)
}

// fetch returns the path of url's file in the user's cache, downloading it
// there first, unless it is there already: what has sha256 as its digest.
func fetch(url, sum string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	file := filepath.Join(cache, "mygo-cloudmusic", "sherpa-onnx", sum, path.Base(url))
	if digest(file) == sum {
		return file, nil
	}
	fmt.Println("downloading", url)
	res, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, res.Status)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	out, err := os.Create(file + ".part")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), res.Body)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != sum {
		os.Remove(file + ".part")
		return "", fmt.Errorf("%s: digest %s, want %s", url, got, sum)
	}
	return file, os.Rename(file+".part", file)
}

func digest(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// extract writes the files of the archive named, wherever in it they are,
// into dir.
func extract(archive, dir string, names []string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	tr := tar.NewReader(bzip2.NewReader(f))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := path.Base(h.Name)
		if h.Typeflag != tar.TypeReg || !want[name] {
			continue
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		delete(want, name)
	}
	if len(want) > 0 {
		return fmt.Errorf("%s has no %v", filepath.Base(archive), want)
	}
	return nil
}
