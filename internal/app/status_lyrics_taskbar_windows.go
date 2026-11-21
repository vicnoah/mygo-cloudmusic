// Explorer taskbar placement and UI Automation are adapted from mygo-qyin (AGPL-3.0).
// https://github.com/Amamiya23/mygo-qyin/blob/81512dc/internal/app/taskbar_lyrics_windows.go

package app

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
)

var taskbarUser32 = syscall.NewLazyDLL("user32.dll")
var taskbarOle32 = syscall.NewLazyDLL("ole32.dll")
var taskbarOleaut32 = syscall.NewLazyDLL("oleaut32.dll")
var taskbarComctl32 = syscall.NewLazyDLL("comctl32.dll")

var taskbarContextMenuProc uintptr

func init() {
	taskbarContextMenuProc = syscall.NewCallback(taskbarContextMenuMessage)
}

func taskbarContextMenuMessage(hwnd, message, wp, lp, id, data uintptr) uintptr {
	if message == 0x007B { // WM_CONTEXTMENU
		// MyGo's surface has already delivered PointerUp. Do not let the
		// default child-window handling synchronously forward its menu to Explorer.
		return 0
	}
	if message == 0x0082 { // WM_NCDESTROY
		taskbarComctl32.NewProc("RemoveWindowSubclass").Call(hwnd, taskbarContextMenuProc, id)
	}
	r, _, _ := taskbarComctl32.NewProc("DefSubclassProc").Call(hwnd, message, wp, lp)
	return r
}

// Install on our own host, on its UI thread, before it becomes Explorer's child.
func guardTaskbarContextMenu(hwnd uintptr) error {
	ok, _, _ := taskbarComctl32.NewProc("SetWindowSubclass").Call(hwnd, taskbarContextMenuProc, 1, 0)
	if ok == 0 {
		return fmt.Errorf("cannot isolate taskbar context menu")
	}
	return nil
}

type taskbarNativeRect struct{ left, top, right, bottom int32 }
type taskbarGUID struct {
	a    uint32
	b, c uint16
	d    [8]byte
}

// Explorer UIA calls run on one dedicated MTA thread, never the UI thread.
func startTaskbarLyricsMonitor(deliver func(taskbarLyricsPlacement)) func() {
	stop := make(chan struct{})
	var stopped atomic.Bool
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		hr, _, _ := taskbarOle32.NewProc("CoInitializeEx").Call(0, 0)
		if int32(hr) < 0 {
			return
		}
		defer taskbarOle32.NewProc("CoUninitialize").Call()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			started := time.Now()
			p, err := readTaskbarLyricsPlacement()
			if err != nil || time.Since(started) > 2*time.Second {
				p = taskbarLyricsPlacement{}
			}
			if stopped.Load() {
				return
			}
			mygo.RunOnMain(func() {
				if !stopped.Load() {
					deliver(p)
				}
			})
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		if stopped.CompareAndSwap(false, true) {
			close(stop)
		}
	}
}

func taskbarFindChild(parent uintptr, name string) uintptr {
	text, _ := syscall.UTF16PtrFromString(name)
	h, _, _ := taskbarUser32.NewProc("FindWindowExW").Call(parent, 0, uintptr(unsafe.Pointer(text)), 0)
	return h
}
func taskbarRect(hwnd uintptr) (taskbarNativeRect, bool) {
	var r taskbarNativeRect
	ok, _, _ := taskbarUser32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r, ok != 0 && r.right > r.left && r.bottom > r.top
}

func readTaskbarLyricsPlacement() (taskbarLyricsPlacement, error) {
	class, _ := syscall.UTF16PtrFromString("Shell_TrayWnd")
	root, _, _ := taskbarUser32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), 0)
	if root == 0 {
		return taskbarLyricsPlacement{}, fmt.Errorf("Explorer taskbar unavailable")
	}
	visible, _, _ := taskbarUser32.NewProc("IsWindowVisible").Call(root)
	rootRect, hasRootRect := taskbarRect(root)
	monitor, _, _ := taskbarUser32.NewProc("MonitorFromWindow").Call(root, 2)
	var info struct {
		size          uint32
		monitor, work taskbarNativeRect
		flags         uint32
	}
	info.size = uint32(unsafe.Sizeof(info))
	hasMonitor, _, _ := taskbarUser32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info)))
	if visible == 0 || !hasRootRect || hasMonitor == 0 || max(int32(0), min(rootRect.bottom, info.monitor.bottom)-max(rootRect.top, info.monitor.top))*2 < rootRect.bottom-rootRect.top {
		return taskbarLyricsPlacement{}, fmt.Errorf("taskbar is hidden or collapsed")
	}
	var version struct {
		size, major, minor, build, platform uint32
		csd                                 [128]uint16
	}
	version.size = uint32(unsafe.Sizeof(version))
	status, _, _ := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&version)))
	if status != 0 {
		return taskbarLyricsPlacement{}, fmt.Errorf("cannot identify taskbar version")
	}
	parent := root
	if version.build < 22000 {
		parent = taskbarFindChild(root, "ReBarWindow32")
		if parent == 0 {
			parent = taskbarFindChild(root, "WorkerW")
		}
		if parent == 0 {
			return taskbarLyricsPlacement{}, fmt.Errorf("classic taskbar container unavailable")
		}
	}
	rect, ok := taskbarRect(parent)
	if !ok || rect.right-rect.left <= rect.bottom-rect.top {
		return taskbarLyricsPlacement{}, fmt.Errorf("horizontal taskbar unavailable")
	}
	var client taskbarNativeRect
	okay, _, _ := taskbarUser32.NewProc("GetClientRect").Call(parent, uintptr(unsafe.Pointer(&client)))
	if okay == 0 || client.right <= 0 || client.bottom <= 0 {
		return taskbarLyricsPlacement{}, fmt.Errorf("taskbar has no client area")
	}
	dpi, _, _ := taskbarUser32.NewProc("GetDpiForWindow").Call(parent)
	if dpi == 0 {
		dpi = 96
	}
	occupied, err := taskbarOccupiedBounds(root, rect)
	if err != nil {
		return taskbarLyricsPlacement{}, err
	}
	rightAligned := taskbarUsesLeftAlignment(version.build)
	x, width := taskbarLyricsPlacementSpace(int(client.right), 280*int(dpi)/96, 100*int(dpi)/96, taskbarLyricsEdgeGap*int(dpi)/96, occupied, rightAligned)
	if width == 0 {
		return taskbarLyricsPlacement{}, fmt.Errorf("taskbar has insufficient free space")
	}
	height := min(int(client.bottom), 48*int(dpi)/96)
	return taskbarLyricsPlacement{parent: parent, x: x, y: (int(client.bottom) - height) / 2, width: width, height: height, dpi: uint32(dpi), dark: taskbarUsesDarkTheme()}, nil
}

// Windows 10 taskbars are always left-aligned; Windows 11 records left
// alignment in Explorer\Advanced\TaskbarAl = 0 and defaults to centered.
func taskbarUsesLeftAlignment(build uint32) bool {
	if build < 22000 {
		return true
	}
	path, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`)
	name, _ := syscall.UTF16PtrFromString("TaskbarAl")
	var value uint32
	size := uint32(4)
	key := int32(-2147483647)
	r, _, _ := syscall.NewLazyDLL("advapi32.dll").NewProc("RegGetValueW").Call(uintptr(key), uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(name)), 0x10, 0, uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&size)))
	return r == 0 && value == 0
}

func taskbarUsesDarkTheme() bool {
	path, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`)
	name, _ := syscall.UTF16PtrFromString("SystemUsesLightTheme")
	var value uint32
	size := uint32(4)
	key := int32(-2147483647)
	r, _, _ := syscall.NewLazyDLL("advapi32.dll").NewProc("RegGetValueW").Call(uintptr(key), uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(name)), 0x10, 0, uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&size)))
	return r != 0 || value == 0
}
func taskbarGetStyle(hwnd uintptr, index int32) uintptr {
	v, _, _ := taskbarUser32.NewProc("GetWindowLongPtrW").Call(hwnd, uintptr(index))
	return v
}
func taskbarSetStyle(hwnd uintptr, index int32, value uintptr) {
	taskbarUser32.NewProc("SetWindowLongPtrW").Call(hwnd, uintptr(index), value)
}
func taskbarAwareness(hwnd uintptr) uintptr {
	context, _, _ := taskbarUser32.NewProc("GetWindowDpiAwarenessContext").Call(hwnd)
	awareness, _, _ := taskbarUser32.NewProc("GetAwarenessFromDpiAwarenessContext").Call(context)
	return awareness
}

func placeTaskbarLyricsWindow(w *mygo.Window, p taskbarLyricsPlacement) error {
	hwnd := w.NativeHandle()
	valid, _, _ := taskbarUser32.NewProc("IsWindow").Call(p.parent)
	if hwnd == 0 || valid == 0 || p.width <= 0 || p.height <= 0 {
		return fmt.Errorf("taskbar placement unavailable")
	}
	// A cross-process DPI mismatch can reset the entire child's process awareness.
	if taskbarAwareness(hwnd) != taskbarAwareness(p.parent) {
		return fmt.Errorf("taskbar DPI awareness differs from application")
	}
	parent, _, _ := taskbarUser32.NewProc("GetAncestor").Call(hwnd, 1)
	attaching := parent != p.parent
	if attaching {
		if err := guardTaskbarContextMenu(hwnd); err != nil {
			return err
		}
		taskbarUser32.NewProc("SetWindowPos").Call(hwnd, ^uintptr(1), 0, 0, 0, 0, 0x1|0x2|0x10)
		taskbarUser32.NewProc("SetParent").Call(hwnd, p.parent)
		parent, _, _ = taskbarUser32.NewProc("GetAncestor").Call(hwnd, 1)
		if parent != p.parent {
			return fmt.Errorf("Explorer rejected lyrics child")
		}
		taskbarSetStyle(hwnd, -16, (taskbarGetStyle(hwnd, -16)&^0x80000000)|0x40000000)
		// Preserve the layered, click-through style when attaching the child.
		taskbarSetStyle(hwnd, -20, (taskbarGetStyle(hwnd, -20)&^(0x80|0x8))|0x08000000)
		styleStatusLyricsWindow(w)
	}
	// The framework can update bounds after a DPI message. Compare the live
	// native rectangle, in parent-client coordinates, rather than saved DIPs.
	current, haveBounds := taskbarRect(hwnd)
	var origin struct{ x, y int32 }
	taskbarUser32.NewProc("ClientToScreen").Call(p.parent, uintptr(unsafe.Pointer(&origin)))
	changed := !haveBounds || int(current.left-origin.x) != p.x || int(current.top-origin.y) != p.y || int(current.right-current.left) != p.width || int(current.bottom-current.top) != p.height
	flags := uintptr(0x10) // SWP_NOACTIVATE
	if attaching {
		flags |= 0x20
	} // SWP_FRAMECHANGED only after parenting
	if !changed && !attaching {
		// Explorer may insert a XAML host in front of an unchanged child. Repair
		// sibling order without resizing the surface or requesting another frame.
		previous, _, _ := taskbarUser32.NewProc("GetWindow").Call(hwnd, 3)
		if previous == 0 {
			return nil
		}
		var previousProcess uint32
		taskbarUser32.NewProc("GetWindowThreadProcessId").Call(previous, uintptr(unsafe.Pointer(&previousProcess)))
		// Avoid raising the lyrics over another window in this process.
		if previousProcess == uint32(os.Getpid()) {
			return nil
		}
		flags |= 0x1 | 0x2 // SWP_NOSIZE | SWP_NOMOVE
	}
	ok, _, err := taskbarUser32.NewProc("SetWindowPos").Call(hwnd, 0, uintptr(p.x), uintptr(p.y), uintptr(p.width), uintptr(p.height), flags)
	if ok == 0 {
		return fmt.Errorf("position taskbar lyrics: %w", err)
	}
	return nil
}
func detachTaskbarLyricsWindow(w *mygo.Window) {
	hwnd := w.NativeHandle()
	var process uint32
	thread, _, _ := taskbarUser32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&process)))
	if thread == 0 || process != uint32(os.Getpid()) {
		return
	}
	taskbarUser32.NewProc("SetParent").Call(hwnd, 0)
	taskbarComctl32.NewProc("RemoveWindowSubclass").Call(hwnd, taskbarContextMenuProc, 1)
	taskbarSetStyle(hwnd, -16, (taskbarGetStyle(hwnd, -16)&^0x40000000)|0x80000000)
	taskbarSetStyle(hwnd, -20, taskbarGetStyle(hwnd, -20)|0x80|0x08000000)
}

// Windows SDK UIAutomationClient.h ABI slots; all references stay on the MTA.
type taskbarCOMObject struct{ vtable *[64]uintptr }
type taskbarCOMReferences []*taskbarCOMObject

//go:uintptrescapes
func taskbarCOMCall(object *taskbarCOMObject, slot int, args ...uintptr) (uintptr, uintptr, syscall.Errno) {
	return syscall.SyscallN(object.vtable[slot], append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)...)
}
func (refs *taskbarCOMReferences) own(object *taskbarCOMObject, hr uintptr) (*taskbarCOMObject, error) {
	if int32(hr) < 0 || object == nil {
		return nil, fmt.Errorf("taskbar UI Automation HRESULT 0x%X", uint32(hr))
	}
	*refs = append(*refs, object)
	return object, nil
}
func (refs *taskbarCOMReferences) release() {
	for i := len(*refs) - 1; i >= 0; i-- {
		taskbarCOMCall((*refs)[i], 2)
	}
}

// VARIANT includes its DECIMAL union, 24 bytes on Windows amd64.
type taskbarVariant struct {
	kind     uint16
	reserved [3]uint16
	value    [2]uint64
}

func taskbarProperty(object *taskbarCOMObject, id int) (taskbarVariant, error) {
	var value taskbarVariant
	hr, _, _ := taskbarCOMCall(object, 10, uintptr(id), uintptr(unsafe.Pointer(&value)))
	if int32(hr) < 0 {
		return value, fmt.Errorf("read taskbar automation property %d: 0x%X", id, uint32(hr))
	}
	return value, nil
}
func taskbarClearVariant(v *taskbarVariant) {
	taskbarOleaut32.NewProc("VariantClear").Call(uintptr(unsafe.Pointer(v)))
}
func taskbarCondition(refs *taskbarCOMReferences, automation *taskbarCOMObject, id int, kind uint16, value uint64) (*taskbarCOMObject, error) {
	v := taskbarVariant{kind: kind, value: [2]uint64{value}}
	var result *taskbarCOMObject
	hr, _, _ := taskbarCOMCall(automation, 23, uintptr(id), uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&result)))
	return refs.own(result, hr)
}
func taskbarJoinCondition(refs *taskbarCOMReferences, automation *taskbarCOMObject, slot int, a, b *taskbarCOMObject) (*taskbarCOMObject, error) {
	var result *taskbarCOMObject
	hr, _, _ := taskbarCOMCall(automation, slot, uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(&result)))
	return refs.own(result, hr)
}
func taskbarFindAll(refs *taskbarCOMReferences, element *taskbarCOMObject, scope int, condition *taskbarCOMObject) ([]*taskbarCOMObject, error) {
	var array *taskbarCOMObject
	hr, _, _ := taskbarCOMCall(element, 6, uintptr(scope), uintptr(unsafe.Pointer(condition)), uintptr(unsafe.Pointer(&array)))
	if _, err := refs.own(array, hr); err != nil {
		return nil, err
	}
	var length int32
	hr, _, _ = taskbarCOMCall(array, 3, uintptr(unsafe.Pointer(&length)))
	if int32(hr) < 0 || length < 0 {
		return nil, fmt.Errorf("invalid taskbar automation array")
	}
	result := make([]*taskbarCOMObject, 0, int(length))
	for i := int32(0); i < length; i++ {
		var item *taskbarCOMObject
		hr, _, _ = taskbarCOMCall(array, 4, uintptr(i), uintptr(unsafe.Pointer(&item)))
		if _, err := refs.own(item, hr); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func taskbarOccupiedBounds(taskbar uintptr, bounds taskbarNativeRect) ([]taskbarLyricsInterval, error) {
	var refs taskbarCOMReferences
	defer refs.release()
	clsid := taskbarGUID{0xFF48DBA4, 0x60EF, 0x4201, [8]byte{0xAA, 0x87, 0x54, 0x10, 0x3E, 0xEF, 0x59, 0x4E}}
	iid := taskbarGUID{0x30CBE57D, 0xD9D0, 0x452A, [8]byte{0xAB, 0x13, 0x7A, 0xC5, 0xAC, 0x48, 0x25, 0xEE}}
	var automation *taskbarCOMObject
	hr, _, _ := taskbarOle32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&automation)))
	if _, err := refs.own(automation, hr); err != nil {
		return nil, err
	}
	var root *taskbarCOMObject
	hr, _, _ = taskbarCOMCall(automation, 6, taskbar, uintptr(unsafe.Pointer(&root)))
	if _, err := refs.own(root, hr); err != nil {
		return nil, err
	}
	var process uint32
	taskbarUser32.NewProc("GetWindowThreadProcessId").Call(taskbar, uintptr(unsafe.Pointer(&process)))
	if process == 0 {
		return nil, fmt.Errorf("taskbar process unavailable")
	}
	explorer, err := taskbarCondition(&refs, automation, 30002, 3, uint64(process))
	if err != nil {
		return nil, err
	}
	visible, err := taskbarCondition(&refs, automation, 30022, 11, 0)
	if err != nil {
		return nil, err
	}
	var interactive *taskbarCOMObject
	for _, kind := range []uint64{50000, 50002, 50013, 50004, 50007} {
		condition, e := taskbarCondition(&refs, automation, 30003, 3, kind)
		if e != nil {
			return nil, e
		}
		if interactive == nil {
			interactive = condition
		} else {
			interactive, e = taskbarJoinCondition(&refs, automation, 28, interactive, condition)
			if e != nil {
				return nil, e
			}
		}
	}
	controls, err := taskbarJoinCondition(&refs, automation, 25, interactive, visible)
	if err != nil {
		return nil, err
	}
	controls, err = taskbarJoinCondition(&refs, automation, 25, controls, explorer)
	if err != nil {
		return nil, err
	}
	hosts, err := taskbarFindAll(&refs, root, 2, explorer)
	if err != nil {
		return nil, err
	}
	var occupied []taskbarLyricsInterval
	add := func(element *taskbarCOMObject) error {
		value, e := taskbarProperty(element, 30001)
		if e != nil {
			return e
		}
		defer taskbarClearVariant(&value)
		if value.kind != 0x2005 || value.value[0] == 0 {
			return nil
		}
		array := uintptr(value.value[0])
		dims, _, _ := taskbarOleaut32.NewProc("SafeArrayGetDim").Call(array)
		var lo, hi int32
		hr, _, _ := taskbarOleaut32.NewProc("SafeArrayGetLBound").Call(array, 1, uintptr(unsafe.Pointer(&lo)))
		if int32(hr) < 0 || dims != 1 {
			return fmt.Errorf("invalid taskbar rectangle")
		}
		hr, _, _ = taskbarOleaut32.NewProc("SafeArrayGetUBound").Call(array, 1, uintptr(unsafe.Pointer(&hi)))
		if int32(hr) < 0 || hi-lo != 3 {
			return fmt.Errorf("invalid taskbar rectangle length")
		}
		var coordinates [4]float64
		for i := range coordinates {
			index := lo + int32(i)
			hr, _, _ = taskbarOleaut32.NewProc("SafeArrayGetElement").Call(array, uintptr(unsafe.Pointer(&index)), uintptr(unsafe.Pointer(&coordinates[i])))
			if int32(hr) < 0 {
				return fmt.Errorf("unreadable taskbar rectangle")
			}
		}
		for _, n := range coordinates {
			if math.IsNaN(n) || math.IsInf(n, 0) {
				return fmt.Errorf("nonfinite taskbar rectangle")
			}
		}
		x, y, width, height := coordinates[0], coordinates[1], coordinates[2], coordinates[3]
		if width > 0 && height > 0 && x < float64(bounds.right) && x+width > float64(bounds.left) && y < float64(bounds.bottom) && y+height > float64(bounds.top) {
			occupied = append(occupied, taskbarLyricsInterval{int(math.Floor(x)) - int(bounds.left), int(math.Ceil(x+width)) - int(bounds.left)})
		}
		return nil
	}
	for _, host := range hosts {
		kind, e := taskbarProperty(host, 30003)
		if e != nil {
			return nil, e
		}
		controlType := uint32(kind.value[0]) // VT_I4 only defines the low 32 bits.
		taskbarClearVariant(&kind)
		if controlType == 50000 || controlType == 50002 || controlType == 50013 || controlType == 50004 || controlType == 50007 {
			offscreen, e := taskbarProperty(host, 30022)
			if e != nil {
				return nil, e
			}
			isVisible := offscreen.kind == 11 && offscreen.value[0]&0xffff == 0
			taskbarClearVariant(&offscreen)
			if isVisible {
				if e = add(host); e != nil {
					return nil, e
				}
			}
		}
		children, e := taskbarFindAll(&refs, host, 4, controls)
		if e != nil {
			return nil, e
		}
		for _, child := range children {
			if e = add(child); e != nil {
				return nil, e
			}
		}
	}
	if len(occupied) == 0 {
		return nil, fmt.Errorf("taskbar automation tree has no occupied controls")
	}
	occupied = append(occupied, taskbarForeignOccupiedBounds(taskbar, bounds, process)...)
	return occupied, nil
}

// Other applications may already embed a lyrics/tool window. Their native
// rectangles are occupancy too; do not enter their UIA trees or count ourselves.
func taskbarForeignOccupiedBounds(taskbar uintptr, bounds taskbarNativeRect, explorer uint32) []taskbarLyricsInterval {
	var occupied []taskbarLyricsInterval
	parents := []uintptr{taskbar}
	currentProcess := uint32(os.Getpid())
	for len(parents) > 0 {
		parent := parents[len(parents)-1]
		parents = parents[:len(parents)-1]
		child, _, _ := taskbarUser32.NewProc("GetWindow").Call(parent, 5) // GW_CHILD
		for child != 0 {
			parents = append(parents, child)
			var process uint32
			taskbarUser32.NewProc("GetWindowThreadProcessId").Call(child, uintptr(unsafe.Pointer(&process)))
			visible, _, _ := taskbarUser32.NewProc("IsWindowVisible").Call(child)
			if visible != 0 && process != 0 && process != explorer && process != currentProcess {
				if rect, ok := taskbarRect(child); ok && rect.left < bounds.right && rect.right > bounds.left && rect.top < bounds.bottom && rect.bottom > bounds.top {
					occupied = append(occupied, taskbarLyricsInterval{int(rect.left - bounds.left), int(rect.right - bounds.left)})
				}
			}
			child, _, _ = taskbarUser32.NewProc("GetWindow").Call(child, 2) // GW_HWNDNEXT
		}
	}
	return occupied
}
