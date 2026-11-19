package dlna

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const avTransport = "urn:schemas-upnp-org:service:AVTransport:1"
const connectionManager = "urn:schemas-upnp-org:service:ConnectionManager:1"

func soapRead(ctx context.Context, endpoint, service, action, args string, client interface {
	Do(*http.Request) (*http.Response, error)
}, output any) error {
	body := `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` + action + ` xmlns:u="` + service + `">` + args + `</u:` + action + `></s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("charset", "utf-8")
	req.Header.Set("SOAPAction", `"`+service+"#"+action+`"`)
	req.Close = true
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var response struct {
			Code string `xml:"Body>Fault>detail>UPnPError>errorCode"`
		}
		_ = xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&response)
		return &transportFault{action, response.Code, resp.StatusCode}
	}
	if output == nil {
		return nil
	}
	return xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(output)
}

type transportFault struct {
	action, code string
	status       int
}

func (e *transportFault) Error() string {
	if e.code == "" {
		return fmt.Sprintf("%s: HTTP %d", e.action, e.status)
	}
	return fmt.Sprintf("%s: UPnP %s", e.action, e.code)
}

func sameMedia(got, expected string) bool {
	got = strings.TrimSpace(got)
	if got == "" || got == "NOT_IMPLEMENTED" || got == expected {
		return true
	}
	a, errA := url.Parse(got)
	b, errB := url.Parse(expected)
	if errA != nil || errB != nil {
		return false
	}
	// Some receivers return a relative URI or normalize its escaping.
	return (a.Host == "" || strings.EqualFold(a.Host, b.Host)) &&
		a.Path == b.Path && a.RawQuery == b.RawQuery
}

type positionInfo struct {
	Position, Duration time.Duration
	Known              bool
	URI                string
}

func (d *Device) readPosition(ctx context.Context) (positionInfo, error) {
	if d.av == nil || d.avCtrl == "" {
		return positionInfo{}, errors.New("设备未提供播放进度接口")
	}
	var response struct {
		Position *struct {
			RelTime, TrackDuration, TrackURI string
		} `xml:"Body>GetPositionInfoResponse"`
	}
	if err := soapRead(ctx, d.avCtrl, avTransport, "GetPositionInfo", "<InstanceID>0</InstanceID>", d.av.RequestHandler, &response); err != nil {
		return positionInfo{}, err
	}
	if response.Position == nil {
		return positionInfo{}, errors.New("设备没有返回播放进度")
	}
	pos, known := parseClock(response.Position.RelTime)
	dur, _ := parseClock(response.Position.TrackDuration)
	return positionInfo{Position: pos, Duration: dur, Known: known, URI: response.Position.TrackURI}, nil
}

func parseDuration(s string) time.Duration {
	d, _ := parseClock(s)
	return d
}

func parseClock(s string) (time.Duration, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) == 1 {
		// A few devices return a bare count of seconds.
		sec, err := strconv.ParseUint(parts[0], 10, 32)
		return time.Duration(sec) * time.Second, err == nil
	}
	if len(parts) != 3 {
		return 0, false
	}
	h, eh := strconv.ParseUint(parts[0], 10, 16)
	m, em := strconv.ParseUint(parts[1], 10, 8)
	sec, es := time.ParseDuration(parts[2] + "s")
	if eh != nil || em != nil || es != nil || m >= 60 || sec < 0 || sec >= time.Minute {
		return 0, false
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + sec, true
}
