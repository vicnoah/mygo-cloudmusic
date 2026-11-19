package app

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
)

// MyGo 0.8.1 exposes a GtkWindow on Linux and leaves this operation to
// the app. Set the shape on the widget so it follows its GDK window.
var gtkDesktopLyricsInput = sync.OnceValue(func() func(uintptr, bool) {
	var setShape func(widget, region uintptr)
	var createRegion func() uintptr
	var destroyRegion, queueDraw func(uintptr)
	for _, symbol := range []struct {
		name string
		fn   any
	}{
		{"gtk_widget_input_shape_combine_region", &setShape},
		{"cairo_region_create", &createRegion},
		{"cairo_region_destroy", &destroyRegion},
		{"gtk_widget_queue_draw", &queueDraw},
	} {
		address, err := purego.Dlsym(purego.RTLD_DEFAULT, symbol.name)
		if err != nil {
			return nil
		}
		purego.RegisterFunc(symbol.fn, address)
	}
	return func(widget uintptr, ignore bool) {
		var region uintptr
		if ignore {
			region = createRegion()
			defer destroyRegion(region)
		}
		setShape(widget, region)
		queueDraw(widget)
	}
})

func setDesktopLyricsMouseIgnored(win *mygo.Window, ignore bool) {
	if widget := win.NativeHandle(); widget != 0 {
		if setInput := gtkDesktopLyricsInput(); setInput != nil {
			setInput(widget, ignore)
		}
	}
}
