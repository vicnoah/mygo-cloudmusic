package mpv

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"slices"
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
	clientAPIVersion   func() uint32
	create             func() uintptr
	initialize         func(handle uintptr) int
	terminateDestroy   func(handle uintptr)
	command            func(handle uintptr, cmd **byte) int
	commandString      func(handle uintptr, cmd *byte) int
	setOptionString    func(handle uintptr, name, value *byte) int
	setPropertyString  func(handle uintptr, name, value *byte) int
	getPropertyString  func(handle uintptr, name *byte) *byte
	free               func(ptr unsafe.Pointer)
	waitEvent          func(handle uintptr, timeout float64) *cEvent
	wakeup             func(handle uintptr)
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

type componentLibrary struct {
	path  string
	files []componentFile
}

func (library componentLibrary) matches(other componentLibrary) bool {
	return library.path != "" && other.path != "" &&
		filepath.Base(library.path) == filepath.Base(other.path) &&
		slices.Equal(library.files, other.files)
}

var (
	loadMu       sync.Mutex
	installed    componentLibrary
	mapped       componentLibrary
	loadFailed   bool
	loadDisabled bool
	mpvLib       cFuncs
)

// Ready reports whether a verified component can be enabled in this process.
func Ready() bool {
	loadMu.Lock()
	defer loadMu.Unlock()
	return installed.path != "" && !loadFailed && !loadDisabled
}

// IsDownloaded reports whether a verified component is selected for this or the next process.
func IsDownloaded() bool {
	loadMu.Lock()
	defer loadMu.Unlock()
	return installed.path != ""
}

// NeedsRestart reports whether the installed payload differs from the mapped library.
func NeedsRestart() bool {
	loadMu.Lock()
	defer loadMu.Unlock()
	return installed.path != "" && mapped.path != "" && !installed.matches(mapped)
}

// activateInstalled restores the mapped decoder when a reinstall verifies the same files.
// Callers hold loadMu while publishing the new active record and updating readiness.
func activateInstalled(path string, manifest componentManifest) {
	installed = componentLibrary{path: path, files: manifest.Files}
	loadFailed = false
	if installed.matches(mapped) {
		loadDisabled = false
	}
}

// Load binds the official installed component and keeps that version mapped until exit.
func Load() error {
	loadMu.Lock()
	defer loadMu.Unlock()
	if installed.path == "" || loadDisabled {
		return ErrUnavailable
	}
	if mapped.path != "" {
		return nil
	}
	library, err := bindLibrary(installed.path)
	if err != nil {
		loadFailed = true
		return err
	}
	mpvLib, mapped = library, installed
	loadFailed = false
	return nil
}

// ProbePath initializes a component in the short-lived process started by the installer.
func ProbePath(path string, progress io.Writer) error {
	stage := func(name string) {
		fmt.Fprintln(progress, probeStagePrefix+name)
	}
	stage("加载动态库")
	library, err := bindLibrary(path)
	if err != nil {
		return err
	}
	handle, err := initializeInstance(library, map[string]string{"ao": "null"}, stage)
	if err != nil {
		return err
	}
	stage("关闭解码器")
	library.terminateDestroy(handle)
	return nil
}

func bindLibrary(path string) (cFuncs, error) {
	var library cFuncs
	handle, err := openLibrary(path)
	if err != nil {
		return library, fmt.Errorf("open library %s: %w", path, err)
	}

	symbols := map[string]any{
		"mpv_client_api_version":   &library.clientAPIVersion,
		"mpv_create":               &library.create,
		"mpv_initialize":           &library.initialize,
		"mpv_terminate_destroy":    &library.terminateDestroy,
		"mpv_command":              &library.command,
		"mpv_command_string":       &library.commandString,
		"mpv_set_option_string":    &library.setOptionString,
		"mpv_set_property_string":  &library.setPropertyString,
		"mpv_get_property_string":  &library.getPropertyString,
		"mpv_free":                 &library.free,
		"mpv_wait_event":           &library.waitEvent,
		"mpv_wakeup":               &library.wakeup,
		"mpv_request_log_messages": &library.requestLogMessages,
	}

	for symName, fnPtr := range symbols {
		sym, err := librarySymbol(handle, symName)
		if err != nil || sym == 0 {
			closeLibrary(handle)
			return cFuncs{}, fmt.Errorf("missing symbol %s in %s", symName, path)
		}
		purego.RegisterFunc(fnPtr, sym)
	}

	// The minimum client API is 1.108 (MPV 0.34). Versions through API 2 are supported.
	if version := library.clientAPIVersion(); version < 1<<16|108 || version>>16 > 2 {
		closeLibrary(handle)
		return cFuncs{}, fmt.Errorf("mpv: unsupported client API %d.%d", version>>16, version&0xffff)
	}
	return library, nil
}

// Instance wraps a running libmpv instance handle.
type Instance struct {
	mu      sync.Mutex
	handle  uintptr
	eventMu sync.Mutex // protects an event read until its native data has been copied
}

// NewInstance creates and configures a new MPV instance for audio playback.
func NewInstance(opts ...map[string]string) (*Instance, error) {
	if err := Load(); err != nil {
		return nil, err
	}

	var options map[string]string
	if len(opts) != 0 {
		options = opts[0]
	}
	handle, err := initializeInstance(mpvLib, options, nil)
	if err != nil {
		return nil, err
	}
	return &Instance{handle: handle}, nil
}

func initializeInstance(library cFuncs, options map[string]string, stage func(string)) (uintptr, error) {
	if stage != nil {
		stage("创建解码器")
	}
	handle := library.create()
	if handle == 0 {
		return 0, errors.New("mpv: failed to create instance")
	}

	setOption := func(name, value string) {
		cName, cValue := cStr(name), cStr(value)
		library.setOptionString(handle, &cName[0], &cValue[0])
	}

	// Default options: audio only, no video decoding, stay idle
	setOption("video", "no")
	setOption("idle", "yes")
	setOption("audio-client-name", "肥牛音乐")
	setOption("prefetch-playlist", "yes")

	// Demuxer buffering for network resilience
	setOption("demuxer-thread", "yes")
	setOption("demuxer-max-bytes", "64MiB")
	setOption("demuxer-max-back-bytes", "32MiB")
	setOption("cache-pause-initial", "no")
	setOption("cache-pause-wait", "1")
	setOption("network-timeout", "30")

	// Platform-specific scheduling and audio driver
	switch runtime.GOOS {
	case "windows":
		// Elevate thread priority to defend against foreground window CPU starvation
		setOption("priority", "high")
	case "linux":
		// Prefer modern PipeWire, fallback to PulseAudio and ALSA
		setOption("ao", "pipewire,pulse,alsa")
	}

	for k, v := range options {
		setOption(k, v)
	}

	if stage != nil {
		stage("初始化解码器")
	}
	if code := library.initialize(handle); code < 0 {
		library.terminateDestroy(handle)
		return 0, fmt.Errorf("mpv: initialize failed with error code %d", code)
	}

	return handle, nil
}

// Close destroys the MPV instance.
func (inst *Instance) Close() {
	inst.mu.Lock()
	h := inst.handle
	if h == 0 {
		inst.mu.Unlock()
		return
	}
	inst.handle = 0
	mpvLib.wakeup(h)
	inst.mu.Unlock()

	inst.eventMu.Lock()
	defer inst.eventMu.Unlock()
	mpvLib.terminateDestroy(h)
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
	inst.eventMu.Lock()
	defer inst.eventMu.Unlock()
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
