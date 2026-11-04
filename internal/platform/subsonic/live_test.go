package subsonic

import (
	"bytes"
	"context"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"testing"
	"time"

	_ "golang.org/x/image/webp"

	"netease-music-mygo/internal/platform"
)

func TestDefaultThumbnailGeneration(t *testing.T) {
	data, err := os.ReadFile("/tmp/navidrome_default_cover.webp")
	if err != nil {
		t.Skip(err)
	}
	thumb := defaultAlbumThumbnail(data)
	if len(thumb) == 0 || bytes.Equal(thumb, data) {
		t.Fatalf("expected thumb to be generated")
	}
	t.Logf("Original size: %d, thumb size: %d", len(data), len(thumb))

	// Verify decoding
	img, err := jpeg.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("failed to decode generated thumbnail: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 600 || b.Dy() != 600 {
		t.Fatalf("expected 600x600, got %dx%d", b.Dx(), b.Dy())
	}
}

// Tests connection, ping, album paging, song streaming, lyrics and search against a real Subsonic server.
func TestLiveSubsonic(t *testing.T) {
	addr := os.Getenv("SUBSONIC_TEST_URL")
	if addr == "" {
		addr = "http://172.16.1.10:4533"
	}
	user := os.Getenv("SUBSONIC_TEST_USER")
	if user == "" {
		user = "wwg"
	}
	pass := os.Getenv("SUBSONIC_TEST_PASS")
	if pass == "" {
		pass = "786b6C0PlKdRNB"
	}

	// Quick reachability check
	checkClient := &http.Client{Timeout: 2 * time.Second}
	resp, err := checkClient.Get(addr + "/rest/ping.view?u=test&p=test&v=1.13.0&c=test")
	if err != nil {
		t.Skipf("live server %s not reachable: %v", addr, err)
	}
	resp.Body.Close()

	client := NewClient(Config{
		Address:  addr,
		Username: user,
		Password: pass,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	if !client.Connected() {
		t.Fatalf("expected Connected() to be true")
	}

	p := Wrap(client)

	// Test ListAlbums starred: should not error even if user has 0 starred albums
	starredPage, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListAlbums,
		Sort: "starred",
		Page: 0,
		Size: 20,
	})
	if err != nil {
		t.Fatalf("Page(ListAlbums, Sort=starred) error: %v", err)
	}
	t.Logf("Page(ListAlbums, Sort=starred) returned %d albums (total=%d)", len(starredPage.Playlists), starredPage.Total)

	// Test ListAlbums frequent pagination stability: Page 0 and last page must report identical totals
	freqPage0, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListAlbums,
		Sort: "frequent",
		Page: 0,
		Size: 30,
	})
	if err != nil {
		t.Fatalf("Page(ListAlbums, Sort=frequent, Page=0) error: %v", err)
	}
	freqPageLast, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListAlbums,
		Sort: "frequent",
		Page: 3, // offset 90
		Size: 30,
	})
	if err != nil {
		t.Fatalf("Page(ListAlbums, Sort=frequent, Page=3) error: %v", err)
	}
	if freqPage0.Total != freqPageLast.Total {
		t.Fatalf("frequent pagination total unstable: Page0 total=%d, Page3 total=%d", freqPage0.Total, freqPageLast.Total)
	}
	t.Logf("frequent albums total=%d, Page0 cards=%d, Page3 cards=%d", freqPage0.Total, len(freqPage0.Playlists), len(freqPageLast.Playlists))

	// Test ListAlbums recent pagination stability
	recentPage0, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListAlbums,
		Sort: "recent",
		Page: 0,
		Size: 30,
	})
	if err != nil {
		t.Fatalf("Page(ListAlbums, Sort=recent, Page=0) error: %v", err)
	}
	recentPageLast, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListAlbums,
		Sort: "recent",
		Page: 3,
		Size: 30,
	})
	if err != nil {
		t.Fatalf("Page(ListAlbums, Sort=recent, Page=3) error: %v", err)
	}
	if recentPage0.Total != recentPageLast.Total {
		t.Fatalf("recent pagination total unstable: Page0 total=%d, Page3 total=%d", recentPage0.Total, recentPageLast.Total)
	}
	t.Logf("recent albums total=%d, Page0 cards=%d, Page3 cards=%d", recentPage0.Total, len(recentPage0.Playlists), len(recentPageLast.Playlists))

	// Test ListAlbums
	page, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListAlbums,
		Page: 0,
		Size: 5,
	})
	if err != nil {
		t.Fatalf("Page(ListAlbums) failed: %v", err)
	}
	if len(page.Playlists) == 0 {
		t.Fatalf("expected at least 1 album card")
	}
	t.Logf("Got %d album cards (total=%d), first: %s by %s", len(page.Playlists), page.Total, page.Playlists[0].Name, page.Playlists[0].Description)

	// Test ListAlbum songs
	albumRef := page.Playlists[0].Ref
	songPage, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListAlbum,
		Ref:  albumRef,
		Page: 0,
		Size: 10,
	})
	if err != nil {
		t.Fatalf("Page(ListAlbum) failed: %v", err)
	}
	t.Logf("Album %s has %d songs", albumRef, len(songPage.Tracks))

	// Test ListArtists and ListArtist
	artistsPage, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListArtists,
		Page: 0,
		Size: 5,
	})
	if err != nil {
		t.Fatalf("Page(ListArtists) failed: %v", err)
	}
	if len(artistsPage.Playlists) > 0 {
		artistRef := artistsPage.Playlists[0].Ref
		artistName := artistsPage.Playlists[0].Name
		t.Logf("First artist: %s (%s)", artistName, artistRef)

		artistSongsPage, err := p.Page(ctx, platform.PageRequest{
			Kind: platform.ListArtist,
			Ref:  artistRef,
			Page: 0,
			Size: 50,
		})
		if err != nil {
			t.Fatalf("Page(ListArtist) failed: %v", err)
		}
		t.Logf("Artist %s has %d songs (total=%d)", artistName, len(artistSongsPage.Tracks), artistSongsPage.Total)

		// Test raw sc.GetArtist
		rawArtist, err := client.Subsonic().GetArtist(artistRef)
		if err != nil {
			t.Fatalf("GetArtist failed: %v", err)
		}
		t.Logf("rawArtist has %d albums", len(rawArtist.Album))
		for _, a := range rawArtist.Album {
			albumData, err := client.Subsonic().GetAlbum(a.ID)
			if err != nil {
				t.Fatalf("GetAlbum %s failed: %v", a.ID, err)
			}
			t.Logf("  Album %s (%s) has %d songs", a.Name, a.ID, len(albumData.Song))
		}
	}

	// Test ListLibrary
	libPage, err := p.Page(ctx, platform.PageRequest{
		Kind: platform.ListLibrary,
		Page: 0,
		Size: 20,
	})
	if err != nil {
		t.Logf("Page(ListLibrary) error: %v", err)
	} else {
		t.Logf("Page(ListLibrary) returned %d songs (total=%d)", len(libPage.Tracks), libPage.Total)
	}

	// Test Search
	searchRes, err := p.Search(ctx, "齐秦", platform.SearchSongs, 0, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	t.Logf("Search for 齐秦 returned %d songs (total=%d)", len(searchRes.Tracks), searchRes.Total)

	searchAll, err := p.SearchAll(ctx, "齐秦")
	if err != nil {
		t.Fatalf("SearchAll failed: %v", err)
	}
	t.Logf("SearchAll: %d songs, %d albums, %d artists", len(searchAll.Tracks), len(searchAll.Albums), len(searchAll.Artists))
	for _, a := range searchAll.Albums {
		t.Logf("  Search album: Name=%s, ID=%d, Ref=%s, Kind=%s", a.Name, a.ID, a.Ref, a.Kind)
		albPage, err := p.Page(ctx, platform.PageRequest{Kind: platform.ListAlbum, Ref: a.Ref, Page: 0, Size: 10})
		if err != nil {
			t.Errorf("  Failed to load search album %s: %v", a.Ref, err)
		} else {
			t.Logf("  Loaded search album %s: %d songs", a.Name, len(albPage.Tracks))
		}
	}
	for _, ar := range searchAll.Artists {
		t.Logf("  Search artist: Name=%s, ID=%d, Ref=%s, Kind=%s", ar.Name, ar.ID, ar.Ref, ar.Kind)
		rawQin, err := client.Subsonic().GetArtist(ar.Ref)
		if err != nil {
			t.Fatalf("GetArtist 齐秦 failed: %v", err)
		}
		t.Logf("齐秦 has %d albums", len(rawQin.Album))
		totalSongs := 0
		for _, alb := range rawQin.Album {
			albData, err := client.Subsonic().GetAlbum(alb.ID)
			if err == nil {
				totalSongs += len(albData.Song)
			}
		}
		t.Logf("齐秦 total songs across albums: %d", totalSongs)
	}

	if len(searchRes.Tracks) > 0 {
		tr := &searchRes.Tracks[0]
		stream, err := p.Stream(ctx, tr)
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
		if stream.URL == "" {
			t.Fatalf("expected non-empty stream URL")
		}
		t.Logf("Stream URL: %s", stream.URL)

		// Test Lyrics
		lyrics, err := p.Lyrics(ctx, tr)
		if err != nil {
			t.Logf("Lyrics error: %v", err)
		} else {
			t.Logf("Lyrics length: %d", len(lyrics.LRC))
		}
	}

	// Test 100 albums coverArt
	albList, err := client.Subsonic().GetAlbumList2("alphabeticalByArtist", map[string]string{"size": "100", "offset": "0"})
	if err == nil {
		emptyCover := 0
		hasCover := 0
		failedFetch := 0
		for i, a := range albList {
			if a.CoverArt == "" {
				emptyCover++
				if emptyCover <= 5 {
					t.Logf("Album %q has EMPTY coverArt! ID=%s", a.Name, a.ID)
				}
			} else {
				hasCover++
				if i < 3 {
					// try Cover fetch
					data, err := p.Cover(ctx, CoverURL(a.CoverArt), 300)
					if err != nil {
						failedFetch++
						t.Logf("Album %q (coverArt=%s) Cover fetch error: %v", a.Name, a.CoverArt, err)
					} else {
						t.Logf("Album %q (coverArt=%s) Cover fetch ok: %d bytes", a.Name, a.CoverArt, len(data))
					}
				}
			}
		}
		t.Logf("100 albums: %d have cover, %d have EMPTY cover, %d failed fetch", hasCover, emptyCover, failedFetch)
	}

	// Check how many albums have 546-byte or small cover
	alb50, err := client.Subsonic().GetAlbumList2("newest", map[string]string{"size": "50", "offset": "0"})
	if err == nil {
		for _, a := range alb50 {
			if a.CoverArt != "" {
				data, err := p.Cover(ctx, CoverURL(a.CoverArt), 300)
				if err == nil && len(data) < 2000 {
					t.Logf("Album %q (coverArt=%s) has small cover: %d bytes", a.Name, a.CoverArt, len(data))
				}
			}
		}
	}
}
