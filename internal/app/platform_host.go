package app

import (
	"context"

	"github.com/egoist/mygo/ui"
	appplatform "github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/artwork"
	"github.com/waygu-music-plugin/sdk/pluginapi/network"
)

type platformHost struct {
	a  *app
	id appplatform.ID
}

func (h platformHost) authorization() (context.Context, func() bool) {
	if h.id == "" {
		return nil, nil
	}
	authority := h.a.platformAuthority.Load()
	if authority == nil || authority.id != h.id {
		return nil, func() bool { return false }
	}
	return authority.ctx, func() bool { return h.a.platformAuthority.Load() == authority }
}

func (h platformHost) canWrite() bool {
	authority := h.a.platformAuthority.Load()
	return h.id == "" || (authority != nil && authority.id == h.id)
}

func allowedWrite(guard func() bool) bool { return guard == nil || guard() }

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

func (h platformHost) Post(fn func()) { h.a.post(h.activationCallback(fn)) }

// An asynchronous result queued by an earlier activation cannot write after
// switching away and returning to the same platform.
func (h platformHost) activationCallback(fn func()) func() {
	_, guard := h.authorization()
	return func() {
		if allowedWrite(guard) {
			fn()
		}
	}
}

func (h platformHost) Invalidate() {
	if h.canWrite() {
		h.a.redraw()
	}
}

func (h platformHost) ShowMain() {
	if h.canWrite() {
		h.a.showMain()
	}
}

func (h platformHost) Toast(msg string) {
	if !h.canWrite() {
		return
	}
	h.a.toast = msg
	h.a.redraw()
}

func (h platformHost) TakeToast() string {
	if !h.canWrite() {
		return ""
	}
	toast := h.a.toast
	h.a.toast = ""
	return toast
}

func (h platformHost) Player() appplatform.PlayerPort {
	if h.a == nil || h.a.pl == nil {
		return nil
	}
	scope, guard := h.authorization()
	return sharedPlayer{p: h.a.pl, write: guard, scope: scope, owns: h.ownsTracks}
}

func (h platformHost) ownsTracks(tracks []appplatform.Track) bool {
	return h.id == "" || h.a.ownsTracksFor(h.id, tracks)
}

func (h platformHost) GlobalSearchHistory() []string {
	return append([]string(nil), h.a.settings.SearchHistory...)
}

func (h platformHost) RememberGlobalSearch(q string) {
	if h.canWrite() {
		h.a.rememberSearch(q)
	}
}

func (h platformHost) ClearSearchHistory() {
	if h.canWrite() {
		h.a.clearSearchHistory()
	}
}

func (h platformHost) CurrentPlatform() appplatform.ID {
	if h.a == nil {
		return ""
	}
	return h.a.platformID
}

func (h platformHost) Artwork(url string, size int) (*ui.Bitmap, bool) {
	if h.a == nil || h.a.art == nil || url == "" {
		return nil, true
	}
	art := h.cachedArtwork(url, size, false)
	if art == nil {
		return nil, false
	}
	return art.Bitmap(), true
}

func (h platformHost) cachedArtwork(raw string, size int, backdrop bool) *artwork.CachedImage {
	url := network.SizedArtworkURL(raw, size)
	key := url
	if _, _, scoped := appplatform.ArtworkOwner(raw); !scoped {
		key = appplatform.ArtworkURL(h.a.platformID, url)
	}
	return h.a.art.Get(key, url, backdrop)
}

func (h platformHost) PlaybackLevels() ([appplatform.SpectrumBands]float32, bool) {
	if h.a == nil || h.a.pl == nil {
		return [appplatform.SpectrumBands]float32{}, false
	}
	return h.a.pl.Levels()
}

func (h platformHost) CopyText(text string) {
	if h.canWrite() {
		copyText(text)
	}
}

func (h platformHost) OpenSettings() {
	if h.canWrite() {
		h.a.openSettings()
	}
}

func (h platformHost) CloseSettings() {
	if win := h.a.settingsWin; h.canWrite() && win != nil {
		win.Close()
	}
}

type sharedPlayer struct {
	write func() bool
	scope context.Context
	owns  func([]appplatform.Track) bool
	p     interface {
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
		PlayList([]appplatform.Track, int, appplatform.QueueFeed)
		PlayRadio(string, []appplatform.Track, appplatform.QueueFeed)
		EndRadio()
		Radio() string
		ToggleShuffle()
		Shuffle() bool
	}
}

func (p sharedPlayer) Current() *appplatform.Track { return p.p.Current() }
func (p sharedPlayer) Playing() bool               { return p.p.Playing() }
func (p sharedPlayer) Loading() bool               { return p.p.Loading() }
func (p sharedPlayer) Err() string                 { return p.p.Err() }
func (p sharedPlayer) Queue() []appplatform.Track  { return p.p.Queue() }
func (p sharedPlayer) PlayOrder() ([]int, int)     { return p.p.PlayOrder() }
func (p sharedPlayer) TogglePlay() {
	if allowedWrite(p.write) {
		p.p.TogglePlay()
	}
}

func (p sharedPlayer) Next() {
	if allowedWrite(p.write) {
		p.p.Next()
	}
}

func (p sharedPlayer) PlayQueue(t []appplatform.Track, i int) {
	if allowedWrite(p.write) && 0 <= i && i < len(t) && p.owns(t) {
		p.p.PlayQueue(t, i)
	}
}

func (p sharedPlayer) AppendQueue(t []appplatform.Track) {
	if allowedWrite(p.write) && p.owns(t) {
		p.p.AppendQueue(t)
	}
}

func (p sharedPlayer) PlayList(t []appplatform.Track, i int, f appplatform.QueueFeed) {
	if allowedWrite(p.write) && 0 <= i && i < len(t) && p.owns(t) {
		p.p.PlayList(t, i, scopedQueueFeed(p.scope, p.write, p.owns, f))
	}
}

func (p sharedPlayer) PlayRadio(n string, t []appplatform.Track, f appplatform.QueueFeed) {
	if allowedWrite(p.write) && p.owns(t) {
		p.p.PlayRadio(n, t, scopedQueueFeed(p.scope, p.write, p.owns, f))
	}
}

func scopedQueueFeed(scope context.Context, write func() bool, owns func([]appplatform.Track) bool, f appplatform.QueueFeed) appplatform.QueueFeed {
	if f == nil || scope == nil || write == nil {
		return f
	}
	return func(ctx context.Context, cursor appplatform.QueueFeedCursor, out func([]appplatform.Track, error)) {
		if !write() || scope.Err() != nil {
			return
		}
		ctx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(scope, cancel)
		context.AfterFunc(ctx, func() { stop() })
		f(ctx, cursor, func(tracks []appplatform.Track, err error) {
			defer cancel()
			if ctx.Err() == nil && scope.Err() == nil && write() {
				if err == nil && !owns(tracks) {
					out(nil, appplatform.ErrQueueFeedUnavailable)
					return
				}
				out(tracks, err)
			}
		})
	}
}

func (p sharedPlayer) EndRadio() {
	if allowedWrite(p.write) {
		p.p.EndRadio()
	}
}

func (p sharedPlayer) Radio() string { return p.p.Radio() }
func (p sharedPlayer) SetShuffle(on bool) {
	if allowedWrite(p.write) && p.p.Shuffle() != on {
		p.p.ToggleShuffle()
	}
}

var _ appplatform.Services = platformHost{}
