//go:build !linux

package mpv

// PrepareRuntime prepares the decoder for the desktop runtime.
func PrepareRuntime() error { return nil }
