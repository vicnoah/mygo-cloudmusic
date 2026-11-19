//go:build !linux

package app

import "github.com/egoist/mygo"

func setDesktopLyricsMouseIgnored(win *mygo.Window, ignore bool) {
	win.SetIgnoreMouseEvents(ignore)
}
