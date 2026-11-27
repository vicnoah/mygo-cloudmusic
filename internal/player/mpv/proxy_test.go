package mpv

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type proxyTransport func(*http.Request) (*http.Response, error)

func (f proxyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A corrupt proxy archive is discarded before the verified direct download replaces it.
func TestArchiveFallsBackAfterCorruptProxy(t *testing.T) {
	data := []byte("valid component")
	digest := sha256.Sum256(data)
	var hosts []string
	old := httpClient
	defer func() { httpClient = old }()
	httpClient = &http.Client{Transport: proxyTransport(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Hostname())
		body := data
		if len(hosts) == 1 {
			body = []byte("corrupted data!")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	file, err := os.CreateTemp(t.TempDir(), "archive")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var progress []int64
	err = fetchArchive(t.Context(), releaseAsset{Archive: "component.zip", Size: int64(len(data)), SHA256: fmt.Sprintf("%x", digest)}, file, func(done, total int64) { progress = append(progress, done) })
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file.Name())
	if err != nil || !bytes.Equal(got, data) || strings.Join(hosts, ",") != "gh-proxy.com,github.com" {
		t.Fatalf("data %q hosts %v error %v", got, hosts, err)
	}
	if len(progress) < 3 || progress[len(progress)-1] != int64(len(data)) {
		t.Fatalf("progress %v", progress)
	}
}
