package mpv

// BuildSpatialFilter returns the MPV audio filter for a spatial mode. MPV
// builds without FFmpeg's libmysofa cannot use sofalizer/headphone with an
// HRIR; crossfeed keeps a stable stereo image and avoids a hard failure.
// Multichannel output is handled by the audio-channels property instead.
func BuildSpatialFilter(mode, profile string) string {
	if mode != "headphones" {
		return ""
	}
	switch profile {
	case "wide":
		return "lavfi=[crossfeed=strength=0.35:range=0.85:slope=0.4,volume=0.65]"
	default:
		return "lavfi=[crossfeed=strength=0.2:range=0.5:slope=0.5,volume=0.65]"
	}
}
