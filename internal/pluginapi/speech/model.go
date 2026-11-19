package speech

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// A file of the model, at its path in the model's directory.
type modelFile struct {
	path   string
	size   int64
	sha256 string
	// from is its path in ModelScope's repository, which differs from
	// Hugging Face's.
	from string
}

// The model: Qwen3-ASR-0.6B, quantized to int8, as sherpa-onnx packs it.
var modelFiles = []modelFile{
	{"conv_frontend.onnx", 44148281, "d22dc4423e0940e49884e903d2ea2f7e5567c14fc1aed97e4e26d6b8f208ef9e", "model_0.6B/conv_frontend.onnx"},
	{"encoder.int8.onnx", 182491662, "60748d3e6744a57c9c91e1b17424a6c2990567e8adceb0783940c03ed98fa9d9", "model_0.6B/encoder.int8.onnx"},
	{"decoder.int8.onnx", 755914231, "4f6885be5959ae26af3089d38ee7972c5fafbeeb1cf8d5e76eab6d8b61ca5771", "model_0.6B/decoder.int8.onnx"},
	{"tokenizer/merges.txt", 1671853, "8831e4f1a044471340f7c0a83d7bd71306a5b867e95fd870f74d0c5308a904d5", "tokenizer/merges.txt"},
	{"tokenizer/tokenizer_config.json", 12487, "4942d005604266809309cabc9f4e9cb89ce855d59b14681fdc0e1cc62ea26c4c", "tokenizer/tokenizer_config.json"},
	{"tokenizer/vocab.json", 2776833, "ca10d7e9fb3ed18575dd1e277a2579c16d108e32f27439684afa0e10b1440910", "tokenizer/vocab.json"},
}

// ModelName names the model, for the transcripts it writes.
const ModelName = "qwen3-asr-0.6b-int8"

// Where the model downloads from: ModelScope first, which listeners in
// China reach, then Hugging Face. Tests replace them.
var modelSources = []func(f modelFile) string{
	func(f modelFile) string {
		return "https://www.modelscope.cn/models/zengshuishui/Qwen3-ASR-onnx/resolve/master/" + f.from
	},
	func(f modelFile) string {
		return "https://huggingface.co/csukuangfj2/sherpa-onnx-qwen3-asr-0.6B-int8-2026-03-25/resolve/68818b2313fe77bd06f6a7c5068ff3ef59d02b8a/" + f.path
	},
}

// A nativeFile is a library of sherpa-onnx, or the VAD, as the app's
// release has it gzipped (scripts/sherpa): signed, on macOS, by the app's
// team, whose hardened runtime loads no other.
type nativeFile struct {
	name, asset      string
	size, gzSize     int64
	sha256, gzSHA256 string
}

// natives returns the libraries of this platform and the VAD, which
// Download downloads with the model; tests replace it.
var natives = func() []nativeFile {
	return append(slices.Clone(libFiles[platform()]), vadFile)
}

// nativeURL is where a native file downloads from; tests replace it.
var nativeURL = func(f nativeFile) string { return libBase + f.asset }

// ModelSize is how many bytes Download downloads: the model, and the
// libraries that run it.
func ModelSize() int64 {
	var n int64
	for _, f := range modelFiles {
		n += f.size
	}
	for _, f := range natives() {
		n += f.gzSize
	}
	return n
}

// Installed reports whether the model and its libraries are in root,
// whole.
func Installed(root string) bool {
	if !Available() {
		return false
	}
	for _, f := range modelFiles {
		info, err := os.Stat(filepath.Join(root, ModelName, filepath.FromSlash(f.path)))
		if err != nil || info.Size() != f.size {
			return false
		}
	}
	for _, f := range natives() {
		info, err := os.Stat(filepath.Join(libDir(root), f.name))
		if err != nil || info.Size() != f.size {
			return false
		}
	}
	return true
}

// Download downloads the model and the libraries that run it into root,
// telling progress how many of their bytes are in, until ctx ends. What
// came in of a file before goes on from where it stopped; each file is
// checked against its digest.
func Download(ctx context.Context, root string, progress func(done, total int64)) error {
	if !Available() {
		return ErrUnavailable
	}
	total := ModelSize()
	var before int64 // the bytes of the files done
	libs := libDir(root)
	if err := os.MkdirAll(libs, 0o755); err != nil {
		return err
	}
	for _, f := range natives() {
		path := filepath.Join(libs, f.name)
		if info, err := os.Stat(path); err == nil && info.Size() == f.size && digest(path) == f.sha256 {
			before += f.gzSize
			progress(before, total)
			continue
		}
		gz := modelFile{path: f.name + ".gz", size: f.gzSize, sha256: f.gzSHA256}
		err := fetch(ctx, nativeURL(f), path+".gz", gz, func(n int64) { progress(before+n, total) })
		if err == nil {
			err = gunzip(path+".gz", path, f)
		}
		if err != nil {
			return err
		}
		before += f.gzSize
		progress(before, total)
	}
	dir := filepath.Join(root, ModelName)
	for _, f := range modelFiles {
		path := filepath.Join(dir, filepath.FromSlash(f.path))
		if info, err := os.Stat(path); err == nil && info.Size() == f.size {
			before += f.size
			progress(before, total)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		var err error
		for _, src := range modelSources {
			err = fetch(ctx, src(f), path, f, func(n int64) { progress(before+n, total) })
			if err == nil || ctx.Err() != nil {
				break
			}
		}
		if err != nil {
			return err
		}
		before += f.size
		progress(before, total)
	}
	return nil
}

// errDigest says a file came in other than the model's.
var errDigest = errors.New("speech: the download does not match the model")

// fetch downloads url to path, through path.part, which it goes on from
// where it stopped, calling got with how much of the file is in.
func fetch(ctx context.Context, url, path string, f modelFile, got func(int64)) error {
	part := path + ".part"
	out, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	h := sha256.New()
	// What is in already counts toward the digest.
	have, err := io.Copy(h, out)
	if err != nil {
		return err
	}
	if have > f.size {
		have, h = 0, sha256.New()
		out.Truncate(0)
		out.Seek(0, io.SeekStart)
	}
	if have < f.size {
		if err := get(ctx, url, out, h, have, f.size, got); err != nil {
			return err
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != f.sha256 {
		// It is no good to go on from: the next try starts over.
		out.Close()
		os.Remove(part)
		return errDigest
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(part, path)
}

// get appends what url has from have on to out, which has have bytes of
// its size, and adds it to h.
func get(ctx context.Context, url string, out *os.File, h hash.Hash, have, size int64, got func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusPartialContent && have > 0:
	case res.StatusCode == http.StatusOK:
		// The whole file, from its start.
		if have > 0 {
			have = 0
			h.Reset()
			if err := out.Truncate(0); err != nil {
				return err
			}
		}
		if _, err := out.Seek(0, io.SeekStart); err != nil {
			return err
		}
	default:
		return fmt.Errorf("speech: %s: %s", url, res.Status)
	}
	buf := make([]byte, 256<<10)
	for have < size {
		n, err := res.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
			h.Write(buf[:n])
			have += int64(n)
			got(have)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// gunzip unpacks the file at gz to path, as f, and removes gz.
func gunzip(gz, path string, f nativeFile) error {
	in, err := os.Open(gz)
	if err != nil {
		return err
	}
	defer in.Close()
	r, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), r)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != f.sha256 {
		err = errDigest
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	in.Close()
	os.Remove(gz)
	return os.Rename(tmp, path)
}

// digest returns the SHA-256 of the file at path, or "".
func digest(path string) string {
	f, err := os.Open(path)
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

// Remove removes the model and its libraries from root.
func Remove(root string) error {
	err := os.RemoveAll(filepath.Join(root, ModelName))
	if e := os.RemoveAll(libDir(root)); err == nil {
		err = e
	}
	return err
}
