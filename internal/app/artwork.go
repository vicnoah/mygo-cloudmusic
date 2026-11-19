package app

import (
	"image"
	"slices"
	"time"
	"weak"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	cover "waygu-music/internal/pluginapi/artwork"
)

// artwork is a cover, decoded and ready to draw, with what the player
// derives from it: its color, and a blurred copy to fill a backdrop.
type artwork struct {
	bmp      *ui.Bitmap
	backdrop *ui.Bitmap
	tint     ui.Color // the cover's most vivid color
	failed   bool
	used     time.Time // when it was last asked for
	size     int       // the bytes it keeps: its data, and the backdrop's pixels
}

// artCache loads covers on goroutines and keeps them, up to artLimit
// bytes: their pixels on the GPU while they show, and on the CPU only
// their data, which takes a fraction of the pixels. Its methods run on
// the main thread; post runs a function there.
type artCache struct {
	art map[string]*artwork
	// gone are the covers dropped, for as long as something else holds
	// them: Now Playing keeps the cover it shows, which comes back as it
	// was rather than loading again.
	gone  map[string]weak.Pointer[artwork]
	busy  map[string]bool
	sem   chan struct{}
	post  func(func())
	now   func() time.Time
	size  int // the bytes the covers in art keep
	limit int
	artPlatformState
}

// artLimit is how many bytes of covers the cache keeps. A cover keeps its
// data, some 50–100 KB for Now Playing's and a few for a row's, so this
// is thousands: going back to a page shows its covers as they were,
// however long ago it showed them, rather than loading them again.
const artLimit = 64 << 20

func newArtCache(post func(func())) *artCache {
	return &artCache{art: map[string]*artwork{}, gone: map[string]weak.Pointer[artwork]{}, busy: map[string]bool{}, sem: make(chan struct{}, 8), post: post, now: time.Now, limit: artLimit, artPlatformState: newArtPlatformState()}
}

// add keeps a cover, and drops those asked for least recently once the
// covers take more than the limit, down to three quarters of it, so that
// it sorts them seldom. Those showing were asked for last, and stay.
func (c *artCache) add(key string, a *artwork) {
	c.art[key] = a
	c.size += a.size
	if c.size <= c.limit {
		return
	}
	for key, p := range c.gone {
		if p.Value() == nil {
			delete(c.gone, key)
		}
	}
	keys := make([]string, 0, len(c.art))
	for key := range c.art {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(x, y string) int { return c.art[x].used.Compare(c.art[y].used) })
	for _, key := range keys {
		if c.size <= c.limit*3/4 {
			break
		}
		a := c.art[key]
		delete(c.art, key)
		c.size -= a.size
		c.gone[key] = weak.Make(a)
	}
}

// get returns the cover under cacheKey, or nil while it loads from url.
func (c *artCache) get(cacheKey, url string, backdrop bool) *artwork {
	if url == "" {
		return nil
	}
	key := cacheKey
	if backdrop {
		key += "#backdrop"
	}
	now := c.now()
	if a, ok := c.art[key]; ok {
		a.used = now
		return a
	}
	if a := c.gone[key].Value(); a != nil {
		delete(c.gone, key)
		a.used = now
		c.add(key, a)
		return a
	}
	if c.busy[key] {
		return nil
	}
	c.busy[key] = true
	gen := c.artworkGeneration()
	var fetch func(string) ([]byte, error)
	if c.fetch != nil {
		fetch = c.fetch()
	}
	go func() {
		c.sem <- struct{}{}
		bmp, img, size, err := loadArtWith(url, fetch)
		<-c.sem
		var blurred *image.RGBA
		var tint ui.Color
		if err == nil {
			tint = cover.DominantColor(img)
			if backdrop {
				blurred = cover.Blur(img)
			}
		}
		c.post(func() {
			delete(c.busy, key)
			if c.discardStaleArtwork(gen, bmp) {
				return
			}
			if err != nil {
				c.add(key, &artwork{failed: true, used: c.now()})
				return
			}
			a := &artwork{bmp: bmp, tint: tint, used: c.now(), size: size}
			if blurred != nil {
				a.backdrop = ui.NewBitmap(blurred)
				a.size += len(blurred.Pix)
			}
			c.add(key, a)
			// Shown by the frame post draws, the cover drops its pixels,
			// which its data decodes again if they are needed.
			time.AfterFunc(artRelease, func() { mygo.RunOnMain(bmp.ReleasePixels) })
		})
	}()
	return nil
}

// artRelease is how long a cover keeps its pixels once loaded: past the
// frame that first shows it, and the smaller copies it shows from there.
const artRelease = 2 * time.Second
