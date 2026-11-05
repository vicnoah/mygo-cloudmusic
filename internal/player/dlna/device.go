package dlna

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/koron/go-ssdp"
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
	LastSeen     time.Time

	mr     *device.MediaRenderer
	av     *avtransport.Client
	rc     *renderingcontrol.Client
	rcOnce sync.Once
}

// retryHandler retries transient SOAP failures up to 3 times, matching Supersonic's resilience pattern.
type retryHandler struct {
	client *http.Client
}

func newRetryHandler(timeout time.Duration) *retryHandler {
	return &retryHandler{
		client: &http.Client{Timeout: timeout},
	}
}

func (h *retryHandler) Do(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}

	var lastErr error
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(time.Duration(attempt*150) * time.Millisecond):
			}
		}
		newReq := req.Clone(req.Context())
		if bodyBytes != nil {
			newReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
		resp, lastErr = h.client.Do(newReq)
		if lastErr == nil && resp != nil && resp.StatusCode < 500 {
			return resp, nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	return resp, lastErr
}

type rawServiceXML struct {
	Type        string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
	EventSubURL string `xml:"eventSubURL"`
}

type rawDeviceXML struct {
	FriendlyName string `xml:"friendlyName"`
	ModelName    string `xml:"modelName"`
	DeviceType   string `xml:"deviceType"`
	ServiceList  struct {
		Services []rawServiceXML `xml:"service"`
	} `xml:"serviceList"`
	DeviceList struct {
		Devices []rawDeviceXML `xml:"device"`
	} `xml:"deviceList"`
}

type rootXML struct {
	XMLName xml.Name     `xml:"root"`
	Device  rawDeviceXML `xml:"device"`
}

func resolveURL(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

func findServiceURLs(d *rawDeviceXML, baseURL *url.URL) (name, model, avCtrl, avEvent, rcCtrl string) {
	if d == nil {
		return
	}
	name = d.FriendlyName
	model = d.ModelName

	for _, s := range d.ServiceList.Services {
		if strings.Contains(s.Type, "AVTransport") && avCtrl == "" {
			avCtrl = resolveURL(baseURL, s.ControlURL)
			avEvent = resolveURL(baseURL, s.EventSubURL)
		}
		if strings.Contains(s.Type, "RenderingControl") && rcCtrl == "" {
			rcCtrl = resolveURL(baseURL, s.ControlURL)
		}
	}

	for i := range d.DeviceList.Devices {
		n, m, ac, ae, rc := findServiceURLs(&d.DeviceList.Devices[i], baseURL)
		if ac != "" {
			if name == "" {
				name = n
			}
			if model == "" {
				model = m
			}
			if avCtrl == "" {
				avCtrl = ac
				avEvent = ae
			}
			if rcCtrl == "" {
				rcCtrl = rc
			}
		}
	}
	return
}

// Discover searches for MediaRenderer devices on the LAN.
// It uses both go-upnpcast's built-in discovery and raw SSDP parsing to ensure devices
// that advertise MediaRenderer:1, MediaRenderer:2, or generic UPnP rootdevice are discovered.
func Discover(ctx context.Context, waitSec int) ([]*Device, error) {
	if waitSec <= 0 {
		waitSec = 2
	}

	var devices []*Device
	seen := make(map[string]bool)
	now := time.Now()

	// 1. Search using go-upnpcast's SearchMediaRenderers with an independent safe timeout.
	searchCtx, cancelSearch := context.WithTimeout(context.Background(), time.Duration(waitSec+5)*time.Second)
	renderers, _ := device.SearchMediaRenderers(searchCtx, waitSec)
	cancelSearch()

	for _, mr := range renderers {
		if mr == nil || mr.URL == "" || seen[mr.URL] {
			continue
		}
		seen[mr.URL] = true
		av, err := mr.AVTransportClient()
		if err != nil {
			continue
		}
		av.RequestHandler = newRetryHandler(8 * time.Second)

		var rc *renderingcontrol.Client
		if rcc, err := mr.RenderingControlClient(); err == nil {
			rcc.Requesthandler = &http.Client{Timeout: 5 * time.Second}
			rc = rcc
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
			LastSeen:     now,
			mr:           mr,
			av:           av,
			rc:           rc,
		})
	}

	// 2. Perform direct SSDP discovery across ssdp:all to discover devices that report
	// ST: urn:schemas-upnp-org:device:MediaRenderer:1 or MediaRenderer:2 directly.
	rawServices, err := ssdp.Search(ssdp.All, waitSec, "")
	if err == nil {
		httpClient := &http.Client{Timeout: 3 * time.Second}
		for _, srv := range rawServices {
			if srv.Location == "" || seen[srv.Location] {
				continue
			}
			st := strings.ToLower(srv.Type)
			if !strings.Contains(st, "mediarenderer") && !strings.Contains(st, "avtransport") && srv.Type != ssdp.RootDevice {
				continue
			}

			parsedBase, err := url.Parse(srv.Location)
			if err != nil || parsedBase.Host == "" {
				continue
			}

			reqCtx, cancelReq := context.WithTimeout(context.Background(), 3*time.Second)
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.Location, nil)
			if err != nil {
				cancelReq()
				continue
			}
			req.Header.Set("Connection", "close")
			resp, err := httpClient.Do(req)
			cancelReq()
			if err != nil {
				continue
			}

			var root rootXML
			decodeErr := xml.NewDecoder(resp.Body).Decode(&root)
			resp.Body.Close()
			if decodeErr != nil {
				continue
			}

			name, model, avCtrl, avEvent, rcCtrl := findServiceURLs(&root.Device, parsedBase)
			if avCtrl == "" {
				continue
			}

			seen[srv.Location] = true
			av := avtransport.NewClient(avCtrl, avEvent)
			av.RequestHandler = newRetryHandler(8 * time.Second)

			var rc *renderingcontrol.Client
			if rcCtrl != "" {
				rc = renderingcontrol.NewClient(rcCtrl)
				rc.Requesthandler = &http.Client{Timeout: 5 * time.Second}
			}

			if name == "" {
				name = model
			}
			if name == "" {
				name = "DLNA 设备"
			}

			devices = append(devices, &Device{
				ID:           srv.Location,
				FriendlyName: name,
				ModelName:    model,
				URL:          srv.Location,
				LastSeen:     now,
				av:           av,
				rc:           rc,
			})
		}
	}

	return devices, nil
}

// Play instructs the device to start playback of the current media.
func (d *Device) Play(ctx context.Context) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}
	err := d.av.Play(ctx)
	if err != nil {
		// Many renderers return error 701 (Transition not available) immediately after SetAVTransportURI.
		// Retrying once after 200ms allows the renderer firmware to complete its pipeline transition.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		err = d.av.Play(ctx)
	}
	return err
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

// SetMedia loads a media item on the device with UPnP AV state transition protection.
func (d *Device) SetMedia(ctx context.Context, item *avtransport.MediaItem) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}

	// 1. DLNA DMRs often return 501 / 705 if SetAVTransportURI is called while playing another track.
	// We send a best-effort Stop with a short timeout to transition into a ready state.
	stopCtx, stopCancel := context.WithTimeout(ctx, 1*time.Second)
	_ = d.av.Stop(stopCtx)
	stopCancel()

	// Short settle interval before loading new metadata.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(80 * time.Millisecond):
	}

	// 2. Set the media URI and DIDL-Lite metadata.
	if err := d.av.SetAVTransportMedia(ctx, item); err != nil {
		return fmt.Errorf("set av transport media: %w", err)
	}

	// Short settle interval for renderer to parse DIDL-Lite before calling Play.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(80 * time.Millisecond):
	}

	return nil
}

// SetNextMedia stages the upcoming track on devices that support UPnP gapless queuing.
func (d *Device) SetNextMedia(ctx context.Context, item *avtransport.MediaItem) error {
	if d.av == nil {
		return errors.New("dlna: av transport client not available")
	}
	return d.av.SetNextAVTransportMedia(ctx, item)
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
		if d.rc == nil && d.mr != nil {
			client, err := d.mr.RenderingControlClient()
			if err == nil {
				client.Requesthandler = &http.Client{Timeout: 5 * time.Second}
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
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	return rc.SetVolume(ctx, vol)
}
