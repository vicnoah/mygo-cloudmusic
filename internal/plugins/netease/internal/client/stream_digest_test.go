package netease

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type streamDigestResponse string

func (body streamDigestResponse) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path != "/eapi/song/enhance/player/url/v1" {
		return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(string(body))), Request: req,
	}, nil
}

// The stream resolver checks a cached file against the digest of the selected audio.
func TestSongURLUsesContentDigest(t *testing.T) {
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	http.DefaultTransport = streamDigestResponse(`{"code":200,"data":[{"id":17,"url":"http://example.test/record.mp3","type":"mp3","level":"exhigh","size":3,"md5":"900150983cd24fb0d6963f7d28e17f72"}]}`)
	stream, err := SongURL(t.Context(), 17, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	if stream.VerifyCachedContent == nil || !stream.VerifyCachedContent(strings.NewReader("abc")) ||
		stream.VerifyCachedContent(strings.NewReader("abd")) {
		t.Fatal("resolved content check did not distinguish equal-sized files")
	}
	http.DefaultTransport = streamDigestResponse(`{"code":200,"data":[{"id":17,"url":"http://example.test/record.mp3","type":"mp3","level":"exhigh","size":3,"md5":"incorrect"}]}`)
	stream, err = SongURL(t.Context(), 17, "exhigh")
	if err != nil || stream.VerifyCachedContent != nil {
		t.Fatalf("invalid digest changed the stream request: %v", err)
	}
}
