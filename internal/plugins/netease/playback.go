package netease

import (
	"context"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/netease/internal/client"
)

func (*adapter) OwnsTrack(tr *pluginapi.Track) bool {
	// NetEase tracks persisted before plugins had no source field.
	return tr.Source == ""
}

func (p *adapter) PlaybackMedia() pluginapi.PlaybackMedia {
	return pluginapi.PlaybackMedia{
		Preferences: func() pluginapi.AudioPreferences {
			s := p.ui.preferences
			return pluginapi.AudioPreferences{Quality: s.Quality, Normalize: s.Normalize}
		},
		Stream: func(ctx context.Context, tr *pluginapi.Track, level string) (*pluginapi.Stream, error) {
			return netease.SongURL(ctx, tr.ID, level)
		},
		CastSource: func(ctx context.Context, tr *pluginapi.Track, format string) (*pluginapi.Stream, pluginapi.SegmentedSource, error) {
			if format != "mp3" {
				return nil, nil, nil
			}
			st, err := netease.SongURL(ctx, tr.ID, "standard")
			return st, nil, err
		},
		Cover: func(ctx context.Context, tr *pluginapi.Track) ([]byte, error) {
			return netease.Fetch(ctx, netease.Sized(tr.Album.PicURL, 600))
		},
		Lyrics: func(ctx context.Context, tr *pluginapi.Track) (string, string, string, error) {
			return netease.Lyrics(ctx, tr.ID)
		},
		ErrorMessage: func(tr *pluginapi.Track, err error) string {
			if err != netease.ErrUnavailable {
				return ""
			}
			if tr.ProgramID != 0 {
				return "这期节目需要购买才能收听"
			}
			return "这首歌需要 VIP 才能播放"
		},
	}
}
