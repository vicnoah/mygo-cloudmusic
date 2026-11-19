package netease

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/internal/plugintest"
)

// Turning off checking for updates turns it off in the updater, and
// installing updates without asking can't be turned on without it.
func TestTurnOffUpdateChecks(t *testing.T) {
	h := plugintest.NewHost(ID)
	h.Desktop.Updates, h.Desktop.UpdateChecks = true, true
	h.Start(Registration(), nil)
	h.Activate(t)
	tt := ui.NewTester(h.Windows[pluginapi.SettingsWindow].Root, 560, 2000)
	tt.Frame()
	// The row's title is labelled as its switch is: click the switch, at
	// the row's end.
	toggle := func(title, desc string) {
		t.Helper()
		top, ok1 := tt.Find(title)
		bottom, ok2 := tt.Find(desc)
		if !ok1 || !ok2 {
			t.Fatalf("no row %s", title)
		}
		tt.ClickAt(560-28-16-12, (top.Y+bottom.Y+bottom.H)/2)
		tt.Frame()
	}
	toggle("自动检查更新", "每天检查一次是否有新版本")
	if h.Desktop.UpdateChecks {
		t.Fatal("checks for updates still")
	}
	toggle("自动下载并安装", "在后台下载新版本，下次打开时生效")
	if h.Desktop.UpdateDownloads {
		t.Fatal("turned on installing updates without checking for them")
	}
	// It does turn on while the updater checks.
	h.Desktop.UpdateChecks = true
	tt.Frame()
	toggle("自动下载并安装", "在后台下载新版本，下次打开时生效")
	if !h.Desktop.UpdateDownloads {
		t.Fatal("didn't turn on installing updates")
	}
}

// 检查更新 in the settings checks for updates as the user asked.
func TestCheckForUpdatesFromSettings(t *testing.T) {
	h := plugintest.NewHost(ID)
	h.Desktop.Updates, h.Desktop.UpdateChecks = true, true
	h.Start(Registration(), nil)
	h.Activate(t)
	tt := ui.NewTester(h.Windows[pluginapi.SettingsWindow].Root, 560, 2000)
	tt.Frame()
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	if h.UpdateChecksRequested != 1 {
		t.Fatalf("checked %d times", h.UpdateChecksRequested)
	}
}
