package app

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// An update whose proxy copy fails signature verification is retried directly and returned intact.
func TestUpdateSourceFallsBackAfterBadSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("verified update")
	digest := sha256.Sum256(body)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, digest[:]))
	const raw = "https://github.com/vicnoah/mygo-cloudmusic/releases/download/v1/file"
	var hosts []string
	source := &githubUpdateSource{key: base64.StdEncoding.EncodeToString(pub), files: map[string]updateFile{raw: {URL: raw, Size: int64(len(body)), Signature: signature}}, client: &http.Client{Transport: updateTransport(func(req *http.Request) (*http.Response, error) {
		hosts = append(hosts, req.URL.Hostname())
		b := body
		if len(hosts) == 1 {
			b = []byte("invalid update!")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
	})}}
	f, err := source.Open(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(got, body) || strings.Join(hosts, ",") != "gh-proxy.com,github.com" {
		t.Fatal(fmt.Sprintf("got %q, hosts %v, error %v", got, hosts, err))
	}
}

// Invalid proxy metadata falls back to GitHub and records the archive needed by the updater.
func TestUpdateManifestFallsBack(t *testing.T) {
	const raw = "https://github.com/vicnoah/mygo-cloudmusic/releases/download/v1/archive.tar.gz"
	valid := fmt.Sprintf(`{"version":"1.0.0","url":%q,"size":15,"signature":"signature"}`, raw)
	var hosts []string
	source := &githubUpdateSource{client: &http.Client{Transport: updateTransport(func(req *http.Request) (*http.Response, error) {
		hosts = append(hosts, req.URL.Hostname())
		data := valid
		if len(hosts) == 1 {
			data = "bad gateway"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data)), Header: make(http.Header)}, nil
	})}}
	f, err := source.Manifest(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil || string(got) != valid || source.files[raw].Size != 15 || strings.Join(hosts, ",") != "gh-proxy.com,github.com" {
		t.Fatalf("manifest %q hosts %v error %v", got, hosts, err)
	}
}
