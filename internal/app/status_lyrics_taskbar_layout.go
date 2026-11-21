// Explorer taskbar gap selection is adapted from mygo-qyin (AGPL-3.0).
// https://github.com/Amamiya23/mygo-qyin/blob/81512dc/internal/app/taskbar_lyrics_layout.go

package app

import "sort"

// Native placement uses physical pixels in the Explorer parent's client space.
type taskbarLyricsPlacement struct {
	parent              uintptr
	x, y, width, height int
	dpi                 uint32
	dark                bool
}

type taskbarLyricsInterval struct{ start, end int }

const taskbarLyricsEdgeGap = 8

// Find a real gap without moving Explorer's buttons. No occupancy means unknown.
func taskbarLyricsFreeSpace(length, requested, minimum, gap int, occupied []taskbarLyricsInterval) (int, int) {
	if length <= 0 || requested < minimum || minimum <= 0 || gap < 0 || len(occupied) == 0 {
		return 0, 0
	}
	blocked := make([]taskbarLyricsInterval, 0, len(occupied))
	for _, item := range occupied {
		if item.end <= item.start || item.end <= 0 || item.start >= length {
			continue
		}
		blocked = append(blocked, taskbarLyricsInterval{max(0, item.start-gap), min(length, item.end+gap)})
	}
	if len(blocked) == 0 {
		return 0, 0
	}
	sort.Slice(blocked, func(i, j int) bool { return blocked[i].start < blocked[j].start })
	// Prefer the leftmost usable space, even when another gap would be wider.
	// Keep the same margin at the outer taskbar edges as around its controls.
	cursor := gap
	for _, item := range blocked {
		if width := min(requested, item.start-cursor); width >= minimum {
			return cursor, width
		}
		cursor = max(cursor, item.end)
	}
	if width := min(requested, length-gap-cursor); width >= minimum {
		return cursor, width
	}
	return 0, 0
}

// Right-anchored placement mirrors the taskbar so the same scan picks the
// rightmost usable gap beside the notification area, then mirrors it back.
func taskbarLyricsFreeSpaceRight(length, requested, minimum, gap int, occupied []taskbarLyricsInterval) (int, int) {
	mirrored := make([]taskbarLyricsInterval, len(occupied))
	for i, item := range occupied {
		mirrored[i] = taskbarLyricsInterval{length - item.end, length - item.start}
	}
	x, width := taskbarLyricsFreeSpace(length, requested, minimum, gap, mirrored)
	if width == 0 {
		return 0, 0
	}
	return length - x - width, width
}

// Centered taskbars strictly stay on the left side and hide when it fills up;
// left-aligned taskbars anchor beside the right notification area so opening
// and closing apps does not bounce the lyrics across the taskbar.
func taskbarLyricsPlacementSpace(length, requested, minimum, gap int, occupied []taskbarLyricsInterval, leftTaskbar bool) (int, int) {
	if leftTaskbar {
		return taskbarLyricsFreeSpaceRight(length, requested, minimum, gap, occupied)
	}
	x, width := taskbarLyricsFreeSpace(length, requested, minimum, gap, occupied)
	if width == 0 || x >= length/2 {
		return 0, 0
	}
	return x, width
}
