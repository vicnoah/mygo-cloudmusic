package player

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"waygu-music/internal/pluginapi"
)

// Cache keeps the songs played on disk, so that they play again at once,
// and without the network. A filename identifies the platform instance,
// original track reference, track format, and quality. The stream metadata
// lives next to the audio. Once they take more than the limit, those
// played longest ago go first.
type Cache struct {
	dir string

	mu    sync.Mutex
	limit int64 // in bytes; 0 keeps nothing
}

// NewCache returns a cache in dir of limit bytes.
func NewCache(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit}
}

// SetLimit changes how much the cache may take, and drops what is over.
func (c *Cache) SetLimit(limit int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limit = limit
	c.pruneLocked("")
}

type trackIdentity struct {
	platform pluginapi.ID
	ref      string
}

func identifyTrack(owner pluginapi.ID, t *pluginapi.Track) trackIdentity {
	ref := t.Ref
	if ref == "" {
		ref = strconv.FormatInt(t.ID, 10)
	}
	return trackIdentity{platform: owner, ref: ref}
}

type cacheTrack struct {
	trackIdentity
	format string
}

func trackCacheRef(owner pluginapi.ID, t *pluginapi.Track) cacheTrack {
	return cacheTrack{trackIdentity: identifyTrack(owner, t), format: strings.ToLower(t.Format)}
}

func trackFingerprint(track cacheTrack) [sha256.Size]byte {
	identity := fmt.Sprintf("%q/%q/%q", track.platform, track.ref, track.format)
	return sha256.Sum256([]byte(identity))
}

func cacheKey(track cacheTrack, level string) string {
	return fmt.Sprintf("v2-%x-%s", trackFingerprint(track), level)
}

// keepable reports whether a song of level and type ext may be kept.
func keepable(level, ext string) bool {
	return slices.Contains(pluginapi.Levels, level) && (ext == "mp3" || ext == "flac")
}

// findLocked finds a complete cached song at the requested quality or better.
func (c *Cache) findLocked(track cacheTrack, level string) (string, *pluginapi.Stream) {
	at := max(slices.Index(pluginapi.Levels, level), 0)
	for _, lv := range pluginapi.Levels[at:] {
		key := cacheKey(track, lv)
		meta, err := os.ReadFile(filepath.Join(c.dir, key+".json"))
		if err != nil {
			continue
		}
		var st pluginapi.Stream
		if json.Unmarshal(meta, &st) != nil || !keepable(lv, st.Type) || st.Level != lv || st.Size <= 0 {
			continue
		}
		path := filepath.Join(c.dir, key+"."+st.Type)
		if fi, err := os.Stat(path); err == nil && fi.Size() == st.Size {
			return path, &st
		}
	}
	return "", nil
}

// filePath returns the cached song file used by MPV or DLNA.
func (c *Cache) filePath(track cacheTrack, level string) string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	path, _ := c.findLocked(track, level)
	return path
}

// lookup opens a completely cached song for decoding.
func (c *Cache) lookup(track cacheTrack, level string) (*download, *pluginapi.Stream, bool) {
	if c == nil {
		return nil, nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	path, st := c.findLocked(track, level)
	if st == nil {
		return nil, nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, false
	}
	// Played now: the last to go.
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return opened(f, st.Size), st, true
}

// promoteLegacy moves an old numeric cache entry only after the current
// resolver has identified its exact bytes.
func (c *Cache) promoteLegacy(ctx context.Context, track cacheTrack, id int64, info *pluginapi.Stream) bool {
	if c == nil || track.platform == "" || info == nil || info.VerifyCachedContent == nil || info.Size <= 0 || ctx.Err() != nil {
		return false
	}
	format := info.Type
	if format == "" {
		format = "mp3"
	}
	if !keepable(info.Level, format) {
		return false
	}
	key := fmt.Sprintf("%d-%s", id, info.Level)
	path := filepath.Join(c.dir, key+"."+format)
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != info.Size {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(stat, opened) || opened.Size() != info.Size {
		f.Close()
		return false
	}
	// Checking a lossless file can take a while. Leave other cache operations
	// free to run and abort the read if playback moves to a different song.
	stop := context.AfterFunc(ctx, func() { f.Close() })
	verified := info.VerifyCachedContent(f)
	stop()
	f.Close()
	if !verified {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return false
	}
	if existing, _ := c.findLocked(track, info.Level); existing != "" {
		return true
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(stat, current) || current.Size() != stat.Size() || !current.ModTime().Equal(stat.ModTime()) {
		return false
	}
	if _, kept := c.keepLocked(track, info, path, stat.Size()); !kept {
		return false
	}
	if published, _ := c.findLocked(track, info.Level); published == "" {
		return false
	}
	_ = os.Remove(filepath.Join(c.dir, key+".json"))
	return true
}

// download fetches the song of the stream st into a file, to keep once it
// has all come in; or into memory, if the cache would not keep it.
func (c *Cache) download(track cacheTrack, st *pluginapi.Stream) *download {
	part := c.part(st)
	if part == nil {
		return startDownloadWith(st.URL, st.Header, st.Size, nil, nil)
	}
	return startDownloadWith(st.URL, st.Header, st.Size, part, func(path string, n int64) (string, bool) {
		return c.keep(track, st, path, n)
	})
}

// part makes a file for a song of the stream st to come into, or returns
// nil if the cache would not keep it. Each has a name of its own: the same
// song played again while it still comes in has another.
func (c *Cache) part(st *pluginapi.Stream) *os.File {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	limit := c.limit
	c.mu.Unlock()
	// A song of more than a quarter of it would push out too many.
	if limit <= 0 || st.Size > limit/4 {
		return nil
	}
	if os.MkdirAll(c.dir, 0o700) != nil {
		return nil
	}
	f, err := os.CreateTemp(c.dir, "*.part")
	if err != nil {
		return nil
	}
	return f
}

// keep keeps the song of the stream st, n bytes, from the file part, which
// is closed, and drops what is over the limit then. It returns where the
// song is: kept, or still at part.
func (c *Cache) keep(track cacheTrack, st *pluginapi.Stream, part string, n int64) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.keepLocked(track, st, part, n)
}

func (c *Cache) keepLocked(track cacheTrack, st *pluginapi.Stream, part string, n int64) (string, bool) {
	if n <= 0 || n > c.limit/4 {
		return part, false
	}
	kept := *st
	if kept.Type == "" {
		kept.Type = "mp3"
	}
	kept.Size = n
	if !keepable(kept.Level, kept.Type) {
		return part, false
	}
	key := cacheKey(track, kept.Level)
	meta, err := json.Marshal(kept)
	if err != nil {
		return part, false
	}
	path := filepath.Join(c.dir, key+"."+kept.Type)
	metaPath := filepath.Join(c.dir, key+".json")

	// A simultaneous download may already have published this audio.
	if existing, stream := c.findLocked(track, kept.Level); existing == path && stream.Size == n {
		if os.Remove(part) == nil {
			now := time.Now()
			_ = os.Chtimes(path, now, now)
			return path, true
		}
		return part, false
	}

	staged := part + ".json"
	if os.WriteFile(staged, meta, 0o600) != nil {
		return part, false
	}
	defer os.Remove(staged)
	if os.Rename(part, path) != nil {
		return part, false
	}
	if os.Rename(staged, metaPath) != nil {
		if os.Rename(path, part) == nil {
			return part, false
		}
		return path, true
	}
	for _, ext := range []string{"mp3", "flac"} {
		if ext != kept.Type {
			_ = os.Remove(filepath.Join(c.dir, key+"."+ext))
		}
	}
	c.pruneLocked(path)
	return path, true
}

// Size returns how many bytes the songs kept take.
func (c *Cache) Size() int64 {
	var n int64
	for _, f := range c.songs() {
		n += f.size
	}
	return n
}

// Clear drops every song kept.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range c.songs() {
		c.remove(f.path)
	}
}

type cachedSong struct {
	path string
	size int64
	used time.Time
}

func (c *Cache) songs() []cachedSong {
	entries, _ := os.ReadDir(c.dir)
	var out []cachedSong
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".mp3" && ext != ".flac" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, cachedSong{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
	}
	return out
}

func (c *Cache) remove(path string) bool {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false
	}
	ext := filepath.Ext(path)
	meta := strings.TrimSuffix(path, ext) + ".json"
	// A previous write can leave an audio file in the other encoding.
	if data, err := os.ReadFile(meta); err == nil {
		var st pluginapi.Stream
		if json.Unmarshal(data, &st) != nil || "."+st.Type == ext {
			_ = os.Remove(meta)
		}
	}
	return true
}

// pruneLocked drops the songs played longest ago until the rest fit the
// limit, retaining the song just kept.
func (c *Cache) pruneLocked(keep string) {
	c.dropParts()
	songs := c.songs()
	slices.SortFunc(songs, func(a, b cachedSong) int { return a.used.Compare(b.used) })
	var total int64
	for _, f := range songs {
		total += f.size
	}
	for _, f := range songs {
		if total <= c.limit {
			return
		}
		if f.path != keep && c.remove(f.path) {
			total -= f.size
		}
	}
}

// dropParts removes the files of songs that never all came in, left by an
// app that quit as they came: one coming in is written to every moment.
func (c *Cache) dropParts() {
	parts, _ := filepath.Glob(filepath.Join(c.dir, "*.part"))
	for _, path := range parts {
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > time.Hour {
			os.Remove(path)
		}
	}
}
