package dlna

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/supersonic-app/go-upnpcast/device"
	"github.com/supersonic-app/go-upnpcast/services/avtransport"
	"github.com/supersonic-app/go-upnpcast/services/renderingcontrol"
)

// Device represents an accessible UPnP/DLNA MediaRenderer on the local network.
type Device struct {
	ID           string // unique identifier, usually device URL or UDN
	FriendlyName string
	ModelName    string
	URL          string

	mr  *device.MediaRenderer
	av  *avtransport.Client
	rc  *renderingcontrol.Client
	rcOnce sync.Once
}

// Discover searches for MediaRenderer devices on the LAN.
func Discover(ctx context.Context, waitSec int) ([]*Device, error) {
	if waitSec <= 0 {
		waitSec = 2
	}
	renderers, err := device.SearchMediaRenderers(ctx, waitSec)
	if err != nil && len(renderers) == 0 {
		return nil, err
	}

	var devices []*Device
	seen := make(map[string]bool)
	for _, mr := range renderers {
		if mr == nil || mr.URL == "" || seen[mr.URL] {
			continue
		}
		seen[mr.URL] = true
		av, err := mr.AVTransportClient()
		if err != nil {
			continue
		}
		name := mr.FriendlyName
		if name == "" {
			name = mr.ModelName
		}
		if name == "" {
			name = "DLNA 设备"
		}
		devices = append(devices, &Device{
			ID:           mr.URL,
			FriendlyName: name,
			ModelName:    mr.ModelName,
			URL:          mr.URL,
			mr:           mr,
			av:           av,
		})
	}
	return devices, nil
}

// Play instructs the device to start playback of the current media.
func (d *Device) Play(ctx context.Context) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}
	return d.av.Play(ctx)
}

// Pause pauses playback on the device.
func (d *Device) Pause(ctx context.Context) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}
	return d.av.Pause(ctx)
}

// Stop stops playback on the device.
func (d *Device) Stop(ctx context.Context) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}
	return d.av.Stop(ctx)
}

// Seek seeks to a target position in seconds.
func (d *Device) Seek(ctx context.Context, targetSeconds int) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}
	return d.av.Seek(ctx, targetSeconds)
}

// SetMedia loads a media URL on the device.
func (d *Device) SetMedia(ctx context.Context, streamURL, title, mimeType string, dur time.Duration) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}
	item := &avtransport.MediaItem{
		URL:         streamURL,
		Title:       title,
		ContentType: mimeType,
		Seekable:    true,
		Duration:    dur,
	}
	return d.av.SetAVTransportMedia(ctx, item)
}

// PositionInfo fetches current playback position and duration.
func (d *Device) PositionInfo(ctx context.Context) (pos, dur time.Duration, err error) {
	if d.av == nil {
		return 0, 0, errors.New("dlna: av transport client not available")
	}
	info, err := d.av.GetPositionInfo(ctx)
	if err != nil {
		return 0, 0, err
	}
	return info.RelTime, info.Duration, nil
}

// TransportState returns the current playback state (e.g. "PLAYING", "PAUSED_PLAYBACK", "STOPPED").
func (d *Device) TransportState(ctx context.Context) (string, error) {
	if d.av == nil {
		return "", errors.New("dlna: av transport client not available")
	}
	info, err := d.av.GetTransportInfo(ctx)
	if err != nil {
		return "", err
	}
	return info.State, nil
}

func (d *Device) renderingControl() *renderingcontrol.Client {
	d.rcOnce.Do(func() {
		if d.mr != nil {
			client, err := d.mr.RenderingControlClient()
			if err == nil {
				d.rc = client
			}
		}
	})
	return d.rc
}

// SetVolume sets the device volume between 0 and 100.
func (d *Device) SetVolume(ctx context.Context, vol int) error {
	rc := d.renderingControl()
	if rc == nil {
		return nil // Non-fatal if device doesn't support RenderingControl
	}
	return rc.SetVolume(ctx, vol)
}
