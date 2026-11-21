package mpv

import "runtime"

// Platform identifies the application process, including under emulation.
func Platform() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}
