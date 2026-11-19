package qqmusic

import (
	"context"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

type searchHistoryHost struct{ Host }

func (searchHistoryHost) Invalidate() {}

// Pressing Enter in QQ Music moves the query to the front of its saved history.
func TestQQSearchUsesPluginHistory(t *testing.T) {
	settings := &platformPreferences{SearchHistory: []string{"邓紫棋", "周杰伦"}}
	var saved []string
	home := NewAdapter(searchHistoryHost{}, settings, func() {
		saved = slices.Clone(settings.SearchHistory)
	}).Home()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	home.activate(ctx)
	defer home.dismissSuggestions()

	tt := ui.NewTester(home.searchField, 600, 250)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("周杰伦")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !slices.Equal(saved, []string{"周杰伦", "邓紫棋"}) {
		t.Fatalf("saved QQ Music history = %v", saved)
	}
	if route := home.Route(); route.Page != PageSearch || route.Query != "周杰伦" {
		t.Fatalf("search opened %+v", route)
	}
}
