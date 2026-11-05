package mpv

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ErrUnavailable is returned when libmpv cannot be loaded.
var ErrUnavailable = errors.New("mpv: library not found or cannot be loaded")

type cFuncs struct {
	create            func() uintptr
	initialize        func(handle uintptr) int
	terminateDestroy  func(handle uintptr)
	command           func(handle uintptr, cmd **byte) int
	commandString     func(handle uintptr, cmd *byte) int
	setOptionString   func(handle uintptr, name, value *byte) int
	setPropertyString func(handle uintptr, name, value *byte) int
	getPropertyString func(handle uintptr, name *byte) *byte
	free              func(ptr unsafe.Pointer)
	waitEvent         func(handle uintptr, timeout float64) *cEvent
	wakeup            func(handle uintptr)
	requestLogMessages func(handle uintptr, minLevel *byte) int
}

type cEvent struct {
	EventID       uint32
	Error         int32
	ReplyUserdata uint64
	Data          unsafe.Pointer
}

const (
	EventNone            uint32 = 0
	EventShutdown        uint32 = 1
	EventLogMessage      uint32 = 2
	EventStartFile       uint32 = 6
	EventEndFile         uint32 = 7
	EventFileLoaded      uint32 = 8
	EventPlaybackRestart uint32 = 21
	EventPropertyChange  uint32 = 22
)

// EndFileReason represents the reason an MPV track stopped or ended.
type EndFileReason int32

const (
	EndFileReasonEOF      EndFileReason = 0
	EndFileReasonStop     EndFileReason = 2
	EndFileReasonQuit     EndFileReason = 3
	EndFileReasonError    EndFileReason = 4
	EndFileReasonRedirect EndFileReason = 5
)

type cEventEndFile struct {
	Reason          int32
	Error           int32
	PlaylistEntryID int64
}

type cLogMessage struct {
	Prefix   *byte
	Level    *byte
	Text     *byte
	LogLevel uint32
}

var (
	loadMu sync.Mutex
	loaded bool
	mpvLib cFuncs
)

// Available reports whether libmpv has been successfully loaded.
func Available() bool {
	loadMu.Lock()
	defer loadMu.Unlock()
	return loaded
}

// Load attempts to load libmpv from the downloaded cache directory or host system paths.
func Load() error {
	loadMu.Lock()
	defer loadMu.Unlock()

	if loaded {
		return nil
	}

	// 1. Check downloaded library in cache
	if p, err := TargetFile(); err == nil {
		if err := loadLibraryAt(p); err == nil {
			loaded = true
			return nil
		}
	}

	// 2. Check system-installed candidate paths
	for _, name := range SystemLibNames() {
		if err := loadLibraryAt(name); err == nil {
			loaded = true
			return nil
		}
	}

	return ErrUnavailable
}

// LoadPath loads libmpv explicitly from the specified file path.
func LoadPath(libPath string) error {
	loadMu.Lock()
	defer loadMu.Unlock()

	if err := loadLibraryAt(libPath); err != nil {
		return err
	}
	loaded = true
	return nil
}

func loadLibraryAt(path string) error {
	h, err := openLibrary(path)
	if err != nil || h == 0 {
		return fmt.Errorf("open library %s: %w", path, err)
	}

	symbols := map[string]any{
		"mpv_create":                &mpvLib.create,
		"mpv_initialize":            &mpvLib.initialize,
		"mpv_terminate_destroy":     &mpvLib.terminateDestroy,
		"mpv_command":               &mpvLib.command,
		"mpv_command_string":        &mpvLib.commandString,
		"mpv_set_option_string":     &mpvLib.setOptionString,
		"mpv_set_property_string":   &mpvLib.setPropertyString,
		"mpv_get_property_string":   &mpvLib.getPropertyString,
		"mpv_free":                  &mpvLib.free,
		"mpv_wait_event":            &mpvLib.waitEvent,
		"mpv_wakeup":                &mpvLib.wakeup,
		"mpv_request_log_messages":  &mpvLib.requestLogMessages,
	}

	for symName, fnPtr := range symbols {
		sym, err := librarySymbol(h, symName)
		if err != nil || sym == 0 {
			return fmt.Errorf("missing symbol %s in %s", symName, path)
		}
		purego.RegisterFunc(fnPtr, sym)
	}

	return nil
}

// Instance wraps a running libmpv instance handle.
type Instance struct {
	mu     sync.Mutex
	handle uintptr
	closed bool
}

// NewInstance creates and configures a new MPV instance for audio playback.
func NewInstance(opts ...map[string]string) (*Instance, error) {
	if err := Load(); err != nil {
		return nil, err
	}

	handle := mpvLib.create()
	if handle == 0 {
		return nil, errors.New("mpv: failed to create instance")
	}

	inst := &Instance{handle: handle}

	// Default options: audio only, no video window, stay idle
	inst.SetOption("vo", "null")
	inst.SetOption("audio-display", "no")
	inst.SetOption("idle", "yes")
	inst.SetOption("audio-client-name", "MyGo 云音乐")
	inst.SetOption("prefetch-playlist", "yes")
	inst.SetOption("demuxer-max-bytes", "64MiB")
	inst.SetOption("demuxer-max-back-bytes", "32MiB")
	inst.SetOption("network-timeout", "30")

	if len(opts) > 0 {
		for k, v := range opts[0] {
			inst.SetOption(k, v)
		}
	}

	if code := mpvLib.initialize(handle); code < 0 {
		mpvLib.terminateDestroy(handle)
		return nil, fmt.Errorf("mpv: initialize failed with error code %d", code)
	}

	return inst, nil
}

// Close destroys the MPV instance.
func (inst *Instance) Close() {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.closed || inst.handle == 0 {
		return
	}
	inst.closed = true
	mpvLib.wakeup(inst.handle)
	mpvLib.terminateDestroy(inst.handle)
	inst.handle = 0
}

// SetOption sets an MPV option before or after initialization.
func (inst *Instance) SetOption(name, value string) int {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.handle == 0 {
		return -1
	}
	cName := cStr(name)
	cVal := cStr(value)
	return mpvLib.setOptionString(inst.handle, &cName[0], &cVal[0])
}

// RequestLogMessages configures MPV to emit log messages as events (e.g. "warn", "info", "v").
func (inst *Instance) RequestLogMessages(minLevel string) int {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.handle == 0 || mpvLib.requestLogMessages == nil {
		return -1
	}
	cLevel := cStr(minLevel)
	return mpvLib.requestLogMessages(inst.handle, &cLevel[0])
}

// SetProperty sets a runtime property as a string.
func (inst *Instance) SetProperty(name, value string) int {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.handle == 0 {
		return -1
	}
	cName := cStr(name)
	cVal := cStr(value)
	return mpvLib.setPropertyString(inst.handle, &cName[0], &cVal[0])
}

// GetProperty returns a property's string value, or "".
func (inst *Instance) GetProperty(name string) string {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.handle == 0 {
		return ""
	}
	cName := cStr(name)
	p := mpvLib.getPropertyString(inst.handle, &cName[0])
	if p == nil {
		return ""
	}
	defer mpvLib.free(unsafe.Pointer(p))
	return goStr(p)
}

// Command executes a command with arguments.
func (inst *Instance) Command(args []string) int {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.handle == 0 || len(args) == 0 {
		return -1
	}

	cStrings := make([][]byte, len(args))
	cPtrs := make([]*byte, len(args)+1)
	for i, arg := range args {
		cStrings[i] = cStr(arg)
		cPtrs[i] = &cStrings[i][0]
	}
	cPtrs[len(args)] = nil

	return mpvLib.command(inst.handle, &cPtrs[0])
}

// CommandString executes a space-separated or quoted command string.
func (inst *Instance) CommandString(cmd string) int {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.handle == 0 {
		return -1
	}
	cCmd := cStr(cmd)
	return mpvLib.commandString(inst.handle, &cCmd[0])
}

// WaitEvent blocks for up to timeout seconds waiting for an MPV event.
func (inst *Instance) WaitEvent(timeout float64) (eventID uint32, errCode int32, endReason EndFileReason) {
	inst.mu.Lock()
	h := inst.handle
	inst.mu.Unlock()

	if h == 0 {
		return EventShutdown, 0, 0
	}

	ev := mpvLib.waitEvent(h, timeout)
	if ev == nil {
		return EventNone, 0, 0
	}
	if ev.EventID == EventLogMessage && ev.Data != nil {
		msg := (*cLogMessage)(ev.Data)
		fmt.Printf("[mpv-log][%s] %s: %s", goStr(msg.Level), goStr(msg.Prefix), goStr(msg.Text))
	}
	if ev.EventID == EventEndFile && ev.Data != nil {
		ef := (*cEventEndFile)(ev.Data)
		return ev.EventID, ev.Error, EndFileReason(ef.Reason)
	}
	return ev.EventID, ev.Error, 0
}

func cStr(s string) []byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	b[len(s)] = 0
	return b
}

func goStr(p *byte) string {
	if p == nil {
		return ""
	}
	var n int
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

func parseSecs(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil || sec < 0 {
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}
