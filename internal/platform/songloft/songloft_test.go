package songloft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"netease-music-mygo/internal/platform"
)

func TestSongloftClientAndProvider(t *testing.T) {
	mux := http.NewServeMux()

	// Mock Auth
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req loginReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Username == "wwg" && req.Password == "correct" {
			_ = json.NewEncoder(w).Encode(loginResp{
				AccessToken:  "mock-access-token",
				RefreshToken: "mock-refresh-token",
				ExpiresIn:    3600,
				TokenType:    "Bearer",
			})
			return
		}
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
	})

	// Mock Songs
	mux.HandleFunc("/api/v1/songs", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mock-access-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(SongListResponse{
			Songs: []Song{
				{
					ID:       2801,
					Title:    "失落沙洲",
					Artist:   "徐佳莹",
					Album:    "LaLa首张创作专辑",
					Duration: 299,
					Format:   "flac",
				},
			},
			Total:  1,
			Limit:  20,
			Offset: 0,
		})
	})

	// Mock Facets
	mux.HandleFunc("/api/v1/songs/facets", func(w http.ResponseWriter, r *http.Request) {
		field := r.URL.Query().Get("field")
		_ = json.NewEncoder(w).Encode(FacetListResponse{
			Facets: []FacetItem{
				{Value: "Test " + field, Count: 5, CoverURL: "/api/v1/songs/2801/cover"},
			},
			Field: field,
			Total: 1,
		})
	})

	// Mock Playlists
	mux.HandleFunc("/api/v1/playlists", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PlaylistListResponse{
			Playlists: []Playlist{
				{ID: 1, Name: "收藏", Labels: []string{"built_in"}, SongCount: 1},
				{ID: 5, Name: "My Playlist", SongCount: 10},
			},
			Total: 2,
		})
	})

	// Mock Playlist songs
	mux.HandleFunc("/api/v1/playlists/1/songs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(SongListResponse{
			Songs: []Song{
				{ID: 2801, Title: "失落沙洲", Artist: "徐佳莹", Album: "LaLa首张创作专辑", Duration: 299, Format: "flac"},
			},
			Total: 1,
		})
	})

	// Mock Lyric
	mux.HandleFunc("/api/v1/songs/2801/lyric", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(LyricResponse{
			Lyric:  "[00:00.00]失落沙洲",
			Tlyric: "[00:00.00]Lost Sandbar",
		})
	})

	// Mock Cover
	mux.HandleFunc("/api/v1/songs/2801/cover", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-jpeg-data"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx := context.Background()
	p := NewProvider(Config{
		Address:  server.URL,
		Username: "wwg",
		Password: "correct",
	})

	if err := p.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if !p.Connected() {
		t.Fatal("expected connected to be true")
	}

	// 1. Page library
	pg, err := p.Page(ctx, platform.PageRequest{Kind: platform.ListLibrary, Size: 20})
	if err != nil {
		t.Fatalf("page library: %v", err)
	}
	if len(pg.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(pg.Tracks))
	}
	tr := pg.Tracks[0]
	if tr.Name != "失落沙洲" || tr.Format != "flac" || !tr.Fav {
		t.Errorf("unexpected track: %+v", tr)
	}

	// 2. Stream
	st, err := p.Stream(ctx, &tr)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if !strings.Contains(st.URL, "/api/v1/songs/2801/play?access_token=mock-access-token") {
		t.Errorf("unexpected stream url: %s", st.URL)
	}

	// 3. Lyrics
	ly, err := p.Lyrics(ctx, &tr)
	if err != nil {
		t.Fatalf("lyrics: %v", err)
	}
	if !strings.Contains(ly.LRC, "失落沙洲") || !strings.Contains(ly.Translation, "Lost Sandbar") {
		t.Errorf("unexpected lyrics: %+v", ly)
	}

	// 4. Cover
	coverBytes, err := p.Cover(ctx, SongCoverURL(2801), 300)
	if err != nil {
		t.Fatalf("cover: %v", err)
	}
	if string(coverBytes) != "fake-jpeg-data" {
		t.Errorf("unexpected cover data: %s", string(coverBytes))
	}

	// 5. Facets albums and artists
	albPg, err := p.Page(ctx, platform.PageRequest{Kind: platform.ListAlbums, Size: 20})
	if err != nil {
		t.Fatalf("albums: %v", err)
	}
	if len(albPg.Playlists) != 1 || albPg.Playlists[0].Name != "Test album" {
		t.Errorf("unexpected album page: %+v", albPg)
	}
}

// Tests that ToLyrics unescapes HTML/XML character entities in Songloft lyric response.
func TestToLyricsUnescape(t *testing.T) {
	resp := LyricResponse{
		Lyric:   "[00:10.00]Don&apos;t say you&quot;re &amp; I&#39;m &lt;fine&gt;",
		Tlyric:  "[00:10.00]别说你&quot;好&quot; &amp; 我很好",
		Lxlyric: "[00:10.00](0,500)Don&apos;t",
	}
	ly := ToLyrics(resp)
	wantLRC := `[00:10.00]Don't say you"re & I'm <fine>`
	if ly.LRC != wantLRC {
		t.Fatalf("LRC = %q, want %q", ly.LRC, wantLRC)
	}
	wantTrans := `[00:10.00]别说你"好" & 我很好`
	if ly.Translation != wantTrans {
		t.Fatalf("Translation = %q, want %q", ly.Translation, wantTrans)
	}
	wantYRC := `[00:10.00](0,500)Don't`
	if ly.YRC != wantYRC {
		t.Fatalf("YRC = %q, want %q", ly.YRC, wantYRC)
	}
}

// Tests that CreatePlaylist sends Type: "normal" as required by Songloft API.
func TestCreatePlaylistTypeNormal(t *testing.T) {
	var receivedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/playlists", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		_ = json.NewEncoder(w).Encode(Playlist{
			ID:   12,
			Name: "New Playlist",
			Type: "normal",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cli, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	cli.accessToken = "dummy"

	pl, err := cli.CreatePlaylist(context.Background(), "New Playlist", "desc")
	if err != nil {
		t.Fatalf("CreatePlaylist failed: %v", err)
	}
	if pl.ID != 12 {
		t.Fatalf("pl.ID = %d, want 12", pl.ID)
	}
	if receivedBody["type"] != "normal" {
		t.Fatalf("received type = %v, want 'normal'", receivedBody["type"])
	}
	if receivedBody["name"] != "New Playlist" {
		t.Fatalf("received name = %v, want 'New Playlist'", receivedBody["name"])
	}
}
