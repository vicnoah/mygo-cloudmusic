package network

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// BrowserUserAgent is used for media endpoints that expect a browser request.
const BrowserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"

var mediaClient = &http.Client{Timeout: 20 * time.Second}

// Fetch downloads a public media resource.
func Fetch(url string) ([]byte, error) {
	return FetchContext(context.Background(), url)
}

// FetchContext downloads a public media resource for the lifetime of ctx.
func FetchContext(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", BrowserUserAgent)
	res, err := mediaClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}
	return io.ReadAll(res.Body)
}
