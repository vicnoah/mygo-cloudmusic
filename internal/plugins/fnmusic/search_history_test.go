package fnmusic

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/internal/plugintest"
)

// Selecting an existing Feiniu search from the sidebar opens its results and moves it to the front of the saved history.
func TestFnmusicSearchRestoresHistory(t *testing.T) {
	h := plugintest.NewHost(ID)
	h.History = []string{"最近搜索", "旧版搜索"}
	home := h.Start(Registration(), nil).(*Adapter).Home()
	h.Activate(t)

	tt := ui.NewTester(h.Windows[pluginapi.MainWindow].Root, 1200, 800)
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("旧版搜索"); err != nil {
		t.Fatal(err)
	}
	if got := home.Route(); got.Page != PageSearch || got.Query != "旧版搜索" {
		t.Fatalf("after selecting history: route = %+v", got)
	}
	if h := h.History; len(h) != 2 || h[0] != "旧版搜索" || h[1] != "最近搜索" {
		t.Fatalf("history after selecting an older search: %v", h)
	}
}
