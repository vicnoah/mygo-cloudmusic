package app

import (
	"github.com/waygu-music-plugin/sdk/desktop"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

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
