package qqmusic

import (
	"math"

	"github.com/egoist/mygo/ui"
)

func (h *Home) coverBitmap(url string) *ui.Bitmap {
	return bitmap(h.host.DecodedArtwork(url, 240, false))
}

func (h *Home) artworkBackdrop(url string) *ui.Bitmap {
	if art := h.host.DecodedArtwork(url, 600, true); art != nil {
		return art.Blurred()
	}
	return nil
}

func (h *Home) artworkColors(url string) (deep, rich, glow ui.Color, ready bool) {
	art := h.host.DecodedArtwork(url, 600, false)
	if art == nil || art.Failed() {
		return ui.Color{}, ui.Color{}, ui.Color{}, false
	}
	hue, sat := 356.0, 0.74
	tint := art.Tint()
	if h, s, _ := hsv(float64(tint.R)/255, float64(tint.G)/255, float64(tint.B)/255); s > 0.15 {
		hue, sat = h, min(max(s*1.2, 0.5), 0.78)
	}
	return hsvColor(hue, sat, 0.24), hsvColor(math.Mod(hue+16, 360), sat*0.9, 0.5), hsvColor(math.Mod(hue+32, 360), sat*0.55, 1), true
}
