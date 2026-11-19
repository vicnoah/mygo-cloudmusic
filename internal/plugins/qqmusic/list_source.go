package qqmusic

import "waygu-music/internal/pluginapi"

// trackList exposes static or paged results to the local track view.
type trackList struct {
	Total  func() int
	Len    func() int
	At     func(int) *pluginapi.Track
	Retry  func()
	Failed func() bool
}
