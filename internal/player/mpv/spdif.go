package mpv

import "strings"

// SpdifCapableDevice reports whether an explicitly selected output looks
// like a receiver/HDMI path that can accept compressed E-AC-3. Built-in
// speakers and "auto" deliberately return false: those only expose PCM
// substreams, and forcing audio-spdif there makes MPV fail to initialize
// CoreAudio instead of decoding the E-AC-3 core.
func SpdifCapableDevice(d AudioDevice) bool {
	text := strings.ToLower(strings.TrimSpace(d.Description + " " + d.Name))
	if text == "" {
		return false
	}
	for _, marker := range []string{"built-in", "builtin", "speaker", "内置", "扬声器"} {
		if strings.Contains(text, marker) {
			return false
		}
	}
	for _, marker := range []string{"hdmi", "displayport", "spdif", "s/pdif", "receiver", "功放", "home theater"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
