package speech

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"waygu-music/internal/pluginapi/lyrics"
)

// How long a caption's line is, in characters: it ends at a sentence's
// end, or at a comma once it is long enough, and at most so long, at a
// space if it can.
const (
	lineMin  = 12
	lineMax  = 32
	lineTiny = 4 // a line shorter goes onto the one before
)

// Lines turns segments into captions' lines, shown as lyrics are: each
// sentence of a segment a line, long ones cut at their commas, and timed
// by their share of what the segment says.
func Lines(segs []Segment) []lyrics.Line {
	var out []lyrics.Line
	for _, s := range segs {
		parts := split(s.Text)
		total := 0
		for _, p := range parts {
			total += spoken(p)
		}
		at, said := s.Start, 0
		for _, p := range parts {
			said += spoken(p)
			end := s.Start + time.Duration(float64(s.End-s.Start)*float64(said)/float64(max(total, 1)))
			text := strings.TrimRightFunc(p, func(r rune) bool { return strings.ContainsRune("。，、；：,.;: ", r) })
			if text != "" {
				out = append(out, lyrics.Line{At: at, End: end, Text: text})
			}
			at = end
		}
	}
	return out
}

// What ends a sentence, and what closes a quote or brackets.
const (
	enders  = "。！？!?…；;"
	closers = "”’」』）)"
)

// split cuts text into lines.
func split(text string) []string {
	var parts []string
	var cur strings.Builder
	n := 0 // the runes in cur
	cut := func() {
		if p := strings.TrimSpace(cur.String()); p != "" {
			if k := len(parts); k > 0 && spoken(p) < lineTiny && utf8.RuneCountInString(parts[k-1])+n <= lineMax {
				parts[k-1] += p
			} else {
				parts = append(parts, p)
			}
		}
		cur.Reset()
		n = 0
	}
	runes := []rune(strings.TrimSpace(text))
	for i, r := range runes {
		cur.WriteRune(r)
		n++
		next := ' '
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		switch {
		case strings.ContainsRune(closers, next):
			// The quote or bracket goes with what it closes.
		case strings.ContainsRune(enders, r) && (r != '…' || next != '…'),
			strings.ContainsRune(closers, r) && i > 0 && strings.ContainsRune(enders, runes[i-1]):
			cut()
		case strings.ContainsRune("，、：,:", r) && n >= lineMin:
			cut()
		case n >= lineMax && (unicode.IsSpace(r) || unicode.Is(unicode.Han, r)):
			cut()
		}
	}
	cut()
	return parts
}

// spoken counts about how many syllables of text are said: one for each
// Chinese character, one for each three letters of a word in English,
// none for punctuation and spaces.
func spoken(text string) int {
	n, word := 0, 0
	for _, r := range text + " " {
		switch {
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
			n++
			fallthrough
		case !unicode.IsLetter(r) && !unicode.IsNumber(r):
			n += (word + 2) / 3
			word = 0
		default:
			word++
		}
	}
	return n
}
