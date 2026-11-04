package subsonic

import (
	"context"
	"fmt"
	"html"
	"strings"

	gosubsonic "github.com/supersonic-app/go-subsonic/subsonic"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
)

func formatStructuredLyrics(sl *gosubsonic.StructuredLyrics) string {
	if sl == nil || len(sl.Lines) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, line := range sl.Lines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			text = strings.TrimSpace(line.Value)
		}
		ms := line.Start
		m := ms / 60000
		s := (ms % 60000) / 1000
		cs := (ms % 1000) / 10
		sb.WriteString(fmt.Sprintf("[%02d:%02d.%02d]%s\n", m, s, cs, text))
	}
	return html.UnescapeString(sb.String())
}

// Lyrics fetches synchronized or plain lyrics for a track.
// It prioritizes OpenSubsonic's GetLyricsBySongId and falls back to standard GetLyrics.
func (p *Provider) Lyrics(ctx context.Context, tr *netease.Track) (platform.Lyrics, error) {
	if tr == nil {
		return platform.Lyrics{}, nil
	}
	sc, err := p.subsonic()
	if err != nil {
		return platform.Lyrics{}, err
	}

	// 1. Try OpenSubsonic GetLyricsBySongId
	if tr.Ref != "" {
		lyricsList, err := sc.GetLyricsBySongId(tr.Ref)
		if err == nil && lyricsList != nil && len(lyricsList.StructuredLyrics) > 0 {
			for _, sl := range lyricsList.StructuredLyrics {
				if lrc := formatStructuredLyrics(sl); lrc != "" {
					return platform.Lyrics{LRC: lrc}, nil
				}
			}
		}
	}

	// 2. Fallback to standard Subsonic GetLyrics
	artist := ""
	if len(tr.Artists) > 0 {
		artist = tr.Artists[0].Name
	}
	lyrics, err := sc.GetLyrics(tr.Name, artist)
	if err == nil && lyrics != nil && strings.TrimSpace(lyrics.Text) != "" {
		return platform.Lyrics{LRC: html.UnescapeString(lyrics.Text)}, nil
	}

	return platform.Lyrics{}, nil
}
