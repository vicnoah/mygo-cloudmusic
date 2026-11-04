package dlna

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Controller manages DLNA media discovery, streaming proxy, and remote device playback.
type Controller struct {
	mu           sync.RWMutex
	proxy        *Proxy
	devices      []*Device
	activeDevice *Device

	playing  bool
	position time.Duration
	duration time.Duration
	volume   float64 // 0..1

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

// RefreshDevices discovers reachable DLNA MediaRenderer devices on the LAN.
func (c *Controller) RefreshDevices(ctx context.Context) ([]*Device, error) {
	devs, err := Discover(ctx, 2)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.devices = devs
	}
	return c.devices, err
}

// Devices returns the most recently discovered devices.
func (c *Controller) Devices() []*Device {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]*Device, len(c.devices))
	copy(res, c.devices)
	return res
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
		c.mu.Unlock()
		c.notify()
		return nil
	}

	var found *Device
	for _, d := range c.devices {
		if d.ID == deviceID || d.FriendlyName == deviceID {
			found = d
			break
		}
	}
	if found == nil {
		c.mu.Unlock()
		return fmt.Errorf("dlna: device %q not found", deviceID)
	}

	c.activeDevice = found
	c.mu.Unlock()
	c.notify()
	return nil
}

// PlayTrack streams a track to the active DLNA device via the local proxy.
func (c *Controller) PlayTrack(ctx context.Context, trackID, title, streamURL, localFile, format string, headers http.Header, dur time.Duration, startPos time.Duration) error {
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

	mime := FormatToMIME(format)
	c.proxy.Register(trackID, StreamSource{
		RemoteURL:   streamURL,
		Headers:     headers,
		LocalFile:   localFile,
		ContentType: mime,
	})
	lanURL := c.proxy.LANURLFor(trackID, dev.URL)

	c.duration = dur
	c.position = startPos
	c.playing = true
	c.mu.Unlock()

	// Load and start playback on the device.
	if err := dev.SetMedia(ctx, lanURL, title, mime, dur); err != nil {
		c.mu.Lock()
		c.playing = false
		c.mu.Unlock()
		c.notify()
		return fmt.Errorf("dlna: set media: %w", err)
	}

	if err := dev.Play(ctx); err != nil {
		c.mu.Lock()
		c.playing = false
		c.mu.Unlock()
		c.notify()
		return fmt.Errorf("dlna: play: %w", err)
	}

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
				switch state {
				case "PLAYING":
					c.playing = true
				case "PAUSED_PLAYBACK":
					c.playing = false
				case "STOPPED":
					c.playing = false
					if wasPlaying && c.onEnd != nil {
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
