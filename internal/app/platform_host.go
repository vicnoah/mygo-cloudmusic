package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	appplatform "waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/network"
)

type platformHost struct{ a *app }

// fetchArtwork sends scoped references to their owner and supports legacy
// unscoped URLs using the instance captured when the request was made.
func (h platformHost) fetchArtwork(ctx context.Context, current appplatform.ID, raw string) ([]byte, error) {
	if owner, ref, ok := appplatform.ArtworkOwner(raw); ok {
		if adapter := h.a.platforms[owner]; adapter != nil {
			data, handled, err := adapter.FetchArtwork(ctx, ref, 600)
			if handled || err != nil {
				return data, err
			}
		}
		return nil, appplatform.ErrUnavailable
	}
	if adapter := h.a.platforms[current]; adapter != nil {
		data, handled, err := adapter.FetchArtwork(ctx, raw, 600)
		if handled {
			return data, err
		}
	}
	return network.FetchContext(ctx, raw)
}

func (h platformHost) Post(fn func()) { h.a.post(fn) }
func (h platformHost) Invalidate()    { h.a.redraw() }
func (h platformHost) ShowMain()      { h.a.showMain() }
func (h platformHost) Toast(msg string) {
	h.a.toast = msg
	h.a.redraw()
}

func (h platformHost) TakeToast() string {
	toast := h.a.toast
	h.a.toast = ""
	return toast
}
func (h platformHost) Player() appplatform.PlayerPort {
	if h.a == nil || h.a.pl == nil {
		return nil
	}
	return sharedPlayer{h.a.pl}
}

func (h platformHost) SwitchPlatform(id appplatform.ID) { h.a.setPlatform(id) }

func (h platformHost) PlatformConfigured(id appplatform.ID) bool {
	if adapter := h.a.platforms[id]; adapter != nil {
		return adapter.Configured()
	}
	return true
}

func (h platformHost) GlobalSearchHistory() []string {
	return append([]string(nil), h.a.settings.SearchHistory...)
}

func (h platformHost) RememberGlobalSearch(q string) { h.a.rememberSearch(q) }

func (h platformHost) ClearSearchHistory() { h.a.clearSearchHistory() }

func (h platformHost) CurrentPlatform() appplatform.ID {
	if h.a == nil {
		return ""
	}
	return h.a.platformID
}

func (h platformHost) PlatformOptions() []appplatform.Info {
	if h.a == nil {
		return nil
	}
	return h.a.platformInfos()
}

func (h platformHost) Artwork(url string, size int) (*ui.Bitmap, bool) {
	if h.a == nil || h.a.art == nil || url == "" {
		return nil, true
	}
	art := h.cachedArtwork(url, size, false)
	if art == nil {
		return nil, false
	}
	return art.bmp, true
}

func (h platformHost) cachedArtwork(raw string, size int, backdrop bool) *artwork {
	url := network.SizedArtworkURL(raw, size)
	key := url
	if _, _, scoped := appplatform.ArtworkOwner(raw); !scoped {
		key = appplatform.ArtworkURL(h.a.platformID, url)
	}
	return h.a.art.get(key, url, backdrop)
}

func (h platformHost) PlaybackLevels() ([appplatform.SpectrumBands]float32, bool) {
	if h.a == nil || h.a.pl == nil {
		return [appplatform.SpectrumBands]float32{}, false
	}
	return h.a.pl.Levels()
}

func (h platformHost) CopyText(text string) { copyText(text) }

func (h platformHost) OpenSettings() { h.a.openSettings() }

func (h platformHost) CloseSettings() {
	if win := h.a.settingsWin; win != nil {
		win.Close()
	}
}

type sharedPlayer struct {
	p interface {
		Current() *appplatform.Track
		Playing() bool
		Loading() bool
		Err() string
		TogglePlay()
		Next()
		Queue() []appplatform.Track
		PlayOrder() ([]int, int)
		PlayQueue([]appplatform.Track, int)
		AppendQueue([]appplatform.Track)
		PlayList([]appplatform.Track, int, func(func([]appplatform.Track)))
		PlayRadio(string, []appplatform.Track, func(func([]appplatform.Track)))
		EndRadio()
		Radio() string
		ToggleShuffle()
		Shuffle() bool
	}
}

func (p sharedPlayer) Current() *appplatform.Track            { return p.p.Current() }
func (p sharedPlayer) Playing() bool                          { return p.p.Playing() }
func (p sharedPlayer) Loading() bool                          { return p.p.Loading() }
func (p sharedPlayer) Err() string                            { return p.p.Err() }
func (p sharedPlayer) TogglePlay()                            { p.p.TogglePlay() }
func (p sharedPlayer) Next()                                  { p.p.Next() }
func (p sharedPlayer) Queue() []appplatform.Track             { return p.p.Queue() }
func (p sharedPlayer) PlayOrder() ([]int, int)                { return p.p.PlayOrder() }
func (p sharedPlayer) PlayQueue(t []appplatform.Track, i int) { p.p.PlayQueue(t, i) }
func (p sharedPlayer) AppendQueue(t []appplatform.Track)      { p.p.AppendQueue(t) }
func (p sharedPlayer) PlayList(t []appplatform.Track, i int, f func(func([]appplatform.Track))) {
	p.p.PlayList(t, i, f)
}
func (p sharedPlayer) PlayRadio(n string, t []appplatform.Track, f func(func([]appplatform.Track))) {
	p.p.PlayRadio(n, t, f)
}
func (p sharedPlayer) EndRadio()     { p.p.EndRadio() }
func (p sharedPlayer) Radio() string { return p.p.Radio() }
func (p sharedPlayer) SetShuffle(on bool) {
	if p.p.Shuffle() != on {
		p.p.ToggleShuffle()
	}
}

var _ appplatform.Services = platformHost{}
