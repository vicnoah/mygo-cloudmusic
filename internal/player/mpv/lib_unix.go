//go:build darwin || linux

package mpv

import (
	"os/exec"

	"github.com/ebitengine/purego"
)

func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

func closeLibrary(handle uintptr) {
	_ = purego.Dlclose(handle)
}

func librarySymbol(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}

func configureProbe(_ *exec.Cmd) {}
