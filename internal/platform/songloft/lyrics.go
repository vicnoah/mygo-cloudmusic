package songloft

import (
	"html"

	"netease-music-mygo/internal/platform"
)

// ToLyrics converts a Songloft LyricResponse into the standard platform.Lyrics model,
// unescaping any HTML/XML character entities (e.g. &apos;, &quot;, &amp;).
func ToLyrics(resp LyricResponse) platform.Lyrics {
	return platform.Lyrics{
		LRC:         html.UnescapeString(resp.Lyric),
		Translation: html.UnescapeString(resp.Tlyric),
		YRC:         html.UnescapeString(resp.Lxlyric),
	}
}
