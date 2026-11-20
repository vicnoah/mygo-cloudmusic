package pluginregistry

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const maxExpanded = 768 << 20
const maxArchiveFiles = 4096

type Version struct {
	Version   string
	Archive   string
	Entry     string
	SHA256    [32]byte
	Platforms []string
}

type Package struct {
	Source   string
	Key      string
	Current  Version
	Previous *Version
}

type Program struct {
	Path string
	Version
}

type Registry struct {
	dir    string
	client *http.Client
	mu     sync.Mutex
}

func New(dir string, client *http.Client) (*Registry, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("plugin registry requires an absolute path")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Registry{dir: dir, client: client}, nil
}

// Install stages a signed release, checks the program while still staged,
// and publishes the new version by replacing the local index atomically.
func (r *Registry) Install(ctx context.Context, source Source, probe func(context.Context, Program) error) (Program, error) {
	if probe == nil {
		return Program{}, errors.New("plugin install requires a compatibility check")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index, err := r.read()
	if err != nil {
		return Program{}, err
	}
	record, installed := index[source.Package]
	if installed && record.Source != source.Catalog {
		return Program{}, errors.New("plugin release is from a different source or key")
	}
	entryInfo, err := source.lookup(ctx, r.client)
	if err != nil {
		return Program{}, err
	}
	key := sha256.Sum256(entryInfo.PublicKey)
	keyID := hex.EncodeToString(key[:])
	if installed && record.Key != keyID {
		return Program{}, errors.New("plugin release is from a different source or key")
	}
	release, err := entryInfo.release(ctx, r.client)
	if err != nil {
		return Program{}, err
	}
	artifact, err := release.artifact(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Program{}, err
	}
	entry, err := localArchivePath(artifact.Entry)
	if err != nil || artifact.Entry == "." ||
		(runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(entry), ".exe")) {
		return Program{}, errors.New("invalid executable path in plugin manifest")
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return Program{}, err
	}
	staging, err := os.MkdirTemp(r.dir, ".stage-")
	if err != nil {
		return Program{}, err
	}
	defer os.RemoveAll(staging)
	if err := r.download(ctx, artifact, filepath.Join(staging, "download.zip")); err != nil {
		return Program{}, err
	}
	extracted := filepath.Join(staging, "files")
	if err := extractArchive(filepath.Join(staging, "download.zip"), extracted); err != nil {
		return Program{}, err
	}
	expected, _ := checksum(artifact.EntrySHA256)
	archive, _ := checksum(artifact.SHA256)
	version := Version{
		Version: release.Version, Archive: hex.EncodeToString(archive[:]),
		Entry: artifact.Entry, SHA256: expected, Platforms: release.Entries,
	}
	p := Program{Path: filepath.Join(extracted, entry), Version: version}
	if err := checkProgram(p, probe, ctx); err != nil {
		return Program{}, err
	}
	finalDir := r.versionDir(source.Package, version)
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o700); err != nil {
		return Program{}, err
	}
	if err := os.Rename(extracted, finalDir); err != nil {
		// A previously verified artifact can still be present after
		// rollback, removal from the index, or an interrupted save.
		p.Path = filepath.Join(finalDir, entry)
		if existing := checkProgram(p, probe, ctx); existing != nil {
			return Program{}, fmt.Errorf("publish plugin release: %w", err)
		}
	}
	if !installed {
		record = Package{Source: source.Catalog, Key: keyID}
	} else if version.Archive != record.Current.Archive || version.Version != record.Current.Version {
		previous := record.Current
		record.Previous = &previous
	}
	record.Current = version
	index[source.Package] = record
	if err := ctx.Err(); err != nil {
		return Program{}, err
	}
	if err := r.write(index); err != nil {
		return Program{}, err
	}
	return Program{Path: filepath.Join(finalDir, entry), Version: version}, nil
}

func (r *Registry) Current(id string) (Program, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index, err := r.read()
	if err != nil {
		return Program{}, false, err
	}
	item, ok := index[id]
	if !ok {
		return Program{}, false, nil
	}
	p, err := r.program(id, item.Current)
	return p, err == nil, err
}

// Rollback rechecks the previous version before restoring its pointer.
func (r *Registry) Rollback(ctx context.Context, id string, probe func(context.Context, Program) error) (Program, error) {
	if probe == nil {
		return Program{}, errors.New("plugin rollback requires a compatibility check")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index, err := r.read()
	if err != nil {
		return Program{}, err
	}
	item, ok := index[id]
	if !ok || item.Previous == nil {
		return Program{}, errors.New("no previous plugin version")
	}
	p, err := r.program(id, *item.Previous)
	if err != nil {
		return Program{}, err
	}
	if err := checkProgram(p, probe, ctx); err != nil {
		return Program{}, err
	}
	previous := item.Current
	item.Current, item.Previous = *item.Previous, &previous
	index[id] = item
	if err := r.write(index); err != nil {
		return Program{}, err
	}
	return p, nil
}

// Unregister removes a plugin from the active index and preserves its data.
// Version directories can still be in use until the old process exits.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	index, err := r.read()
	if err != nil {
		return err
	}
	delete(index, id)
	return r.write(index)
}

func (r *Registry) program(id string, version Version) (Program, error) {
	entry, err := localArchivePath(version.Entry)
	_, archiveErr := checksum(version.Archive)
	if !safeID(id) || !safeID(version.Version) || err != nil || archiveErr != nil {
		return Program{}, errors.New("invalid stored plugin version")
	}
	return Program{Path: filepath.Join(r.versionDir(id, version), entry), Version: version}, nil
}

func (r *Registry) versionDir(id string, version Version) string {
	return filepath.Join(r.dir, "releases", id, version.Version+"-"+version.Archive)
}

func checkProgram(p Program, probe func(context.Context, Program) error, ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(p.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return errors.New("plugin executable is not a regular file")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if [32]byte(h.Sum(nil)) != p.SHA256 {
		return errors.New("plugin executable checksum differs from manifest")
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(p.Path, 0o700); err != nil {
		return err
	}
	return probe(ctx, p)
}

func (r *Registry) read() (map[string]Package, error) {
	index := make(map[string]Package)
	b, err := os.ReadFile(filepath.Join(r.dir, "index.json"))
	if os.IsNotExist(err) {
		return index, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &index); err != nil {
		return nil, err
	}
	if index == nil {
		return nil, errors.New("invalid plugin registry")
	}
	return index, nil
}

func (r *Registry) write(index map[string]Package) error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(r.dir, ".index-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	encodeErr := json.NewEncoder(f).Encode(index)
	closeErr := f.Close()
	if err := errors.Join(encodeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(r.dir, "index.json"))
}

func (r *Registry) download(ctx context.Context, artifact Artifact, dest string) error {
	response, err := get(ctx, r.client, artifact.URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, h), io.LimitReader(response.Body, artifact.Size+1))
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	want, _ := checksum(artifact.SHA256)
	if n != artifact.Size || [32]byte(h.Sum(nil)) != want {
		return errors.New("downloaded plugin archive differs from manifest")
	}
	return nil
}

func localArchivePath(path string) (string, error) {
	if !fs.ValidPath(path) || strings.Contains(path, "\\") {
		return "", errors.New("invalid path in plugin archive")
	}
	return filepath.Localize(path)
}

func extractArchive(archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) > maxArchiveFiles {
		return errors.New("plugin archive has too many entries")
	}
	var remaining uint64 = maxExpanded
	for _, f := range z.File {
		path, err := localArchivePath(strings.TrimSuffix(f.Name, "/"))
		if err != nil || path == "." || f.UncompressedSize64 > remaining {
			return errors.New("invalid or oversized entry in plugin archive")
		}
		name := filepath.Join(dest, path)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(name, 0o700); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return errors.New("plugin archive contains a non-regular file")
		}
		if err := extractFile(f, name); err != nil {
			return err
		}
		remaining -= f.UncompressedSize64
	}
	return nil
}

func extractFile(source *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	input, err := source.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(input, int64(source.UncompressedSize64)+1))
	if n != int64(source.UncompressedSize64) {
		copyErr = errors.Join(copyErr, errors.New("plugin archive size mismatch"))
	}
	return errors.Join(copyErr, out.Close())
}
