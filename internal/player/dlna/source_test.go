package dlna

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A server returning FLAC or rejecting authentication cannot masquerade as compatible MP3.
func TestCompatibleSourceRejectsWrongFormatAndAuth(t *testing.T) {
	p, err := NewProxy()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/private" {
			http.Error(w, "secret upstream response", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "fLaCfake")
	}))
	defer srv.Close()
	for _, tc := range []struct{ path, want string }{
		{"/ignored", "没有返回请求的 MP3"},
		{"/private", "HTTP 401"},
	} {
		var out bytes.Buffer
		_, staged, err := p.PrepareMP3(t.Context(), StreamSource{RemoteURL: srv.URL + tc.path}, &out)
		if err == nil || !strings.Contains(err.Error(), tc.want) || staged || out.Len() != 0 {
			t.Fatalf("preparing %s: got %v (staged=%t, bytes=%d)", tc.path, err, staged, out.Len())
		}
	}
}
