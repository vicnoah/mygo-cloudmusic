package platform

import (
	"context"
	"testing"

	"waygu-music/internal/pluginapi"
)

type testProvider struct{ id pluginapi.ID }

func (p testProvider) ID() pluginapi.ID { return p.id }
func (p testProvider) Name() string     { return string(p.id) }
func (p testProvider) Page(context.Context, PageRequest) (Page, error) {
	return Page{}, nil
}
func (p testProvider) Search(context.Context, string, SearchKind, int, int) (Page, error) {
	return Page{}, nil
}
func (p testProvider) Stream(context.Context, *pluginapi.Track) (*pluginapi.Stream, error) {
	return nil, nil
}
func (p testProvider) Lyrics(context.Context, *pluginapi.Track) (Lyrics, error) {
	return Lyrics{}, nil
}
func (p testProvider) Cover(context.Context, string, int) ([]byte, error) {
	return nil, nil
}

// A registry rejects duplicate platform IDs and returns a copy of its order.
func TestRegistryRegistration(t *testing.T) {
	a := testProvider{id: "first"}
	b := testProvider{id: "second"}
	r, err := NewRegistry(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Get(b.id); got != b {
		t.Fatalf("Get(%s) = %#v, want %#v", b.id, got, b)
	}
	ids := r.IDs()
	ids[0] = b.id
	if got := r.IDs()[0]; got != a.id {
		t.Fatalf("IDs returned internal storage: %v", r.IDs())
	}
	if err := r.Register(a); err == nil {
		t.Fatal("Register duplicate succeeded")
	}
}
