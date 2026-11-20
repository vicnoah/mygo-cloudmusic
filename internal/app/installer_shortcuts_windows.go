package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shortcutOle32 = windows.NewLazySystemDLL("ole32.dll")

// PrepareInstallerShortcuts gives the application's shortcuts the same taskbar
// identity as the host and plugin windows.
func PrepareInstallerShortcuts() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	status, _, _ := shortcutOle32.NewProc("CoInitializeEx").Call(0, 2)
	if err := shortcutHRESULT(status); err != nil {
		return err
	}
	defer shortcutOle32.NewProc("CoUninitialize").Call()
	for _, folder := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Programs, windows.FOLDERID_Desktop} {
		dir, err := windows.KnownFolderPath(folder, windows.KF_FLAG_DEFAULT)
		if err != nil {
			return err
		}
		link := filepath.Join(dir, appName+".lnk")
		if _, err := os.Stat(link); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := setShortcutAppID(link, exe); err != nil {
			return err
		}
	}
	return nil
}

type shortcutCOM struct{ vtable *[32]uintptr }

//go:uintptrescapes
func (obj *shortcutCOM) call(method int, args ...uintptr) uintptr {
	result, _, _ := syscall.SyscallN(obj.vtable[method], append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)...)
	return result
}

func shortcutHRESULT(status uintptr) error {
	if int32(status) < 0 {
		return fmt.Errorf("Windows shortcut HRESULT %#x", uint32(status))
	}
	return nil
}

func setShortcutAppID(path, exe string) error {
	class := windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	linkIID := windows.GUID{Data1: 0x000214f9, Data4: class.Data4}
	persistIID := windows.GUID{Data1: 0x0000010b, Data4: class.Data4}
	storeIID := windows.GUID{Data1: 0x886d8eeb, Data2: 0x8cf2, Data3: 0x4446, Data4: [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}
	var link, persist, store *shortcutCOM
	status, _, _ := shortcutOle32.NewProc("CoCreateInstance").Call(
		uintptr(unsafe.Pointer(&class)), 0, 1, uintptr(unsafe.Pointer(&linkIID)), uintptr(unsafe.Pointer(&link)))
	if err := shortcutHRESULT(status); err != nil {
		return err
	}
	defer link.call(2)
	if err := shortcutHRESULT(link.call(0, uintptr(unsafe.Pointer(&persistIID)), uintptr(unsafe.Pointer(&persist)))); err != nil {
		return err
	}
	defer persist.call(2)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := shortcutHRESULT(persist.call(5, uintptr(unsafe.Pointer(name)), 2)); err != nil {
		return err
	}
	target := make([]uint16, 32768)
	if err := shortcutHRESULT(link.call(3, uintptr(unsafe.Pointer(&target[0])), uintptr(len(target)), 0, 4)); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(windows.UTF16ToString(target)), filepath.Clean(exe)) {
		return nil
	}
	if err := shortcutHRESULT(link.call(0, uintptr(unsafe.Pointer(&storeIID)), uintptr(unsafe.Pointer(&store)))); err != nil {
		return err
	}
	defer store.call(2)
	key := struct {
		Format windows.GUID
		ID     uint32
	}{windows.GUID{Data1: 0x9f4c2855, Data2: 0x9f79, Data3: 0x4b39, Data4: [8]byte{0xa8, 0xd0, 0xe1, 0xd4, 0x2d, 0xe1, 0xd5, 0xf3}}, 5}
	id, err := windows.UTF16PtrFromString(desktopApplicationID())
	if err != nil {
		return err
	}
	value := struct {
		Type     uint16
		Reserved [3]uint16
		Text     *uint16
		Padding  uintptr
	}{Type: 31, Text: id}
	original := value
	original.Type, original.Text = 0, nil
	if err := shortcutHRESULT(store.call(5, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&original)))); err == nil {
		matches := original.Type == value.Type && windows.UTF16PtrToString(original.Text) == desktopApplicationID()
		shortcutOle32.NewProc("PropVariantClear").Call(uintptr(unsafe.Pointer(&original)))
		if matches {
			return nil
		}
	}
	if err := shortcutHRESULT(store.call(6, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&value)))); err != nil {
		return err
	}
	if err := shortcutHRESULT(store.call(7)); err != nil {
		return err
	}
	err = shortcutHRESULT(persist.call(6, uintptr(unsafe.Pointer(name)), 1))
	runtime.KeepAlive(value)
	runtime.KeepAlive(name)
	return err
}
