package mpv

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"waygu-music/internal/githubdownload"
)

var (
	baseDownloadURL = "https://github.com/vicnoah/mygo-cloudmusic/releases/download/mpv-components-v1/"
	releaseKey, _   = base64.StdEncoding.DecodeString("rTpebkc591D+UvB6e7hP/WigaCvcqExcyG7m5PREahM=")
	httpClient      = githubdownload.Client()
)

// Download validates an official component in a staging directory before replacing the active record.
func Download(ctx context.Context, onProgress func(done, total int64)) error {
	dlMu.Lock()
	defer dlMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	index, err := fetchRelease(ctx)
	if err != nil {
		return err
	}
	var asset releaseAsset
	for _, candidate := range index.Assets {
		if candidate.Platform == Platform() {
			asset = candidate
			break
		}
	}
	if asset.Archive == "" {
		return fmt.Errorf("mpv: no official component for %s", Platform())
	}
	root, err := CacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	archive, err := os.CreateTemp(root, ".download-")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	if err := errors.Join(fetchArchive(ctx, asset, archive, onProgress), archive.Close()); err != nil {
		return err
	}

	stage, err := os.MkdirTemp(root, componentPrefix)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	manifest, err := unpackComponent(ctx, archive.Name(), stage, index.Version)
	if err != nil {
		return fmt.Errorf("mpv: unpack component: %w", err)
	}
	library := filepath.Join(stage, manifest.Library)
	if err := checkComponent(ctx, library, manifest); err != nil {
		return err
	}

	loadMu.Lock()
	defer loadMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeActive(root, installedComponent{Directory: filepath.Base(stage), Manifest: manifest}); err != nil {
		return err
	}
	keep = true
	activateInstalled(library, manifest)
	return nil
}

func fetchRelease(ctx context.Context) (releaseIndex, error) {
	var index releaseIndex
	err := githubdownload.Try(ctx, httpClient, baseDownloadURL+"index.json", func(resp *http.Response) error {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize+1))
		if err != nil {
			return err
		}
		if len(raw) > maxManifestSize {
			return errors.New("mpv: oversized release index")
		}
		index, err = parseRelease(raw, releaseKey)
		return err
	})
	return index, err
}

func fetchArchive(ctx context.Context, asset releaseAsset, file *os.File, onProgress func(done, total int64)) error {
	return githubdownload.Try(ctx, httpClient, baseDownloadURL+asset.Archive, func(resp *http.Response) error {
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if onProgress != nil {
			onProgress(0, asset.Size)
		}
		hash := sha256.New()
		progress := &progressReader{r: io.LimitReader(resp.Body, asset.Size+1), total: asset.Size, onProgress: onProgress}
		size, err := io.Copy(io.MultiWriter(file, hash), progress)
		if err != nil {
			return err
		}
		if size != asset.Size || hex.EncodeToString(hash.Sum(nil)) != asset.SHA256 {
			return errors.New("mpv: download size or checksum mismatch")
		}
		return nil
	})
}

// Delete deactivates the component immediately and leaves an in-use directory for startup cleanup.
func Delete() error {
	dlMu.Lock()
	defer dlMu.Unlock()
	root, err := CacheDir()
	if err != nil {
		return err
	}
	loadMu.Lock()
	defer loadMu.Unlock()
	if err := os.Remove(filepath.Join(root, "active.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	installed = componentLibrary{}
	loadFailed = false
	var inUse string
	if mapped.path != "" {
		loadDisabled = true
		inUse = filepath.Dir(mapped.path)
	}
	return cleanComponents(root, "", inUse)
}

type progressReader struct {
	r          io.Reader
	done       int64
	total      int64
	onProgress func(done, total int64)
	lastUpdate time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if n > 0 && p.onProgress != nil && (p.done >= p.total || time.Since(p.lastUpdate) >= 100*time.Millisecond) {
		p.lastUpdate = time.Now()
		p.onProgress(min(p.done, p.total), p.total)
	}
	return n, err
}
