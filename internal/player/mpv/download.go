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
)

var (
	baseDownloadURL = "https://github.com/vicnoah/mygo-cloudmusic/releases/download/mpv-components-v1/"
	releaseKey, _   = base64.StdEncoding.DecodeString("rTpebkc591D+UvB6e7hP/WigaCvcqExcyG7m5PREahM=")
	httpClient      = &http.Client{Timeout: 5 * time.Minute}
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
	installedPath = library
	loadFailed = false
	return nil
}

func fetchRelease(ctx context.Context) (releaseIndex, error) {
	body, err := openDownload(ctx, "index.json")
	if err != nil {
		return releaseIndex{}, err
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, maxManifestSize+1))
	if err != nil {
		return releaseIndex{}, err
	}
	if len(raw) > maxManifestSize {
		return releaseIndex{}, errors.New("mpv: oversized release index")
	}
	return parseRelease(raw, releaseKey)
}

func fetchArchive(ctx context.Context, asset releaseAsset, file *os.File, onProgress func(done, total int64)) error {
	body, err := openDownload(ctx, asset.Archive)
	if err != nil {
		return err
	}
	defer body.Close()
	hash := sha256.New()
	progress := &progressReader{r: io.LimitReader(body, asset.Size+1), total: asset.Size, onProgress: onProgress}
	size, err := io.Copy(io.MultiWriter(file, hash), progress)
	if err != nil {
		return fmt.Errorf("mpv: download %s: %w", asset.Archive, err)
	}
	if size != asset.Size || hex.EncodeToString(hash.Sum(nil)) != asset.SHA256 {
		return errors.New("mpv: download size or checksum mismatch")
	}
	return nil
}

func openDownload(ctx context.Context, name string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseDownloadURL+name, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mpv: download %s: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("mpv: download %s: HTTP %s", name, resp.Status)
	}
	return resp.Body, nil
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
	installedPath = ""
	loadFailed = false
	var inUse string
	if loadedPath != "" {
		loadDisabled = true
		inUse = filepath.Dir(loadedPath)
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
