package subsonic

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"netease-music-mygo/internal/netease"
)

// FolderNode represents a directory or music folder in the folder view.
type FolderNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDir     bool   `json:"isDir"`
	ItemCount int    `json:"itemCount"`
}

// FolderPage represents the contents of a directory (folders and tracks).
type FolderPage struct {
	ID       string
	Name     string
	ParentID string
	Folders  []FolderNode
	Tracks   []netease.Track
}

// FolderPage fetches the folders and tracks within the specified directory ID.
// When id is empty, it returns the top-level music folders or root indexes.
func (c *Client) FolderPage(ctx context.Context, id string) (FolderPage, error) {
	sc := c.Subsonic()
	if sc == nil {
		return FolderPage{}, ErrSignedOut
	}

	id = strings.TrimSpace(id)
	if id == "" {
		// Root: return music folders
		folders, err := sc.GetMusicFolders()
		if err != nil {
			return FolderPage{}, err
		}
		var nodes []FolderNode
		for _, f := range folders {
			nodes = append(nodes, FolderNode{
				ID:    fmt.Sprintf("folder:%d", f.ID),
				Name:  f.Name,
				IsDir: true,
			})
		}
		return FolderPage{
			ID:      "",
			Name:    "文件夹",
			Folders: nodes,
		}, nil
	}

	// If it's a music folder root: e.g. "folder:1"
	if strings.HasPrefix(id, "folder:") {
		folderIDStr := strings.TrimPrefix(id, "folder:")
		params := map[string]string{}
		if folderID, err := strconv.Atoi(folderIDStr); err == nil && folderID > 0 {
			params["musicFolderId"] = folderIDStr
		}
		indexes, err := sc.GetIndexes(params)
		if err != nil {
			return FolderPage{}, err
		}
		var nodes []FolderNode
		for _, idx := range indexes.Index {
			for _, ar := range idx.Artist {
				nodes = append(nodes, FolderNode{
					ID:    ar.ID,
					Name:  ar.Name,
					IsDir: true,
				})
			}
		}
		var tracks []netease.Track
		for _, ch := range indexes.Child {
			if ch.IsDir {
				nodes = append(nodes, FolderNode{
					ID:    ch.ID,
					Name:  ch.Title,
					IsDir: true,
				})
			} else {
				if tr, ok := ToTrackWithPlatform(c.PlatformID(), ch); ok {
					tracks = append(tracks, tr)
				}
			}
		}
		return FolderPage{
			ID:       id,
			Name:     "音乐目录",
			ParentID: "",
			Folders:  nodes,
			Tracks:   tracks,
		}, nil
	}

	// Normal directory ID
	dir, err := sc.GetMusicDirectory(id)
	if err != nil {
		return FolderPage{}, err
	}

	var nodes []FolderNode
	var tracks []netease.Track
	for _, ch := range dir.Child {
		if ch.IsDir {
			nodes = append(nodes, FolderNode{
				ID:    ch.ID,
				Name:  ch.Title,
				IsDir: true,
			})
		} else {
			if tr, ok := ToTrackWithPlatform(c.PlatformID(), ch); ok {
				tracks = append(tracks, tr)
			}
		}
	}

	return FolderPage{
		ID:       dir.ID,
		Name:     dir.Name,
		ParentID: dir.Parent,
		Folders:  nodes,
		Tracks:   tracks,
	}, nil
}
