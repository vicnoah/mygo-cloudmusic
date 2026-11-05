package mpv

import "testing"

func TestSpdifCapableDevice(t *testing.T) {
	tests := []struct {
		name string
		dev  AudioDevice
		want bool
	}{
		{"builtin speaker", AudioDevice{Name: "coreaudio/BuiltInSpeakerDevice", Description: "MacBook Pro Speakers"}, false},
		{"auto", AudioDevice{Name: "auto", Description: "System default"}, false},
		{"hdmi receiver", AudioDevice{Name: "coreaudio/HDMI", Description: "Denon AVR-X2700H (HDMI)"}, true},
		{"displayport", AudioDevice{Name: "coreaudio/DP", Description: "DisplayPort Audio"}, true},
		{"usb dac", AudioDevice{Name: "coreaudio/USB", Description: "USB DAC"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpdifCapableDevice(tt.dev); got != tt.want {
				t.Fatalf("SpdifCapableDevice(%+v) = %v, want %v", tt.dev, got, tt.want)
			}
		})
	}
}
