package dlna

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Tests that the DLNA proxy starts, registers sources, and serves byte ranges for local files.
func TestDLNAProxyLocalFile(t *testing.T) {
	proxy, err := NewProxy()
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	defer proxy.Close()

	// Create a temp audio-like file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.flac")
	data := []byte("fLaC\x00\x00\x00\x22dummy flac content for streaming test")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	streamURL := proxy.Register("track-1001", StreamSource{
		LocalFile:   filePath,
		ContentType: "audio/flac",
	})

	if streamURL == "" {
		t.Fatal("expected non-empty streamURL")
	}

	// 1. Test regular GET
	resp, err := http.Get(streamURL)
	if err != nil {
		t.Fatalf("GET proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "audio/flac" {
		t.Errorf("got Content-Type %s, want audio/flac", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(data) {
		t.Errorf("got %q, want %q", string(body), string(data))
	}

	// 2. Test Range request (crucial for DLNA seek)
	req, _ := http.NewRequest("GET", streamURL, nil)
	req.Header.Set("Range", "bytes=4-15")
	rangeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Range request: %v", err)
	}
	defer rangeResp.Body.Close()

	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Errorf("got range status %d, want %d", rangeResp.StatusCode, http.StatusPartialContent)
	}
	rangeBody, _ := io.ReadAll(rangeResp.Body)
	if string(rangeBody) != string(data[4:16]) {
		t.Errorf("got range body %q, want %q", string(rangeBody), string(data[4:16]))
	}

	// 3. Test unregister
	proxy.Unregister("track-1001")
	notFoundResp, err := http.Get(streamURL)
	if err != nil {
		t.Fatalf("GET unregistered: %v", err)
	}
	defer notFoundResp.Body.Close()
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Errorf("got status %d after unregister, want 404", notFoundResp.StatusCode)
	}
}

// Tests that the DLNA proxy forwards requests to an upstream remote URL with headers.
func TestDLNAProxyRemoteStream(t *testing.T) {
	// Mock remote streaming server
	remoteHit := false
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteHit = true
		if auth := r.Header.Get("Authorization"); auth != "Bearer token-123" {
			t.Errorf("remote got auth header %q, want 'Bearer token-123'", auth)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock mp3 content"))
	}))
	defer remoteServer.Close()

	proxy, err := NewProxy()
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	defer proxy.Close()

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer token-123")
	streamURL := proxy.Register("track-remote", StreamSource{
		RemoteURL:   remoteServer.URL,
		Headers:     headers,
		ContentType: "audio/mpeg",
	})

	resp, err := http.Get(streamURL)
	if err != nil {
		t.Fatalf("GET proxy remote: %v", err)
	}
	defer resp.Body.Close()

	if !remoteHit {
		t.Error("remote server was not hit")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "mock mp3 content" {
		t.Errorf("got %q, want 'mock mp3 content'", string(b))
	}

	// Test LocalURL
	localURL := proxy.LocalURL("track-remote")
	localResp, err := http.Get(localURL)
	if err != nil {
		t.Fatalf("GET LocalURL: %v", err)
	}
	defer localResp.Body.Close()
	if localResp.StatusCode != http.StatusOK {
		t.Errorf("got LocalURL status %d, want 200", localResp.StatusCode)
	}
}

// Tests that the proxy overrides bogus text/html upstream Content-Type from servers like Feiniu.
func TestDLNAProxyOverridesBogusContentType(t *testing.T) {
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("audio binary payload"))
	}))
	defer remoteServer.Close()

	proxy, err := NewProxy()
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	defer proxy.Close()

	streamURL := proxy.Register("track-bogus", StreamSource{
		RemoteURL:   remoteServer.URL,
		ContentType: "audio/flac",
	})

	resp, err := http.Get(streamURL)
	if err != nil {
		t.Fatalf("GET proxy: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "audio/flac" {
		t.Errorf("got Content-Type %q, want 'audio/flac'", ct)
	}
}

// Tests MIME type detection for DLNA compatibility.
func TestFormatToMIME(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{"flac", "audio/flac"},
		{"mp3", "audio/mpeg"},
		{"m4a", "audio/mp4"},
		{"aac", "audio/mp4"},
		{"wav", "audio/wav"},
		{"dsd", "audio/x-dsd"},
		{"ape", "audio/x-ape"},
		{"unknown", "audio/mpeg"},
	}
	for _, tt := range tests {
		got := FormatToMIME(tt.format)
		if got != tt.want {
			t.Errorf("FormatToMIME(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

// Tests that the DLNA proxy serves album cover images for smart TVs and screen renderers.
func TestDLNAProxyCover(t *testing.T) {
	proxy, err := NewProxy()
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	defer proxy.Close()

	// 1. Local cover file
	tmpDir := t.TempDir()
	coverFile := filepath.Join(tmpDir, "cover.jpg")
	coverData := []byte("fake-jpeg-cover-data")
	if err := os.WriteFile(coverFile, coverData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	proxy.RegisterCover("track-cover-1", coverFile)
	coverURL := proxy.LANCoverURLFor("track-cover-1", "http://192.168.1.50:1400/desc.xml")

	resp, err := http.Get(coverURL)
	if err != nil {
		t.Fatalf("GET cover URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(coverData) {
		t.Errorf("got body %q, want %q", string(body), string(coverData))
	}
}
