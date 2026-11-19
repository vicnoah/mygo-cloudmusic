package plugintest

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/eq"
)

type autoEQHost struct {
	*Host
	posted  chan func()
	applied []eq.Settings
}

func (h *autoEQHost) Post(fn func())                   { h.posted <- fn }
func (h *autoEQHost) ApplyEqualizer(prefs eq.Settings) { h.applied = append(h.applied, prefs) }

func CheckAutoEQActivation(t *testing.T, entry pluginapi.Registration, request func(pluginapi.Adapter, *eq.AutoEQ, eq.Meta)) {
	t.Helper()
	dir := t.TempDir()
	meta := eq.Meta{Name: "Test headphones", Path: "test/headphones"}
	profile := eq.Profile{Meta: meta, Preamp: -3, Bands: make([]float64, 10)}
	b, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	cacheName := fmt.Sprintf("%x.json", md5.Sum([]byte(meta.Path)))
	if err := os.WriteFile(filepath.Join(dir, cacheName), b, 0o600); err != nil {
		t.Fatal(err)
	}
	auto := eq.NewAutoEQ(dir, time.Second)
	host := &autoEQHost{Host: NewHost(entry.ID), posted: make(chan func(), 1)}
	start := entry.Start
	entry.Start = func(s pluginapi.Startup) pluginapi.Adapter {
		s.Host = host
		return start(s)
	}
	adapter := host.Start(entry, nil)

	take := func() func() {
		t.Helper()
		select {
		case fn := <-host.posted:
			return fn
		case <-time.After(5 * time.Second):
			t.Fatal("the cached profile did not post a result")
			return nil
		}
	}

	first, cancel := context.WithCancel(t.Context())
	defer cancel()
	host.Lifecycle.Activate(first, nil)
	request(adapter, auto, meta)
	delayed := take()
	cancel()
	host.Lifecycle.Deactivate(true)
	host.Activate(t)
	delayed()
	if len(host.applied) != 0 || host.TakeToast() != "" {
		t.Fatal("an earlier activation changed the current equalizer or showed a toast")
	}

	request(adapter, auto, meta)
	take()()
	if len(host.applied) != 1 || host.applied[0].AutoEQName != meta.Name || host.applied[0].Preamp != profile.Preamp {
		t.Fatalf("the reactivated plugin did not apply the requested profile: %+v", host.applied)
	}
}
