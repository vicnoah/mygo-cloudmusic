package app

import (
	"strings"

	"github.com/waygu-music-plugin/sdk/desktop"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"
)

func (a *app) setDesktopLyricsColor(value string) error {
	if value != "" {
		if _, ok := desktop.ParseLyricsColor(value); !ok {
			return protocol.Failure{Code: "invalid-settings"}
		}
		value = strings.ToLower(value)
	}
	if a.settings.DesktopLyricsColor == value {
		return nil
	}
	next := a.settings
	next.DesktopLyricsColor = value
	if err := next.save(); err != nil {
		return err
	}
	a.settings = next
	a.desktopLyrics.Invalidate()
	a.desktopTasksChanged()
	return nil
}

func (a *app) newDesktopLyrics() {
	if a.desktopLyrics.IsOpen() || a.platformWindows[a.platformID][pluginapi.DesktopLyricsWindow].Root == nil {
		return
	}
	w := a.localDesktopLyrics()
	w.Open(a.windowOptions(pluginapi.DesktopLyricsWindow), func() bool { return a.quitting })
}

func (a *app) localDesktopLyrics() *desktop.LyricsWindow {
	w := &a.desktopLyrics
	if !w.IsOpen() {
		w.Configure(a.desktopLyricsPrefs(), func(prefs desktopLyricsSettings) { _ = a.saveDesktopLyricsPrefs(prefs) })
	}
	return w
}

func (a *app) desktopLyricsPrefs() desktopLyricsSettings {
	prefs := pluginapi.DefaultDesktopLyricsPreferences()
	prefs.Locked = a.settings.DesktopLyricsLocked
	if a.settings.DesktopLyrics != nil {
		prefs = *a.settings.DesktopLyrics
	}
	if prefs.FontSize <= 0 {
		prefs.FontSize = 30
	}
	prefs.Color = max(0, prefs.Color)
	return prefs
}

func (a *app) saveDesktopLyricsPrefs(prefs desktopLyricsSettings) error {
	before := a.desktopLyricsPrefs()
	next := a.settings
	next.DesktopLyricsLocked, next.DesktopLyrics = prefs.Locked, &prefs
	if err := next.save(); err != nil {
		return err
	}
	a.settings = next
	if before.Locked != prefs.Locked && a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
	return nil
}
