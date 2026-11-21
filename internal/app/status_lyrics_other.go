//go:build !windows

package app

import "time"

// macOS and Linux use the text label of the native tray item.
type statusLyricsNative struct{}

func (*statusLyricsNative) update(bool, statusLyricsFrame, func() time.Duration) {}
