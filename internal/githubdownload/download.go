// Package githubdownload provides automatic fallback for GitHub downloads.
package githubdownload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Try calls consume for each eligible source until it succeeds. consume must
// discard any partial output before returning an error.
func Try(ctx context.Context, client *http.Client, raw string, consume func(*http.Response) error) error {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return errors.New("invalid download URL")
	}
	urls := []string{raw}
	switch strings.ToLower(u.Hostname()) {
	case "github.com", "raw.githubusercontent.com", "raw.github.com", "api.github.com":
		if u.Scheme == "https" && u.Port() == "" {
			urls = []string{"https://gh-proxy.com/" + raw, raw}
		}
	}
	var failures []error
	for _, target := range urls {
		if err := ctx.Err(); err != nil {
			return err
		}
		attemptCtx, cancel := context.WithCancel(ctx)
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, target, nil)
		if err == nil {
			req.Header.Set("User-Agent", "mygo-updater")
			var resp *http.Response
			resp, err = client.Do(req)
			if err == nil {
				if resp.StatusCode != http.StatusOK {
					err = fmt.Errorf("HTTP %s", resp.Status)
				} else {
					timer := time.AfterFunc(30*time.Second, cancel)
					resp.Body = &idleBody{ReadCloser: resp.Body, timer: timer}
					err = consume(resp)
					timer.Stop()
				}
				resp.Body.Close()
			}
		}
		cancel()
		if err == nil {
			return nil
		}
		failures = append(failures, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("download failed: %w", errors.Join(failures...))
}

// Client limits connection and response-header waits without limiting large downloads.
func Client() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 15 * time.Second
	return &http.Client{Transport: t}
}

// idleBody aborts an attempt when no response bytes arrive for a while.
type idleBody struct {
	io.ReadCloser
	timer *time.Timer
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(30 * time.Second)
	}
	return n, err
}
