package mpv

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	baseDownloadURL  = "https://github.com/vicnoah/mygo-cloudmusic/releases/download/libmpv-" + LibVersion + "/"
	cacheDirOverride string
)

// SetBaseURL overrides the download release base URL (useful for testing or mirrors).
func SetBaseURL(u string) {
	baseDownloadURL = u
}

// SetCacheDir overrides the cache directory for testing.
func SetCacheDir(d string) {
	cacheDirOverride = d
}

// CacheDir returns the directory path where downloaded libmpv binaries are kept.
func CacheDir() (string, error) {
	if cacheDirOverride != "" {
		return cacheDirOverride, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "mygo-cloudmusic", "libmpv-"+LibVersion), nil
}

// TargetFile returns the target library file path inside CacheDir for the current platform.
func TargetFile() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	asset, ok := NativeAssets[Platform()]
	if !ok {
		return "", fmt.Errorf("mpv: unsupported platform %s", Platform())
	}
	return filepath.Join(dir, asset.Name), nil
}

// IsDownloaded reports whether the library has already been downloaded to the cache directory.
func IsDownloaded() bool {
	p, err := TargetFile()
	if err != nil {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.Size() > 1024
}

// SystemPath searches system paths to see if libmpv is already installed on the host.
func SystemPath() string {
	for _, name := range SystemLibNames() {
		h, err := openLibrary(name)
		if err == nil && h != 0 {
			// On Unix/Windows, test if mpv_create exists
			if sym, _ := librarySymbol(h, "mpv_create"); sym != 0 {
				return name
			}
		}
	}
	return ""
}

// Ready reports whether libmpv is available to use (either system-installed or downloaded).
func Ready() bool {
	return IsDownloaded() || SystemPath() != ""
}

var (
	dlMu   sync.Mutex
	cancel context.CancelFunc
)

// Download downloads and unpacks the platform's libmpv binary into CacheDir.
func Download(ctx context.Context, onProgress func(done, total int64)) error {
	dlMu.Lock()
	defer dlMu.Unlock()

	asset, ok := NativeAssets[Platform()]
	if !ok {
		return fmt.Errorf("mpv: no precompiled asset for platform %s", Platform())
	}

	dir, err := CacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mpv: mkdir cache: %w", err)
	}

	targetPath := filepath.Join(dir, asset.Name)
	downloadURL := baseDownloadURL + asset.Asset

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("mpv: fetch %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mpv: download failed: HTTP %s", resp.Status)
	}

	total := resp.ContentLength
	if total <= 0 {
		total = asset.GzSize
	}

	// Decompress gzip on the fly
	gzr, err := gzip.NewReader(&progressReader{
		r:          resp.Body,
		total:      total,
		onProgress: onProgress,
	})
	if err != nil {
		return fmt.Errorf("mpv: gzip reader: %w", err)
	}
	defer gzr.Close()

	tmpFile := targetPath + ".part"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	if _, err := io.Copy(mw, gzr); err != nil {
		f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("mpv: unpack: %w", err)
	}
	_ = f.Sync()
	f.Close()

	if asset.SHA256 != "" {
		sum := hex.EncodeToString(hasher.Sum(nil))
		if sum != asset.SHA256 {
			_ = os.Remove(tmpFile)
			return errors.New("mpv: checksum mismatch for downloaded library")
		}
	}

	if err := os.Rename(tmpFile, targetPath); err != nil {
		return err
	}

	return nil
}

// Delete removes the downloaded libmpv files from the cache directory.
func Delete() error {
	dlMu.Lock()
	defer dlMu.Unlock()
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

type progressReader struct {
	r          io.Reader
	done       int64
	total      int64
	onProgress func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.onProgress != nil {
		p.onProgress(p.done, p.total)
	}
	return n, err
}
