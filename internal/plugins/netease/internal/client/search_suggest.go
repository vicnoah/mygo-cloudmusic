package netease

import (
	"context"
	"net/url"
)

// Suggestions are what a search may be after, as its query is typed, in
// the order NetEase ranks their kinds. It suggests no users or podcasts.
type Suggestions struct {
	Songs     []Track
	Artists   []Artist
	Albums    []AlbumProfile
	Playlists []Playlist
	Order     []string // "songs", "artists", "albums" and "playlists"
}

// SearchSuggest returns what a search for query may be after.
func SearchSuggest(ctx context.Context, query string) (Suggestions, error) {
	var res struct {
		Result struct {
			Songs   []legacyTrack `json:"songs"`
			Artists []Artist      `json:"artists"`
			Albums  []struct {
				AlbumProfile
				Artist Artist `json:"artist"` // a suggested album has the one
			} `json:"albums"`
			Playlists []Playlist `json:"playlists"`
			Order     []string   `json:"order"`
		} `json:"result"`
	}
	if err := get(ctx, "/api/search/suggest/web", url.Values{"s": {query}}, &res); err != nil {
		return Suggestions{}, err
	}
	r := res.Result
	s := Suggestions{Artists: r.Artists, Playlists: r.Playlists, Order: r.Order}
	for _, t := range r.Songs {
		s.Songs = append(s.Songs, t.track())
	}
	for _, al := range r.Albums {
		al.Artists = []Artist{al.Artist}
		s.Albums = append(s.Albums, al.AlbumProfile)
	}
	return s, nil
}
