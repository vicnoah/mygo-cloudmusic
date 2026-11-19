package app

import (
	"testing"

	"waygu-music/internal/pluginapi"
)

// The host selects the first launchable public registration on startup and includes developer plugins only when enabled.
func TestStartupAndVisibilityFollowRegistrations(t *testing.T) {
	a := newTestApp()
	preview := testRegistration("preview", nil)
	preview.DeveloperOnly = true
	a.platformRegistrations = []pluginapi.Registration{
		preview, testRegistration("first", nil), testRegistration("second", nil),
	}
	if !a.initPlatformState() || a.platformID != "first" {
		t.Fatalf("first launch selected %q", a.platformID)
	}
	infos := a.platformInfos()
	if len(infos) != 2 || infos[0].ID != "first" || infos[1].ID != "second" {
		t.Fatalf("visible platforms = %+v", infos)
	}
	a.settings.DeveloperMode = true
	infos = a.platformInfos()
	if len(infos) != 3 || infos[0].ID != "preview" || infos[1].ID != "first" || infos[2].ID != "second" {
		t.Fatalf("developer platforms = %+v", infos)
	}
}

// Clicking version 5 times activates developer mode, and 5 more times deactivates it.
func TestDeveloperModeToggle(t *testing.T) {
	a := newTestApp()
	if a.settings.DeveloperMode {
		t.Fatal("developer mode enabled by default")
	}

	for i := 1; i <= 4; i++ {
		a.onVersionClicked()
		if a.settings.DeveloperMode {
			t.Fatalf("developer mode enabled prematurely at click %d", i)
		}
		if a.toast != "" {
			t.Fatalf("unexpected toast on intermediate click %d: %q", i, a.toast)
		}
		if i == 3 && ((platformHost{a: a}).DesktopSettings().VersionClicksRemaining != 2) {
			t.Fatal("incorrect remaining click count on click 3")
		}
		if i == 4 && ((platformHost{a: a}).DesktopSettings().VersionClicksRemaining != 1) {
			t.Fatal("incorrect remaining click count on click 4")
		}
	}
	a.onVersionClicked() // 5th click
	if !a.settings.DeveloperMode {
		t.Fatal("developer mode not enabled after 5 clicks")
	}
	if a.toast != "已开启开发者模式" {
		t.Fatalf("unexpected toast: %q", a.toast)
	}

	// 5 more clicks to disable
	a.toast = ""
	for i := 1; i <= 4; i++ {
		a.onVersionClicked()
		if !a.settings.DeveloperMode {
			t.Fatalf("developer mode disabled prematurely at click %d", i)
		}
		if a.toast != "" {
			t.Fatalf("unexpected toast on disable click %d: %q", i, a.toast)
		}
		if i == 3 && ((platformHost{a: a}).DesktopSettings().VersionClicksRemaining != 2) {
			t.Fatal("incorrect remaining click count on disable click 3")
		}
	}
	a.onVersionClicked() // 5th click
	if a.settings.DeveloperMode {
		t.Fatal("developer mode not disabled after 5 clicks")
	}
	if a.toast != "已关闭开发者模式" {
		t.Fatalf("unexpected toast: %q", a.toast)
	}
}
