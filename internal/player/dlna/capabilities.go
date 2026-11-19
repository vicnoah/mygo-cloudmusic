package dlna

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AudioCapabilities is what the renderer advertises for HTTP audio delivery.
// An unavailable response leaves source compatibility unknown.
type AudioCapabilities struct {
	protocols []string
}

func (a AudioCapabilities) Known() bool { return len(a.protocols) != 0 }

func (a AudioCapabilities) Supports(format string) bool {
	mime := strings.ToLower(strings.Split(FormatToMIME(format), ";")[0])
	if mime == "application/octet-stream" {
		return false
	}
	for _, item := range a.protocols {
		parts := strings.SplitN(item, ":", 4)
		if len(parts) != 4 || !strings.EqualFold(parts[0], "http-get") {
			continue
		}
		media := strings.ToLower(strings.TrimSpace(strings.SplitN(parts[2], ";", 2)[0]))
		if mime == "audio/l16" && media == mime &&
			(!strings.Contains(strings.ToLower(parts[2]), "rate=44100") || !strings.Contains(strings.ToLower(parts[2]), "channels=2")) {
			continue
		}
		if media == "*" || media == "audio/*" || media == mime || (mime == "audio/flac" && media == "audio/x-flac") {
			if mime == "audio/mpeg" && strings.Contains(strings.ToUpper(parts[3]), "DLNA.ORG_PN=") &&
				!strings.Contains(strings.ToUpper(parts[3]), "DLNA.ORG_PN=MP3") {
				continue
			}
			return true
		}
	}
	return false
}

// AudioCapabilities reads the receiver's Sink profiles and caches successful discovery.
func (d *Device) AudioCapabilities(ctx context.Context) AudioCapabilities {
	d.capMu.Lock()
	if !d.capAt.IsZero() && time.Since(d.capAt) < 5*time.Minute {
		caps := d.caps
		d.capMu.Unlock()
		return caps
	}
	d.capMu.Unlock()

	base, err := url.Parse(d.URL)
	if err != nil || base.Host == "" {
		return AudioCapabilities{}
	}
	query, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(query, http.MethodGet, d.URL, nil)
	if err != nil {
		return AudioCapabilities{}
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return AudioCapabilities{}
	}
	var desc rootXML
	err = xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&desc)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || err != nil {
		return AudioCapabilities{}
	}
	endpoint := connectionEndpoint(desc.Device, base)
	if endpoint == "" {
		return AudioCapabilities{}
	}
	var answer struct {
		Sink string `xml:"Body>GetProtocolInfoResponse>Sink"`
	}
	if err := soapRead(query, endpoint, connectionManager, "GetProtocolInfo", "", client, &answer); err != nil {
		return AudioCapabilities{}
	}
	var caps AudioCapabilities
	for _, item := range strings.Split(answer.Sink, ",") {
		item = strings.TrimSpace(item)
		parts := strings.SplitN(item, ":", 4)
		if len(parts) == 4 && strings.EqualFold(parts[0], "http-get") {
			caps.protocols = append(caps.protocols, item)
		}
	}
	if caps.Known() {
		d.capMu.Lock()
		d.capAt, d.caps = time.Now(), caps
		d.capMu.Unlock()
	}
	return caps
}

func connectionEndpoint(dev rawDeviceXML, base *url.URL) string {
	for _, svc := range dev.ServiceList.Services {
		if strings.HasPrefix(svc.Type, "urn:schemas-upnp-org:service:ConnectionManager:") {
			return resolveURL(base, svc.ControlURL)
		}
	}
	for _, child := range dev.DeviceList.Devices {
		if endpoint := connectionEndpoint(child, base); endpoint != "" {
			return endpoint
		}
	}
	return ""
}
