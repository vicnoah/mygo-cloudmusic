// Package storage provides instance-scoped paths and credentials for music plugins.
package storage

import (
	"encoding/hex"
	"io/fs"
	"path/filepath"

	"github.com/egoist/mygo"

	"waygu-music/internal/pluginapi"
)

// Scope identifies the files and credentials of one registered plugin instance.
type Scope struct{ id pluginapi.ID }

func For(id pluginapi.ID) Scope { return Scope{id: id} }

func (s Scope) DataPath(name string) (string, error) {
	return s.path(mygo.PathUserData, name)
}

func (s Scope) CachePath(name string) (string, error) {
	return s.path(mygo.PathCache, name)
}

func (s Scope) path(root mygo.PathName, name string) (string, error) {
	if s.id == "" || !filepath.IsLocal(name) || filepath.Clean(name) == "." {
		return "", fs.ErrInvalid
	}
	return appPath(root, filepath.Join("plugins", s.directory(), name))
}

// Use the same directory for an ID on case-sensitive and case-insensitive filesystems.
func (s Scope) directory() string { return hex.EncodeToString([]byte(s.id)) }

// LegacyDataPath locates an existing file in the app's original data layout.
func LegacyDataPath(name string) (string, error) {
	return appPath(mygo.PathUserData, name)
}

// LegacyCachePath locates an existing file in the app's original cache layout.
func LegacyCachePath(name string) (string, error) {
	return appPath(mygo.PathCache, name)
}

func appPath(root mygo.PathName, name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fs.ErrInvalid
	}
	dir, err := mygo.App.Path(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
