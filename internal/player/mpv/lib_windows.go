//go:build windows

package mpv

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func openLibrary(path string) (uintptr, error) {
	handle, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	return uintptr(handle), err
}

func closeLibrary(handle uintptr) {
	_ = windows.FreeLibrary(windows.Handle(handle))
}

func librarySymbol(handle uintptr, name string) (uintptr, error) {
	return windows.GetProcAddress(windows.Handle(handle), name)
}

func configureProbe(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
