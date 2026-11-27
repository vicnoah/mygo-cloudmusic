package githubdownload

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Failed proxy responses and incomplete bodies fall back to the original GitHub URL.
func TestFallback(t *testing.T) {
	for _, mode := range []string{"http", "body", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			var hosts []string
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				hosts = append(hosts, r.URL.Hostname())
				if len(hosts) == 1 && mode == "http" {
					return &http.Response{StatusCode: 502, Status: "502", Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
				}
				body := "good"
				if len(hosts) == 1 {
					body = "bad"
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			err := Try(t.Context(), client, "https://github.com/example/file", func(r *http.Response) error {
				b, _ := io.ReadAll(r.Body)
				if string(b) != "good" {
					return errors.New("bad content")
				}
				return nil
			})
			if err != nil || len(hosts) != 2 || hosts[0] != "gh-proxy.com" || hosts[1] != "github.com" {
				t.Fatalf("hosts %v, error %v", hosts, err)
			}
		})
	}
}

// Canceling the proxy attempt does not start a direct download.
func TestCancelStopsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) { calls++; cancel(); return nil, context.Canceled })}
	if err := Try(ctx, client, "https://github.com/example/file", func(*http.Response) error { return nil }); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls %d, error %v", calls, err)
	}
}

// Non-GitHub URLs are fetched directly without being sent to the GitHub proxy.
func TestOtherHostIsDirect(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Hostname() != "example.com" {
			t.Fatalf("unexpected host %s", r.URL.Hostname())
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	if err := Try(t.Context(), client, "https://example.com/file", func(r *http.Response) error { return nil }); err != nil || calls != 1 {
		t.Fatalf("calls %d error %v", calls, err)
	}
}
