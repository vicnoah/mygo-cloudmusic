//go:build linux

package mpv

import (
	"errors"

	"github.com/ebitengine/purego"
)

const lcNumeric = 1 // Linux libc's LC_NUMERIC category.

// PrepareRuntime sets MPV's numeric locale after the desktop toolkit initializes it.
func PrepareRuntime() error {
	symbol, err := purego.Dlsym(purego.RTLD_DEFAULT, "setlocale")
	if err != nil {
		return err
	}
	var setLocale func(category int32, locale string) uintptr
	purego.RegisterFunc(&setLocale, symbol)
	if setLocale(lcNumeric, "C") == 0 {
		return errors.New("mpv: cannot set LC_NUMERIC to C")
	}
	return nil
}
