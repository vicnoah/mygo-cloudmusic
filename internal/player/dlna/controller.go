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
	ExpectCover     bool
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
	Header          func() http.Header
	telemetry       *streamTelemetry
}

// Controller manages DLNA media discovery, streaming proxy, and remote device playback.
type Controller struct {
	mu           sync.RWMutex
	remoteMu     sync.Mutex
	proxy        *Proxy
	knownDevices map[string]*Device // Keyed by device URL / ID to prevent flapping
	devices      []*Device          // Stable list presented to the UI
	activeDevice *Device
	currentID    string
	nextID       string
	remoteStops  map[string]chan struct{}

	playing  bool
	session  *castSession
	position time.Duration
	duration time.Duration
	volume   float64 // 0..1

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
		if c.activeDevice == nil || c.activeDevice.ID != d.ID {
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
		if c.activeDevice != nil && dev.ID == c.activeDevice.ID {
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
	oldDev := c.activeDevice
	if deviceID == "" {
		c.stopDeviceLocked(oldDev)
		c.clearTracksLocked()
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
		c.stopDeviceLocked(oldDev)
		c.clearTracksLocked()
	}

	if oldDev != nil && oldDev.ID == found.ID {
		found = oldDev
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

// SetNextTrack stages the upcoming track on devices that support UPnP gapless queuing.
func (c *Controller) SetNextTrack(ctx context.Context, meta TrackMeta) error {
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return err
	}
	dev := c.activeDevice
	if dev == nil || c.currentID == "" {
		c.mu.Unlock()
		return fmt.Errorf("dlna: no active track")
	}
	if c.nextID != "" && c.nextID != c.currentID {
		c.proxy.Unregister(c.nextID)
	}
	c.nextID = meta.ID
	item := c.registerTrack(meta, dev)
	c.mu.Unlock()
	return dev.SetNextMedia(ctx, item)
}

func (c *Controller) registerTrack(meta TrackMeta, dev *Device) *avtransport.MediaItem {
	mime := FormatToMIME(meta.Format)
	c.proxy.Register(meta.ID, StreamSource{
		RemoteURL:     meta.StreamURL,
		Headers:       meta.Headers,
		LocalFile:     meta.LocalFile,
		ContentType:   mime,
		ContentLength: meta.Size,
		Header:        meta.Header,
		Telemetry:     meta.telemetry,
	})

	lanURL := c.proxy.LANURLFor(meta.ID, dev.URL, meta.Format)
	coverURI := meta.CoverURL
	if len(meta.CoverData) > 0 {
		c.proxy.RegisterCoverData(meta.ID, meta.CoverData)
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	} else if meta.CoverURL != "" {
		c.proxy.RegisterCover(meta.ID, meta.CoverURL)
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	} else if meta.ExpectCover {
		coverURI = c.proxy.LANCoverURLFor(meta.ID, dev.URL)
	}

	return &avtransport.MediaItem{
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
}

// Stop drops the cast stream and stops remote playback without blocking the UI.
func (c *Controller) Stop() {
	c.mu.Lock()
	if c.cancelPoll != nil {
		c.cancelPoll()
		c.cancelPoll = nil
	}
	c.stopDeviceLocked(c.activeDevice)
	c.clearTracksLocked()
	c.playing = false
	c.position = 0
	c.duration = 0
	c.mu.Unlock()

	c.notify()
}

func (c *Controller) clearTracksLocked() {
	if c.cancelPoll != nil {
		c.cancelPoll()
		c.cancelPoll = nil
	}
	c.session = nil
	c.playing = false
	c.position, c.duration = 0, 0
	c.revokeTracksLocked()
}

func (c *Controller) revokeTracksLocked() {
	if c.currentID != "" {
		c.proxy.Unregister(c.currentID)
	}
	if c.nextID != "" && c.nextID != c.currentID {
		c.proxy.Unregister(c.nextID)
	}
	c.currentID, c.nextID = "", ""
}

// A new cast waits for any queued Stop so it cannot be stopped by the old session.
func (c *Controller) stopDeviceLocked(dev *Device) {
	if dev == nil || c.session == nil || !c.session.sent {
		return
	}
	if c.remoteStops == nil {
		c.remoteStops = make(map[string]chan struct{})
	}
	dev, media := c.session.device, c.session.item.URL
	deviceID := dev.ID
	previous := c.remoteStops[deviceID]
	stopped := make(chan struct{})
	c.remoteStops[deviceID] = stopped
	go func() {
		if previous != nil {
			<-previous
		}
		c.remoteMu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		query, stopQuery := context.WithTimeout(ctx, 500*time.Millisecond)
		info, _ := dev.readPosition(query)
		stopQuery()
		if sameMedia(info.URI, media) {
			_ = dev.Stop(ctx)
		}
		cancel()
		c.remoteMu.Unlock()
		c.mu.Lock()
		if c.remoteStops[deviceID] == stopped {
			delete(c.remoteStops, deviceID)
		}
		close(stopped)
		c.mu.Unlock()
	}()
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
	return c.playing, c.positionLocked(), c.duration
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
	c.stopDeviceLocked(c.activeDevice)
	c.clearTracksLocked()
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
