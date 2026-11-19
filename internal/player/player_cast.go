package player

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"waygu-music/internal/player/dlna"
	"waygu-music/internal/pluginapi"
)

type castPlayback struct {
	id        string
	cover     []byte
	cancel    context.CancelFunc
	release   context.CancelFunc
	retry     bool
	retryable bool
	attempt   int
	preparing string
}

func (p *Player) toggleCast() {
	if p.track == nil {
		return
	}
	if p.cast == nil {
		p.loadSkipping(p.at, 0, true, p.pending)
		return
	}
	ctx := p.trackContext()
	status := p.dlnaCtrl.CastStatus()
	p.playing = !p.castWantsPlay()
	if !p.playing {
		if errors.Is(status.Err, dlna.ErrRendererDidNotStart) {
			p.cast.retry = true
		}
		if p.cast.cancel != nil {
			p.cast.cancel()
			p.cast.cancel = nil
			p.loading = false
		}
		_ = p.dlnaCtrl.Pause(ctx)
	} else if status.ID == p.cast.id && status.Ready {
		_ = p.dlnaCtrl.Resume(ctx)
	} else {
		if status.ID == p.cast.id {
			p.pending = status.Position
			if status.Ended {
				p.pending = 0
			}
		}
		p.cast.retry = false
		p.loadCast(ctx, *p.track, p.playbackPreferences(p.track).Quality, false)
	}
	p.sync()
}

// castWantsPlay reads controller intent directly, before the next posted UI update.
func (p *Player) castWantsPlay() bool {
	if p.cast != nil && !p.loading {
		if status := p.dlnaCtrl.CastStatus(); status.ID == p.cast.id {
			return status.Want || errors.Is(status.Err, dlna.ErrRendererDidNotStart) && p.cast.retryable && !p.cast.retry
		}
	}
	return p.playing
}

func (p *Player) castChanged() {
	if p.cast != nil && p.IsCasting() {
		status := p.dlnaCtrl.CastStatus()
		if status.ID == p.cast.id && !p.loading {
			if !status.Ready && p.cast.release != nil {
				p.cast.release()
				p.cast.release = nil
			}
			if errors.Is(status.Err, dlna.ErrRendererDidNotStart) && p.cast.retryable && !p.cast.retry {
				p.pending = status.Position
				p.cast.retry = true
				p.playing = true
				p.dlnaCtrl.Stop()
				p.loadCast(p.trackContext(), *p.track, p.playbackPreferences(p.track).Quality, true)
				return
			}
			p.playing = status.Want
			if status.Playing {
				p.markStarted()
			}
		}
	}
	p.sync()
}

func (p *Player) loadCast(parent context.Context, track pluginapi.Track, level string, fallback bool) {
	cast := p.cast
	device := p.dlnaCtrl.ActiveDevice()
	if cast == nil || device == nil {
		return
	}
	gen := p.gen
	ctx, cancel := context.WithCancel(parent)
	if cast.release != nil {
		p.dlnaCtrl.Stop()
		cast.release()
		cast.release = nil
	}
	cast.attempt++
	id := fmt.Sprintf("%s-%d", p.castTrackID(&track, gen, p.at), cast.attempt)
	cast.id = id
	cast.cancel = cancel
	p.loading, p.err = true, ""
	cast.preparing = "正在准备投播…"
	p.sync()
	go func() {
		media, retryable, err := p.prepareCast(ctx, track, level, device, fallback, func() {
			p.post(func() {
				if p.cast == cast && ctx.Err() == nil && p.loading {
					cast.preparing = "正在获取兼容音源…"
					p.sync()
				}
			})
		})
		p.post(func() {
			if gen != p.gen || p.cast != cast || ctx.Err() != nil {
				return
			}
			cast.cancel = nil
			cast.retryable = retryable
			p.loading = false
			if err != nil {
				cancel()
				p.err, p.playing = err.Error(), false
				p.sync()
				return
			}
			meta := castMetadata(id, track, media)
			meta.CoverData = cast.cover
			meta.ExpectCover = track.Album.PicURL != ""
			p.dlnaCtrl.OnEnd(func() {
				p.post(func() {
					if p.cast == cast && ctx.Err() == nil {
						p.ended()
					}
				})
			})
			if err := p.dlnaCtrl.PrepareTrack(parent, meta, p.pending); err != nil {
				cancel()
				p.err, p.playing = err.Error(), false
				p.sync()
				return
			}
			cast.release = cancel
			p.info = media.info
			if p.playing {
				_ = p.dlnaCtrl.Resume(parent)
			}
			p.sync()
		})
	}()
}

// LoadingText describes which part of casting the player is waiting for.
func (p *Player) LoadingText() string {
	if p.cast != nil {
		if p.loading {
			return p.cast.preparing
		}
		if p.dlnaCtrl.CastStatus().Loading {
			return "正在等待投播设备…"
		}
	}
	return "正在缓冲…"
}

func castMetadata(id string, t pluginapi.Track, media playbackSource) dlna.TrackMeta {
	info := media.info
	var bitrate int
	if secs := int64(t.Length().Seconds()); secs > 0 {
		bitrate = int(info.Size / secs)
	}
	return dlna.TrackMeta{
		ID: id, Title: t.Name, Artist: t.ArtistNames(), Album: t.Album.Name,
		StreamURL: info.URL, LocalFile: media.localFile, Format: info.Type,
		Duration: t.Length(), Size: info.Size, Bitrate: bitrate, Header: info.Header,
	}
}

func (p *Player) prepareCast(ctx context.Context, t pluginapi.Track, level string, dev *dlna.Device, fallback bool, onCompatible func()) (playbackSource, bool, error) {
	caps := dev.AudioCapabilities(ctx)
	if !fallback {
		base, err := p.providerCastSource(ctx, &t, "")
		if err != nil {
			return playbackSource{}, false, err
		}
		if base.location() == "" {
			base, err = p.openOrdinary(ctx, t, level, true)
			if err != nil {
				return playbackSource{}, false, err
			}
		}
		if base.info == nil || base.location() == "" {
			return playbackSource{}, false, errors.New("没有可投播的音源")
		}
		if base.info.Type == "" && t.Format != "" {
			info := *base.info
			info.Type = t.Format
			base.info = &info
		}
		current := strings.TrimPrefix(strings.ToLower(base.info.Type), ".")
		if !caps.Known() || caps.Supports(current) {
			return base, !caps.Known() && current != "mp3", nil
		}
	}
	if caps.Known() && !caps.Supports("mp3") {
		return playbackSource{}, false, errors.New("设备不支持当前音源，也不支持备用 MP3 格式")
	}
	if onCompatible != nil {
		onCompatible()
	}
	compatible, err := p.providerCastSource(ctx, &t, "mp3")
	if err != nil {
		return playbackSource{}, false, fmt.Errorf("平台未能提供兼容的 MP3 音源：%w", err)
	}
	if compatible.location() == "" || compatible.info == nil || !strings.EqualFold(compatible.info.Type, "mp3") {
		return playbackSource{}, false, errors.New("设备不支持原音源，平台没有提供兼容的 MP3 音源")
	}
	return compatible, false, nil
}

func (p *Player) providerCastSource(ctx context.Context, t *pluginapi.Track, format string) (playbackSource, error) {
	if p.ResolveCastSource == nil {
		return playbackSource{}, nil
	}
	request, done := context.WithTimeout(ctx, 90*time.Second)
	defer done()
	info, segments, err := p.ResolveCastSource(request, t, format)
	if segments != nil {
		defer segments.Close()
	}
	if err != nil || info == nil {
		return playbackSource{}, err
	}
	if segments == nil {
		if format != "mp3" || info.URL == "" || !strings.EqualFold(info.Type, "mp3") {
			return playbackSource{info: info}, nil
		}
		return stageCastMP3(ctx, request, info, p.dlnaCtrl.Proxy())
	}
	if !strings.EqualFold(info.Type, "mp3") {
		return playbackSource{}, errors.New("平台返回了不支持的分段音频格式")
	}
	return withCastFile(ctx, "mp3", func(f *os.File) (playbackSource, error) {
		var size int64
		for i := range segments.SegmentCount() {
			if err := request.Err(); err != nil {
				return playbackSource{}, err
			}
			piece, err := segments.FetchSegment(request, i)
			if err != nil {
				return playbackSource{}, err
			}
			size += int64(len(piece))
			if len(piece) == 0 || size > 512<<20 {
				return playbackSource{}, errors.New("转码分段不完整或超过体积上限")
			}
			if _, err := f.Write(piece); err != nil {
				return playbackSource{}, err
			}
		}
		if size == 0 {
			return playbackSource{}, errors.New("转码没有生成音频")
		}
		return playbackSource{
			info:      &pluginapi.Stream{Type: "mp3", Level: info.Level, Size: size},
			localFile: f.Name(),
		}, nil
	})
}

func stageCastMP3(ctx, request context.Context, info *pluginapi.Stream, proxy *dlna.Proxy) (playbackSource, error) {
	return withCastFile(ctx, "mp3", func(f *os.File) (playbackSource, error) {
		size, staged, err := proxy.PrepareMP3(request, dlna.StreamSource{RemoteURL: info.URL, Header: info.Header}, f)
		if err != nil {
			return playbackSource{}, err
		}
		result := *info
		if size > 0 {
			result.Size = size
		}
		media := playbackSource{info: &result}
		if staged {
			result.URL, result.Header, result.VerifyCachedContent = "", nil, nil
			media.localFile = f.Name()
		}
		return media, nil
	})
}

// withCastFile owns a temporary file until the cast ends or the player cancels.
func withCastFile(ctx context.Context, format string, write func(*os.File) (playbackSource, error)) (playbackSource, error) {
	f, err := os.CreateTemp("", "wagyu-cast-*."+format)
	if err != nil {
		return playbackSource{}, err
	}
	path := f.Name()
	media, err := write(f)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil || media.localFile == "" {
		_ = os.Remove(path)
		return media, err
	}
	context.AfterFunc(ctx, func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Windows can still have the file open when a stream is cancelled.
			time.AfterFunc(time.Second, func() { _ = os.Remove(path) })
		}
	})
	return media, nil
}
