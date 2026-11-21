package app

import (
	"strings"
	"time"

	"github.com/go-text/typesetting/segmenter"
)

// The native tray chooses the font and width. Keep each page within the
// previous label's approximate width and prefer breaks between words.
func statusLyricsTrayPages(text string) []string {
	if text == "" {
		return nil
	}
	const pageWidth = 32
	var seg segmenter.Segmenter
	if err := seg.InitWithString(text); err != nil {
		return nil
	}
	pages := make([]string, 0, 1)
	start, width := 0, 0
	space, widthAtSpace := -1, 0
	for it := seg.GraphemeIterator(); it.Next(); {
		g := it.Grapheme()
		cell := 1
		if g.Text[0] > 127 {
			cell = 2
		}
		if width+cell > pageWidth {
			end := g.OffsetInBytes
			remaining := 0
			if space > start && width-widthAtSpace <= pageWidth/3 {
				end, remaining = space, width-widthAtSpace
			}
			pages = append(pages, strings.TrimSpace(text[start:end]))
			start, width, space = end, remaining, -1
		}
		width += cell
		if g.Text[0] == ' ' {
			space, widthAtSpace = g.OffsetInBytes+g.LengthInBytes, width
		}
	}
	return append(pages, strings.TrimSpace(text[start:]))
}

func statusLyricsTrayPageAt(pages []string, timing statusLyricsTiming, position time.Duration) (string, time.Duration) {
	if len(pages) == 0 {
		return "", 0
	}
	if len(pages) == 1 || timing.end <= timing.start {
		return pages[0], 0
	}
	elapsed := max(0, position-timing.start)
	pageTime := max(time.Nanosecond, (timing.end-timing.start)/time.Duration(len(pages)))
	index := min(int(elapsed/pageTime), len(pages)-1)
	if index == len(pages)-1 {
		return pages[index], 0
	}
	return pages[index], pageTime - elapsed%pageTime
}
