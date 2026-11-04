package songloft

import (
	"context"
	"os"
	"testing"
	"time"

	"netease-music-mygo/internal/platform"
)

func TestLiveSongloft(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("skipping live test without LIVE=1")
	}

	addr := os.Getenv("SONGLOFT_ADDR")
	if addr == "" {
		addr = os.Getenv("SONGLOFT_TEST_URL")
	}
	user := os.Getenv("SONGLOFT_USER")
	pass := os.Getenv("SONGLOFT_PASS")
	if addr == "" || user == "" {
		t.Skip("LIVE is set, but SONGLOFT_ADDR and SONGLOFT_USER do not name a service to try")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p := NewProvider(Config{
		Address:  addr,
		Username: user,
		Password: pass,
	})

	if err := p.Connect(ctx); err != nil {
		t.Fatalf("live connect failed: %v", err)
	}

	pg, err := p.Page(ctx, platform.PageRequest{Kind: platform.ListLibrary, Size: 5})
	if err != nil {
		t.Fatalf("live list library failed: %v", err)
	}
	if len(pg.Tracks) == 0 {
		t.Fatal("live library returned 0 tracks")
	}
	t.Logf("live connected successfully, total songs: %d, first: %s", pg.Total, pg.Tracks[0].Name)
}
