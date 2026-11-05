package dlna

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/supersonic-app/go-upnpcast/services/avtransport"
)

// TrackMeta contains track details required to build rich DIDL-Lite metadata for DLNA devices.
type TrackMeta struct {
	ID              string
	Title           string
	Artist          string
	Album           string
	CoverURL        string
	CoverData       []byte // Raw cover bytes (for custom schemes like fnmusic:// or pre-resolved covers)
	StreamURL       string
	LocalFile       string
	Format          string
	Duration        time.Duration
	Size            int64
	Bitrate         int // bytes/sec (from size/duration or kbps*125)
	SampleFrequency int // e.g. 44100, 48000, 96000
	BitsPerSample   int // e.g. 16, 24
	Channels        int // e.g. 2
	Headers         http.Header
}

// Controller manages DLNA media discovery, streaming proxy, and remote device playback.
type Controller struct {
	mu           sync.RWMutex
	proxy        *Proxy
	knownDevices map[string]*Device // Keyed by device URL / ID to prevent flapping
	devices      []*Device          // Stable list presented to the UI
	activeDevice *Device

	playing      bool
	playStart    time.Time
	lastSyncPos  time.Duration
	lastSyncTime time.Time
	position     time.Duration
	duration     time.Duration
	volume       float64 // 0..1

	volMu      sync.Mutex
	pendingVol float64
	volSeq     int64

	cancelPoll context.CancelFunc

	onStatusChange func()
	onEnd          func()
}

// NewController creates a DLNA casting controller with its own local streaming proxy.
func NewController(onStatusChange func()) (*Controller, error) {
	proxy, err := NewProxy()
	if err != nil {
		return nil, fmt.Errorf("dlna controller: init proxy: %w", err)
	}

	c := &Controller{
		proxy:          proxy,
		knownDevices:   make(map[string]*Device),
		volume:         0.8,
		onStatusChange: onStatusChange,
	}

	return c, nil
}

// Proxy returns the local streaming proxy.
func (c *Controller) Proxy() *Proxy {
	if c == nil {
		return nil
	}
	return c.proxy
}

// OnEnd registers a callback for when remote track playback finishes.
func (c *Controller) OnEnd(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEnd = fn
}

// RefreshDevices discovers reachable DLNA MediaRenderer devices on the LAN and merges them.
func (c *Controller) RefreshDevices(ctx context.Context) ([]*Device, error) {
	devs, err := Discover(ctx, 2)
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.knownDevices == nil {
		c.knownDevices = make(map[string]*Device)
	}

	now := time.Now()
	for _, d := range devs {
		if d == nil || d.ID == "" {
			continue
		}
		if existing, ok := c.knownDevices[d.ID]; ok {
			existing.URL = d.URL
			existing.FriendlyName = d.FriendlyName
			existing.ModelName = d.ModelName
			existing.mr = d.mr
			existing.av = d.av
			existing.rc = d.rc
			existing.LastSeen = now
		} else {
			d.LastSeen = now
			c.knownDevices[d.ID] = d
		}
	}

	// Under no circumstances should the active device ever disappear from the list
	if c.activeDevice != nil {
		c.knownDevices[c.activeDevice.ID] = c.activeDevice
	}

	c.rebuildDeviceListLocked()
	return c.devices, err
}

func (c *Controller) rebuildDeviceListLocked() {
	var others []*Device
	var active *Device

	for _, dev := range c.knownDevices {
		if c.activeDevice != nil && (dev.ID == c.activeDevice.ID || dev.FriendlyName == c.activeDevice.FriendlyName) {
			active = dev
		} else {
			others = append(others, dev)
		}
	}

	sort.Slice(others, func(i, j int) bool {
		return others[i].FriendlyName < others[j].FriendlyName
	})

	var list []*Device
	if active != nil {
		list = append(list, active)
	} else if c.activeDevice != nil {
		list = append(list, c.activeDevice)
	}
	list = append(list, others...)
	c.devices = list
}

// Devices returns the known devices, keeping the list stable across discovery cycles.
func (c *Controller) Devices() []*Device {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]*Device, len(c.devices))
	copy(res, c.devices)
	return res
}

// AddDevice registers or updates a device in the known device collection.
func (c *Controller) AddDevice(d *Device) {
	if d == nil || d.ID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.knownDevices == nil {
		c.knownDevices = make(map[string]*Device)
	}
	d.LastSeen = time.Now()
	c.knownDevices[d.ID] = d
	c.rebuildDeviceListLocked()
}

// ActiveDevice returns the currently connected casting device, or nil if playing locally.
func (c *Controller) ActiveDevice() *Device {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeDevice
}

// SelectDevice connects to a target device (or disconnects if target is nil/empty).
func (c *Controller) SelectDevice(deviceID string) error {
	c.mu.Lock()
	if c.cancelPoll != nil {
		c.cancelPoll()
		c.cancelPoll = nil
	}

	oldDev := c.activeDevice
	if deviceID == "" {
		if oldDev != nil {
			go func(d *Device) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = d.Stop(ctx)
			}(oldDev)
		}
		c.activeDevice = nil
		c.playing = false
		c.rebuildDeviceListLocked()
		c.mu.Unlock()
		c.notify()
		return nil
	}

	var found *Device
	// 1. Search knownDevices
	for _, d := range c.knownDevices {
		if d.ID == deviceID || d.FriendlyName == deviceID {
			found = d
			break
		}
	}
	// 2. Fallback to devices slice
	if found == nil {
		for _, d := range c.devices {
			if d.ID == deviceID || d.FriendlyName == deviceID {
				found = d
				break
			}
		}
	}

	if found == nil {
		c.mu.Unlock()
		return fmt.Errorf("dlna: device %q not found", deviceID)
	}

	// If switching between two different DLNA devices, stop the previous one first
	if oldDev != nil && oldDev.ID != found.ID {
		go func(d *Device) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = d.Stop(ctx)
		}(oldDev)
	}

	c.activeDevice = found
	if c.knownDevices == nil {
		c.knownDevices = make(map[string]*Device)
	}
	c.knownDevices[found.ID] = found
	c.rebuildDeviceListLocked()
	c.mu.Unlock()
	c.notify()
	return nil
}

// PlayTrack streams a track to the active DLNA device via the local proxy with rich DIDL-Lite metadata.
func (c *Controller) PlayTrack(ctx context.Context, meta TrackMeta, startPos time.Duration) error {
	c.mu.Lock()
	dev := c.activeDevice
	if dev == nil {
		c.mu.Unlock()
		return fmt.Errorf("dlna: no active device")
	}

	if c.cancelPoll != nil {
		c.cancelPoll()
		c.cancelPoll = nil
	}

	mime := FormatToMIME(meta.Format)
	c.proxy.Register(meta.ID, StreamSource{
		RemoteURL:     meta.StreamURL,
		Headers:       meta.Headers,
		LocalFile:     meta.LocalFile,
		ContentType:   mime,
		ContentLength: meta.Size,
	})

	// Generate LAN URL with file extension (e.g. .flac, .mp3) so renderers can safely identify format
	lanURL := c.proxy.LANURLFor(meta.ID, dev.URL, meta.Format)

	now := time.Now()
	c.duration = meta.Duration
	c.position = startPos
	c.lastSyncPos = startPos
	c.lastSyncTime = now
	c.playing = true
	c.playStart = now
	c.mu.Unlock()

	// Register and route cover image through local proxy so smart TVs / screen renderers can safely fetch it on LAN
	coverURI := meta.CoverURL
	if len(meta.CoverData) > 0 {
		c.proxy.RegisterCoverData(meta.ID, meta.CoverData)
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	} else if meta.CoverURL != "" {
		c.proxy.RegisterCover(meta.ID, meta.CoverURL)
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	}

	// Assemble rich DIDL-Lite metadata compliant with UPnP MusicTrack specs
	item := &avtransport.MediaItem{
		URL:             lanURL,
		Title:           meta.Title,
		Artist:          meta.Artist,
		Album:           meta.Album,
		AlbumArtURI:     coverURI,
		ContentType:     mime,
		Seekable:        true,
		Duration:        meta.Duration,
		Size:            meta.Size,
		Bitrate:         meta.Bitrate,
		SampleFrequency: meta.SampleFrequency,
		BitsPerSample:   meta.BitsPerSample,
		NrAudioChannels: meta.Channels,
	}

	// 1. Set media with automatic Stop-before-Set protection to clear busy states
	if err := dev.SetMedia(ctx, item); err != nil {
		c.mu.Lock()
		c.playing = false
		c.mu.Unlock()
		c.notify()
		return fmt.Errorf("dlna: set media: %w", err)
	}

	// 2. Instruct renderer to play
	if err := dev.Play(ctx); err != nil {
		c.mu.Lock()
		c.playing = false
		c.mu.Unlock()
		c.notify()
		return fmt.Errorf("dlna: play: %w", err)
	}

	// 3. Apply initial seek offset asynchronously if resuming from another output,
	// waiting for renderer pipeline to buffer first (following Supersonic's pattern).
	if startPos > 0 {
		go func() {
			time.Sleep(2 * time.Second)
			c.mu.RLock()
			active := c.activeDevice
			playing := c.playing
			c.mu.RUnlock()
			if playing && active == dev {
				seekCtx, seekCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer seekCancel()
				_ = dev.Seek(seekCtx, int(startPos.Seconds()))
			}
		}()
	}

	pollCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancelPoll = cancel
	c.mu.Unlock()

	go c.pollProgress(pollCtx, dev)
	c.notify()
	return nil
}

// SetNextTrack stages the upcoming track on devices that support UPnP gapless queuing.
func (c *Controller) SetNextTrack(ctx context.Context, meta TrackMeta) error {
	c.mu.Lock()
	dev := c.activeDevice
	if dev == nil {
		c.mu.Unlock()
		return fmt.Errorf("dlna: no active device")
	}
	c.mu.Unlock()

	mime := FormatToMIME(meta.Format)
	c.proxy.Register(meta.ID, StreamSource{
		RemoteURL:     meta.StreamURL,
		Headers:       meta.Headers,
		LocalFile:     meta.LocalFile,
		ContentType:   mime,
		ContentLength: meta.Size,
	})

	lanURL := c.proxy.LANURLFor(meta.ID, dev.URL, meta.Format)
	coverURI := meta.CoverURL
	if len(meta.CoverData) > 0 {
		c.proxy.RegisterCoverData(meta.ID, meta.CoverData)
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	} else if meta.CoverURL != "" {
		c.proxy.RegisterCover(meta.ID, meta.CoverURL)
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	}

	item := &avtransport.MediaItem{
		URL:             lanURL,
		Title:           meta.Title,
		Artist:          meta.Artist,
		Album:           meta.Album,
		AlbumArtURI:     coverURI,
		ContentType:     mime,
		Seekable:        true,
		Duration:        meta.Duration,
		Size:            meta.Size,
		Bitrate:         meta.Bitrate,
		SampleFrequency: meta.SampleFrequency,
		BitsPerSample:   meta.BitsPerSample,
		NrAudioChannels: meta.Channels,
	}

	return dev.SetNextMedia(ctx, item)
}

func (c *Controller) pollProgress(ctx context.Context, dev *Device) {
	// 1. Initial 2-second settle: wait for remote renderer to start streaming and reach playing state
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}

	// 2. Initial calibration
	c.calibrateProgress(ctx, dev)

	// 3. Sparse calibration ticker: poll every 4 seconds to avoid overwhelming DMR microservices
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ended := c.calibrateProgress(ctx, dev)
			if ended {
				return
			}
		}
	}
}

func (c *Controller) calibrateProgress(ctx context.Context, dev *Device) bool {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	pos, dur, err := dev.PositionInfo(tctx)
	cancel()

	now := time.Now()
	c.mu.Lock()
	if err == nil {
		if dur > 0 {
			c.duration = dur
		}
		if pos >= 0 {
			c.position = pos
			c.lastSyncPos = pos
			c.lastSyncTime = now
		}
	}

	durTotal := c.duration
	calcPos := c.position
	if c.playing && !c.lastSyncTime.IsZero() {
		calcPos = c.lastSyncPos + time.Since(c.lastSyncTime)
	}
	wasPlaying := c.playing
	sinceStart := time.Since(c.playStart)
	c.mu.Unlock()

	// If track duration has elapsed or is within 1s of finish, check if track has ended
	trackNearingEnd := durTotal > 0 && calcPos >= durTotal-1*time.Second
	if trackNearingEnd || (durTotal > 0 && sinceStart > durTotal+2*time.Second) {
		sctx, scancel := context.WithTimeout(ctx, 2*time.Second)
		state, serr := dev.TransportState(sctx)
		scancel()

		if serr == nil && (state == "STOPPED" || state == "TRANSITIONING" || trackNearingEnd) {
			c.mu.Lock()
			c.playing = false
			endFn := c.onEnd
			c.mu.Unlock()
			c.notify()
			if endFn != nil {
				endFn()
			}
			return true
		}
	} else if err != nil {
		// Only check state when position call failed
		sctx, scancel := context.WithTimeout(ctx, 2*time.Second)
		state, serr := dev.TransportState(sctx)
		scancel()
		if serr == nil {
			c.mu.Lock()
			if state == "STOPPED" && wasPlaying && sinceStart > 5*time.Second {
				c.playing = false
				endFn := c.onEnd
				c.mu.Unlock()
				c.notify()
				if endFn != nil {
					endFn()
				}
				return true
			} else if state == "PAUSED_PLAYBACK" {
				c.playing = false
				c.mu.Unlock()
				c.notify()
				return false
			}
			c.mu.Unlock()
		}
	}

	c.notify()
	return false
}

// Stop stops playback on the remote device and cancels polling.
func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	if c.cancelPoll != nil {
		c.cancelPoll()
		c.cancelPoll = nil
	}
	dev := c.activeDevice
	c.playing = false
	c.lastSyncPos = 0
	c.lastSyncTime = time.Time{}
	c.position = 0
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		go func() {
			sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer scancel()
			_ = dev.Stop(sctx)
		}()
	}
	return nil
}

// Pause pauses the remote device.
func (c *Controller) Pause(ctx context.Context) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.playing = false
	if !c.lastSyncTime.IsZero() {
		c.lastSyncPos += time.Since(c.lastSyncTime)
		c.lastSyncTime = time.Time{}
		c.position = c.lastSyncPos
	}
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		go func() {
			pctx, pcancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer pcancel()
			_ = dev.Pause(pctx)
		}()
	}
	return nil
}

// Resume resumes playback on the remote device.
func (c *Controller) Resume(ctx context.Context) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.playing = true
	c.lastSyncTime = time.Now()
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		go func() {
			pctx, pcancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer pcancel()
			_ = dev.Play(pctx)
		}()
	}
	return nil
}

// Seek moves playback on the remote device to pos without blocking the UI thread.
func (c *Controller) Seek(ctx context.Context, pos time.Duration) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.position = pos
	c.lastSyncPos = pos
	c.lastSyncTime = time.Now()
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		go func(targetPos time.Duration) {
			sctx, scancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer scancel()
			_ = dev.Seek(sctx, int(targetPos.Seconds()))
		}(pos)
	}
	return nil
}

// SetVolume sets the volume (0..1) on the remote device with async debouncing to prevent UI freezes.
func (c *Controller) SetVolume(ctx context.Context, vol float64) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.volume = vol
	c.mu.Unlock()

	if dev != nil {
		c.volMu.Lock()
		c.pendingVol = vol
		c.volSeq++
		seq := c.volSeq
		c.volMu.Unlock()

		go func() {
			time.Sleep(80 * time.Millisecond) // 80ms debounce
			c.volMu.Lock()
			if seq != c.volSeq {
				c.volMu.Unlock()
				return
			}
			targetVol := c.pendingVol
			c.volMu.Unlock()

			sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer scancel()
			_ = dev.SetVolume(sctx, int(targetVol*100))
		}()
	}
	return nil
}

// Status returns current remote playback state with smooth local progression.
func (c *Controller) Status() (playing bool, pos time.Duration, dur time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.playing && !c.lastSyncTime.IsZero() {
		elapsed := time.Since(c.lastSyncTime)
		calcPos := c.lastSyncPos + elapsed
		if c.duration > 0 && calcPos > c.duration {
			calcPos = c.duration
		}
		return c.playing, calcPos, c.duration
	}
	return c.playing, c.position, c.duration
}

// IsCasting reports whether a DLNA device is currently selected and active.
func (c *Controller) IsCasting() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeDevice != nil
}

// Close shuts down the proxy and stops any active polling.
func (c *Controller) Close() error {
	c.mu.Lock()
	if c.cancelPoll != nil {
		c.cancelPoll()
		c.cancelPoll = nil
	}
	c.activeDevice = nil
	c.mu.Unlock()

	if c.proxy != nil {
		return c.proxy.Close()
	}
	return nil
}

func (c *Controller) notify() {
	if c.onStatusChange != nil {
		c.onStatusChange()
	}
}
