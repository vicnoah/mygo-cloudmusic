package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/egoist/mygo"
	"golang.org/x/image/draw"

	"waygu-music/resources"
)

// Both the host and plugin windows use the installed application's identity.
func desktopApplicationID() string {
	switch runtime.GOOS {
	case "windows":
		return "Egoist.WagyuMusic"
	case "linux":
		if path, err := os.Executable(); err == nil {
			return filepath.Base(path)
		}
	case "darwin":
		return "dev.egoist.cloudmusic"
	}
	return ""
}

var desktopApplicationIcon = sync.OnceValue(func() []byte {
	art := resources.Icon
	if mygo.IsDev() {
		art = resources.DevelopmentIcon
	}
	original, err := png.Decode(bytes.NewReader(art))
	if err != nil {
		return nil
	}
	// The 1024px bundle icon exceeds the X11 window-property request limit.
	window := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	draw.CatmullRom.Scale(window, window.Bounds(), original, original.Bounds(), draw.Src, nil)
	var pngIcon bytes.Buffer
	if err := png.Encode(&pngIcon, window); err != nil {
		return nil
	}
	return pngIcon.Bytes()
})
