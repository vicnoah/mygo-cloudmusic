package netease

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type cancelRoundTripper struct {
	cancel func()
}

func (r cancelRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.cancel()
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("activation context did not reach the request")
}

// Cancelling a platform activation stops an in-flight catalog request.
func TestCancelCatalogRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	previous := httpClient
	defer func() { httpClient = previous }()
	httpClient = &http.Client{Transport: cancelRoundTripper{cancel: cancel}}
	if _, err := SearchPage(ctx, "test", SearchSongs, 5, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("request returned %v, want cancellation", err)
	}
}
