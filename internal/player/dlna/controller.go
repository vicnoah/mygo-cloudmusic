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
	ID        string
	Title     string
	Artist    string
	Album     string
	CoverURL  string
	StreamURL string
	LocalFile string
	Format    string
	Duration  time.Duration
	Size      int64
	Headers   http.Header
}

// Controller manages DLNA media discovery, streaming proxy, and remote device playback.
type Controller struct {
	mu           sync.RWMutex
	proxy        *Proxy
	knownDevices map[string]*Device // Keyed by device URL / ID to prevent flapping
	devices      []*Device          // Stable list presented to the UI
	activeDevice *Device

	playing   bool
	playStart time.Time
	position  time.Duration
	duration  time.Duration
	volume    float64 // 0..1

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

	if deviceID == "" {
		if c.activeDevice != nil {
			go func(d *Device) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = d.Stop(ctx)
			}(c.activeDevice)
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

	c.duration = meta.Duration
	c.position = startPos
	c.playing = true
	c.playStart = time.Now()
	c.mu.Unlock()

	// Register and route cover image through local proxy so smart TVs / screen renderers can safely fetch it on LAN
	coverURI := meta.CoverURL
	if meta.CoverURL != "" {
		c.proxy.RegisterCover(meta.ID, meta.CoverURL)
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	}

	// Assemble rich DIDL-Lite metadata compliant with UPnP MusicTrack specs
	item := &avtransport.MediaItem{
		URL:         lanURL,
		Title:       meta.Title,
		Artist:      meta.Artist,
		Album:       meta.Album,
		AlbumArtURI: coverURI,
		ContentType: mime,
		Seekable:    true,
		Duration:    meta.Duration,
		Size:        meta.Size,
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

	// 3. Apply initial seek offset if resuming from another output
	if startPos > 0 {
		_ = dev.Seek(ctx, int(startPos.Seconds()))
	}

	pollCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancelPoll = cancel
	c.mu.Unlock()

	go c.pollProgress(pollCtx, dev)
	c.notify()
	return nil
}

func (c *Controller) pollProgress(ctx context.Context, dev *Device) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			pos, dur, err := dev.PositionInfo(tctx)
			cancel()

			if err == nil {
				c.mu.Lock()
				if dur > 0 {
					c.duration = dur
				}
				if pos >= 0 {
					c.position = pos
				}
				c.mu.Unlock()
				c.notify()
			}

			// Check transport state
			sctx, scancel := context.WithTimeout(ctx, 2*time.Second)
			state, serr := dev.TransportState(sctx)
			scancel()

			if serr == nil {
				c.mu.Lock()
				wasPlaying := c.playing
				sinceStart := time.Since(c.playStart)
				switch state {
				case "PLAYING":
					c.playing = true
				case "PAUSED_PLAYBACK":
					c.playing = false
				case "STOPPED":
					c.playing = false
					// Only fire onEnd if we were actively playing and the song has run for at least 5s,
					// or position has reached near duration. This prevents transient STOPPED state
					// right at track loading from skipping songs.
					shouldEnd := wasPlaying && sinceStart > 5*time.Second &&
						(c.duration == 0 || c.position >= c.duration-3*time.Second || c.position == 0)
					if shouldEnd && c.onEnd != nil {
						endFn := c.onEnd
						c.mu.Unlock()
						endFn()
						c.mu.Lock()
					}
				}
				c.mu.Unlock()
				c.notify()
			}
		}
	}
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
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		return dev.Stop(ctx)
	}
	return nil
}

// Pause pauses the remote device.
func (c *Controller) Pause(ctx context.Context) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.playing = false
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		return dev.Pause(ctx)
	}
	return nil
}

// Resume resumes playback on the remote device.
func (c *Controller) Resume(ctx context.Context) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.playing = true
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		return dev.Play(ctx)
	}
	return nil
}

// Seek moves playback on the remote device to pos.
func (c *Controller) Seek(ctx context.Context, pos time.Duration) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.position = pos
	c.mu.Unlock()

	c.notify()
	if dev != nil {
		return dev.Seek(ctx, int(pos.Seconds()))
	}
	return nil
}

// SetVolume sets the volume (0..1) on the remote device.
func (c *Controller) SetVolume(ctx context.Context, vol float64) error {
	c.mu.Lock()
	dev := c.activeDevice
	c.volume = vol
	c.mu.Unlock()

	if dev != nil {
		return dev.SetVolume(ctx, int(vol*100))
	}
	return nil
}

// Status returns current remote playback state.
func (c *Controller) Status() (playing bool, pos time.Duration, dur time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
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
