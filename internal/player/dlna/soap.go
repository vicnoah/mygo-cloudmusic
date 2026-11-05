package dlna

import (
	"fmt"
	"math"
	"strings"

	"github.com/supersonic-app/go-upnpcast/services/avtransport"
)

// escapeXML escapes XML characters <, >, &, and single quote.
// Note: Double quotes are preserved unescaped so that attributes inside DIDL-Lite
// remain valid and Samsung/Sony TV parsers do not fail on &#34;.
func escapeXML(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '&':
			buf.WriteString("&amp;")
		case '\'':
			buf.WriteString("&apos;")
		default:
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

func secondsToClockTime(secs int) string {
	hours := secs / 3600
	secs %= 3600
	minutes := secs / 60
	secs %= 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

// buildEnhancedDIDLLite generates UPnP / DLNA DIDL-Lite XML containing all compatible
// artwork tags for smart TVs, screen renderers, and standard UPnP speakers.
func buildEnhancedDIDLLite(item *avtransport.MediaItem) string {
	var class string
	if strings.HasPrefix(item.ContentType, "video/") {
		class = "object.item.videoItem.movie"
	} else {
		class = "object.item.audioItem.musicTrack"
	}

	contentFeatures := buildContentFeatures(item.ContentType)

	var sb strings.Builder
	sb.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" `)
	sb.WriteString(`xmlns:dc="http://purl.org/dc/elements/1.1/" `)
	sb.WriteString(`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" `)
	sb.WriteString(`xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/" `)
	sb.WriteString(`xmlns:sec="http://www.sec.co.kr/">`)
	sb.WriteString(`<item id="1" parentID="0" restricted="1">`)

	if item.Title != "" {
		sb.WriteString(`<dc:title>`)
		sb.WriteString(escapeXML(item.Title))
		sb.WriteString(`</dc:title>`)
	}
	if item.Artist != "" {
		sb.WriteString(`<dc:creator>`)
		sb.WriteString(escapeXML(item.Artist))
		sb.WriteString(`</dc:creator>`)
		sb.WriteString(`<upnp:artist>`)
		sb.WriteString(escapeXML(item.Artist))
		sb.WriteString(`</upnp:artist>`)
	}
	if item.Album != "" {
		sb.WriteString(`<upnp:album>`)
		sb.WriteString(escapeXML(item.Album))
		sb.WriteString(`</upnp:album>`)
	}
	if item.TrackNumber > 0 {
		fmt.Fprintf(&sb, `<upnp:originalTrackNumber>%d</upnp:originalTrackNumber>`, item.TrackNumber)
	}

	// Cover / Artwork elements for Smart TVs and screen renderers:
	// We inject multiple standard UPnP and DLNA tags so every TV platform
	// (Samsung Tizen, LG webOS, Sony Bravia, Xiaomi TV, Huawei, LeBo, etc.) can pick up artwork.
	if item.AlbumArtURI != "" {
		artURI := escapeXML(item.AlbumArtURI)
		// 1. DLNA profile-specific tags
		sb.WriteString(`<upnp:albumArtURI dlna:profileID="JPEG_MED" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">` + artURI + `</upnp:albumArtURI>`)
		sb.WriteString(`<upnp:albumArtURI dlna:profileID="JPEG_TN" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">` + artURI + `</upnp:albumArtURI>`)
		// 2. Generic UPnP albumArtURI and icon
		sb.WriteString(`<upnp:albumArtURI>` + artURI + `</upnp:albumArtURI>`)
		sb.WriteString(`<upnp:icon>` + artURI + `</upnp:icon>`)
	}

	sb.WriteString(`<upnp:class>` + class + `</upnp:class>`)

	// Primary Audio Stream res element
	sb.WriteString(`<res protocolInfo="http-get:*:`)
	sb.WriteString(item.ContentType)
	sb.WriteString(`:`)
	sb.WriteString(contentFeatures)
	sb.WriteString(`"`)
	if item.Duration > 0 {
		durStr := secondsToClockTime(int(math.Round(item.Duration.Seconds())))
		fmt.Fprintf(&sb, ` duration="%s"`, durStr)
	}
	if item.Size > 0 {
		fmt.Fprintf(&sb, ` size="%d"`, item.Size)
	}
	if item.Bitrate > 0 {
		fmt.Fprintf(&sb, ` bitrate="%d"`, item.Bitrate)
	}
	if item.SampleFrequency > 0 {
		fmt.Fprintf(&sb, ` sampleFrequency="%d"`, item.SampleFrequency)
	}
	if item.BitsPerSample > 0 {
		fmt.Fprintf(&sb, ` bitsPerSample="%d"`, item.BitsPerSample)
	}
	if item.NrAudioChannels > 0 {
		fmt.Fprintf(&sb, ` nrAudioChannels="%d"`, item.NrAudioChannels)
	}
	sb.WriteString(`>`)
	sb.WriteString(escapeXML(item.URL))
	sb.WriteString(`</res>`)

	// Image res elements for TVs that only parse <res> elements for artwork
	if item.AlbumArtURI != "" {
		artURI := escapeXML(item.AlbumArtURI)
		sb.WriteString(`<res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_MED;DLNA.ORG_FLAGS=00900000000000000000000000000000">` + artURI + `</res>`)
		sb.WriteString(`<res protocolInfo="http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN;DLNA.ORG_FLAGS=00900000000000000000000000000000">` + artURI + `</res>`)
		sb.WriteString(`<res protocolInfo="http-get:*:image/jpeg:*">` + artURI + `</res>`)
	}

	// Subtitles
	if strings.Contains(item.SubtitlesURL, "srt") {
		subURL := escapeXML(item.SubtitlesURL)
		sb.WriteString(`<res protocolInfo="http-get:*:text/srt:*">` + subURL + `</res>`)
		sb.WriteString(`<sec:CaptionInfo sec:type="srt">` + subURL + `</sec:CaptionInfo>`)
	}

	sb.WriteString(`</item></DIDL-Lite>`)
	return sb.String()
}

// buildSetAVTransportSOAP builds the SOAP envelope for SetAVTransportURI with escaped DIDL-Lite.
func buildSetAVTransportSOAP(currentURI, didl string) []byte {
	escapedMeta := escapeXML(didl)
	escapedURI := escapeXML(currentURI)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	sb.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`)
	sb.WriteString(`<s:Body>`)
	sb.WriteString(`<u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">`)
	sb.WriteString(`<InstanceID>0</InstanceID>`)
	sb.WriteString(`<CurrentURI>`)
	sb.WriteString(escapedURI)
	sb.WriteString(`</CurrentURI>`)
	sb.WriteString(`<CurrentURIMetaData>`)
	sb.WriteString(escapedMeta)
	sb.WriteString(`</CurrentURIMetaData>`)
	sb.WriteString(`</u:SetAVTransportURI>`)
	sb.WriteString(`</s:Body>`)
	sb.WriteString(`</s:Envelope>`)
	return []byte(sb.String())
}

// buildSetNextAVTransportSOAP builds the SOAP envelope for SetNextAVTransportURI with escaped DIDL-Lite.
func buildSetNextAVTransportSOAP(nextURI, didl string) []byte {
	escapedMeta := escapeXML(didl)
	escapedURI := escapeXML(nextURI)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	sb.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`)
	sb.WriteString(`<s:Body>`)
	sb.WriteString(`<u:SetNextAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">`)
	sb.WriteString(`<InstanceID>0</InstanceID>`)
	sb.WriteString(`<NextURI>`)
	sb.WriteString(escapedURI)
	sb.WriteString(`</NextURI>`)
	sb.WriteString(`<NextURIMetaData>`)
	sb.WriteString(escapedMeta)
	sb.WriteString(`</NextURIMetaData>`)
	sb.WriteString(`</u:SetNextAVTransportURI>`)
	sb.WriteString(`</s:Body>`)
	sb.WriteString(`</s:Envelope>`)
	return []byte(sb.String())
}
