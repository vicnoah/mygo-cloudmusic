package app

import (
	"log"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// statusLyricsNative owns the read-only lyric window in the Explorer taskbar.
type statusLyricsNative struct {
	win       *mygo.Window
	monitor   func()
	placement taskbarLyricsPlacement
	frame     statusLyricsFrame
	position  func() time.Duration
	ready     bool
	lastError string
}

func (s *statusLyricsNative) update(enabled bool, frame statusLyricsFrame, position func() time.Duration) {
	if !enabled {
		if s.monitor != nil {
			s.monitor()
			s.monitor = nil
		}
		s.destroyWindow()
		s.frame, s.placement = statusLyricsFrame{}, taskbarLyricsPlacement{}
		s.position = nil
		return
	}
	s.position = position
	if s.monitor == nil {
		s.monitor = startTaskbarLyricsMonitor(s.setPlacement)
	}
	if s.frame == frame {
		return
	}
	s.frame = frame
	if s.win != nil && !s.win.IsDestroyed() {
		s.win.Invalidate()
	}
	s.present()
}

func (s *statusLyricsNative) setPlacement(p taskbarLyricsPlacement) {
	old := s.placement
	if p.parent != 0 && p.parent != old.parent {
		s.destroyWindow()
	}
	s.placement = p
	if s.win != nil && old.dark != p.dark {
		s.win.Invalidate()
	}
	s.present()
}

func (s *statusLyricsNative) present() {
	if (s.frame.statusLyricsLine == (statusLyricsLine{}) && s.frame.cover == nil) || s.placement.parent == 0 {
		if s.win != nil && !s.win.IsDestroyed() && s.win.IsVisible() {
			s.win.Hide()
		}
		return
	}
	if s.win == nil || s.win.IsDestroyed() {
		s.openWindow()
		return
	}
	if !s.ready {
		return
	}
	if err := placeTaskbarLyricsWindow(s.win, s.placement); err != nil {
		s.report(err)
		s.destroyWindow()
		return
	}
	s.lastError = ""
	if !s.win.IsVisible() {
		s.win.ShowInactive()
	}
}

func (s *statusLyricsNative) openWindow() {
	p := s.placement
	dpi := int(max(p.dpi, 96))
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: "任务栏歌词", Panel: true, Hidden: true, Frameless: true,
		Transparent: true, DisableShadow: true, DisableResize: true,
		Vibrancy: mygo.VibrancyGlassClear,
		Width:    max(1, p.width*96/dpi), Height: max(1, p.height*96/dpi),
		Content: ui.View(func(c *ui.Context) {
			frame := s.frame
			if frame.playing {
				frame.position = s.position()
			}
			taskbarLyricsView(c, frame, s.placement.dark)
		}),
	})
	s.win, s.ready = w, false
	w.SetVibrancy(mygo.VibrancyNone)
	w.SetIgnoreMouseEvents(true)
	if err := placeTaskbarLyricsWindow(w, p); err != nil {
		s.report(err)
		s.destroyWindow()
		return
	}
	w.OnReadyToShow(func() {
		if s.win == w {
			s.ready = true
			s.present()
		}
	})
	w.OnClosed(func() {
		if s.win == w {
			s.win, s.ready = nil, false
		}
	})
}

func (s *statusLyricsNative) destroyWindow() {
	w := s.win
	s.win, s.ready = nil, false
	if w != nil && !w.IsDestroyed() {
		w.Hide()
		detachTaskbarLyricsWindow(w)
		w.Destroy()
	}
}

func (s *statusLyricsNative) report(err error) {
	if s.lastError != err.Error() {
		s.lastError = err.Error()
		log.Print("status lyrics: ", err)
	}
}

func styleStatusLyricsWindow(w *mygo.Window) {
	hwnd := w.NativeHandle()
	// The window remains transparent and never takes activation from Explorer.
	taskbarSetStyle(hwnd, -20, taskbarGetStyle(hwnd, -20)|0x08000000)
	dwmSetAttribute := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if dwmSetAttribute.Find() == nil {
		noBorder, square := uint32(0xFFFFFFFE), uint32(1)
		dwmSetAttribute.Call(hwnd, 34, uintptr(unsafe.Pointer(&noBorder)), 4)
		dwmSetAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&square)), 4)
	}
	const frameChanged = 0x20
	const noMoveSizeZOrderActivate = 0x2 | 0x1 | 0x4 | 0x10
	taskbarUser32.NewProc("SetWindowPos").Call(hwnd, 0, 0, 0, 0, 0, frameChanged|noMoveSizeZOrderActivate)
}
