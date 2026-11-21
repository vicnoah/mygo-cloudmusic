package mpv

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	componentFormat = 1
	maxArchiveSize  = 256 << 20
	maxExpandedSize = 768 << 20
	maxManifestSize = 256 << 10
	componentPrefix = "package-"
)

var (
	dlMu             sync.Mutex
	cacheDirOverride string
	componentProbe   = runComponentProbe
	componentName    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,159}$`)
)

type componentFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type componentManifest struct {
	Format         int             `json:"format"`
	Version        string          `json:"version"`
	MPVVersion     string          `json:"mpvVersion"`
	Platform       string          `json:"platform"`
	MinimumRuntime string          `json:"minimumRuntime"`
	Library        string          `json:"library"`
	Files          []componentFile `json:"files"`
}

type installedComponent struct {
	Directory string            `json:"directory"`
	Manifest  componentManifest `json:"manifest"`
}

type releaseAsset struct {
	Platform string `json:"platform"`
	Archive  string `json:"archive"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

type releaseIndex struct {
	Format  int            `json:"format"`
	Version string         `json:"version"`
	Assets  []releaseAsset `json:"assets"`
}

type signedIndex struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

// CacheDir returns the user-owned root for versioned decoder downloads.
func CacheDir() (string, error) {
	if cacheDirOverride != "" {
		return cacheDirOverride, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "mygo-cloudmusic", "mpv-components"), nil
}

func validHash(hash string) bool {
	sum, err := hex.DecodeString(hash)
	return err == nil && len(sum) == sha256.Size && hash == strings.ToLower(hash)
}

func parseRelease(raw []byte, key ed25519.PublicKey) (releaseIndex, error) {
	var envelope signedIndex
	var index releaseIndex
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return index, fmt.Errorf("mpv: read release index: %w", err)
	}
	if len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, envelope.Payload, envelope.Signature) {
		return index, errors.New("mpv: invalid release signature")
	}
	if err := json.Unmarshal(envelope.Payload, &index); err != nil {
		return index, fmt.Errorf("mpv: read signed release: %w", err)
	}
	if index.Format != componentFormat || !componentName.MatchString(index.Version) {
		return index, errors.New("mpv: incompatible release index")
	}
	seen := make(map[string]bool)
	for _, asset := range index.Assets {
		expected := "mpv-" + index.Version + "-" + asset.Platform + ".zip"
		if !componentName.MatchString(asset.Platform) || !componentName.MatchString(asset.Archive) ||
			asset.Archive != expected || asset.Size <= 0 || asset.Size > maxArchiveSize ||
			!validHash(asset.SHA256) || seen[asset.Platform] {
			return index, errors.New("mpv: invalid asset in release index")
		}
		seen[asset.Platform] = true
	}
	return index, nil
}

func (manifest componentManifest) validate(version string) error {
	if manifest.Format != componentFormat || manifest.Version != version ||
		!componentName.MatchString(manifest.Version) || manifest.Platform != Platform() ||
		manifest.MPVVersion == "" || manifest.MinimumRuntime == "" ||
		!componentName.MatchString(manifest.Library) || len(manifest.Files) == 0 || len(manifest.Files) > 256 {
		return errors.New("mpv: incompatible component manifest")
	}

	seen := map[string]bool{"component.json": true}
	var total int64
	foundLibrary := false
	for _, file := range manifest.Files {
		key := strings.ToLower(file.Name)
		if !componentName.MatchString(file.Name) || strings.HasSuffix(file.Name, ".") ||
			seen[key] || file.Size < 0 || file.Size > maxExpandedSize-total || !validHash(file.SHA256) {
			return fmt.Errorf("mpv: invalid component file %q", file.Name)
		}
		total += file.Size
		seen[key] = true
		foundLibrary = foundLibrary || file.Name == manifest.Library && file.Size > 0
	}
	if !foundLibrary {
		return errors.New("mpv: missing entry library in component manifest")
	}
	slices.SortFunc(manifest.Files, func(a, b componentFile) int { return strings.Compare(a.Name, b.Name) })
	return nil
}

func unpackComponent(ctx context.Context, archivePath, dir, version string) (componentManifest, error) {
	var manifest componentManifest
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return manifest, err
	}
	defer archive.Close()

	entries := make(map[string]*zip.File)
	for _, file := range archive.File {
		key := strings.ToLower(file.Name)
		if !componentName.MatchString(file.Name) || !file.Mode().IsRegular() || entries[key] != nil {
			return manifest, fmt.Errorf("mpv: invalid ZIP entry %q", file.Name)
		}
		entries[key] = file
	}

	meta := entries["component.json"]
	if meta == nil || meta.UncompressedSize64 > maxManifestSize {
		return manifest, errors.New("mpv: missing or oversized component manifest")
	}
	data, err := readSmall(meta)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if err := manifest.validate(version); err != nil {
		return manifest, err
	}
	if len(entries) != len(manifest.Files)+1 {
		return manifest, errors.New("mpv: ZIP entries differ from the component manifest")
	}
	for _, expected := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return manifest, err
		}
		entry := entries[strings.ToLower(expected.Name)]
		if entry == nil || entry.Name != expected.Name || entry.UncompressedSize64 != uint64(expected.Size) {
			return manifest, fmt.Errorf("mpv: invalid archive entry %s", expected.Name)
		}
		if err := extractFile(entry, expected, filepath.Join(dir, expected.Name)); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}

func readSmall(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
}

func extractFile(entry *zip.File, expected componentFile, path string) error {
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, expected.Size+1))
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return fmt.Errorf("mpv: extract %s: %w", expected.Name, err)
	}
	if size != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("mpv: checksum mismatch for %s", expected.Name)
	}
	return nil
}

func writeActive(root string, component installedComponent) error {
	tmp, err := os.CreateTemp(root, ".active-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	writeErr := json.NewEncoder(tmp).Encode(component)
	if err := errors.Join(writeErr, tmp.Sync(), tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(root, "active.json"))
}

func cleanComponents(root, keep, loaded string) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var cleanupErr error
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(root, name)
		staleArchive := strings.HasPrefix(name, ".download-") || strings.HasPrefix(name, ".active-")
		unusedPackage := strings.HasPrefix(name, componentPrefix) && name != keep && path != loaded
		if staleArchive || unusedPackage {
			cleanupErr = errors.Join(cleanupErr, os.RemoveAll(path))
		}
	}
	return cleanupErr
}

func runComponentProbe(ctx context.Context, library string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, executable, "--mpv-probe", library)
	configureProbe(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("mpv: load probe failed (%w): %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func checkComponent(ctx context.Context, library string, manifest componentManifest) error {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if err := componentProbe(ctx, library); err != nil {
		return fmt.Errorf("解码组件 %s（最低要求 %s）: %w", manifest.MPVVersion, manifest.MinimumRuntime, err)
	}
	return nil
}

// Restore validates the last installed component and removes versions left by previous processes.
func Restore(ctx context.Context) error {
	dlMu.Lock()
	defer dlMu.Unlock()
	root, err := CacheDir()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "active.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cleanComponents(root, "", "")
	}
	if err != nil {
		return err
	}
	var record installedComponent
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	if !componentName.MatchString(record.Directory) || !strings.HasPrefix(record.Directory, componentPrefix) {
		return errors.New("mpv: invalid installed component directory")
	}
	if err := record.Manifest.validate(record.Manifest.Version); err != nil {
		return err
	}
	library := filepath.Join(root, record.Directory, record.Manifest.Library)
	if err := checkComponent(ctx, library, record.Manifest); err != nil {
		return err
	}
	loadMu.Lock()
	activateInstalled(library, record.Manifest)
	loadMu.Unlock()
	return cleanComponents(root, record.Directory, "")
}
