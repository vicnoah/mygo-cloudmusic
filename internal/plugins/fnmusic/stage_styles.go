package fnmusic

import (
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi/lyrics"
)

// The stage sets its lyrics in one of the styles of Folia's visualizers
// (github.com/chthollyphile/folia-major), named as Folia names them: 流光,
// its classic (ui_stage.go); 云阶, which stacks a line in staggered rows,
// each springing in as it is sung; and 倾诉, which breaks a line into a
// few large lines, one of them leaning, its characters swelling as they
// are sung.
var stageStyles = []struct{ id, label string }{
	{"", "流光"},
	{"partita", "云阶"},
	{"tilt", "倾诉"},
}

// stageStyleButton names the stage's style, and opens the menu of the
// others.
func (view *Home) stageStyleButton(c *ui.Context) {
	label := stageStyles[0].label
	for _, s := range stageStyles {
		if s.id == view.settings.StageStyle {
			label = s.label
		}
	}
	b := ui.ButtonBase(c.Key("stage-style")).Padding(6, 10, 6, 14).Radius(999).Gap(4).AlignItems(ui.Center).
		FocusRing(false).Label("律动效果").Tooltip("律动效果")
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if b.Hovered() {
		bg, fg = white.Alpha(0.06), white.Alpha(0.8)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg)
		ui.Icon(c, iconDown).Size(12, 12).TextColor(fg)
	})
	b.Menu(func(m *ui.Menu) {
		for _, s := range stageStyles {
			if m.Item(s.label).Checked(view.settings.StageStyle == s.id).Chosen() && view.settings.StageStyle != s.id {
				view.settings.StageStyle = s.id
				view.savePresentation()
			}
		}
		m.Separator()
		if m.Item("火花").Checked(view.settings.StageSparks).Chosen() {
			view.settings.StageSparks = !view.settings.StageSparks
			view.savePresentation()
		}
	})
}

// sparkTime is how long a word's sparks fly.
const sparkTime = 0.6

// stageSparks throws sparks off e as a word lands, since seconds after it
// is sung, seed telling one word's from another's: short streaks of the
// accent, white-hot at first, fly out from its edge, slowing, shortening
// and fading; more and farther off a line's last word. They are drawn only
// while they fly, in frames the stage draws for the word anyway.
func stageSparks(e ui.Element, since float32, seed float64, last bool, accent ui.Color) {
	if since < 0 || since >= sparkTime {
		return
	}
	t := since / sparkTime
	e.DrawOver(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		n, reach := 9, max(r.W, r.H)*0.75
		if last {
			n, reach = 16, reach*1.6
		}
		out := 1 - (1-t)*(1-t)*(1-t)
		fade := (1 - t) * (1 - t)
		color := accent.Mix(white, 0.6*(1-t))
		for i := range n {
			angle := 2 * math.Pi * (float64(i) + float64(lineRandom(seed, float64(i)))*0.7) / float64(n)
			dx, dy := float32(math.Cos(angle)), float32(math.Sin(angle))
			// From the word's edge, each at its own speed.
			edge := r.W/2*0.7*abs32(dx) + r.H/2*0.7*abs32(dy)
			head := edge + reach*(0.55+0.45*lineRandom(seed, float64(i)+50))*out
			tail := max(edge, head-reach*0.3*(1-t))
			p.Line(cx+dx*tail, cy+dy*tail, cx+dx*head, cy+dy*head, 0.8+1.6*(1-t), color.Alpha(0.9*fade))
		}
	})
}

func abs32(v float32) float32 { return max(v, -v) }

// stageUnits splits words into the units 云阶 and 倾诉 break a line
// between, as spans [from, to) of its words: a word of a language spaced
// into words, or a phrase of Chinese or Japanese up to its space or
// punctuation, cut in fours when long. Punctuation stays with the word
// before it, and a contraction is one word.
func stageUnits(words []lyrics.Word) [][2]int {
	var units [][2]int
	phrase := func(from, to int) {
		// A long run of characters, in pieces of about four.
		n := (to - from + 3) / 4
		if to-from <= 6 {
			n = 1
		}
		for k := range n {
			units = append(units, [2]int{from + (to-from)*k/n, from + (to-from)*(k+1)/n})
		}
	}
	from := 0
	for i, w := range words {
		if i == from && len(units) > 0 && strings.TrimFunc(w.Text, isMark) == "" {
			// Punctuation alone, after a break: onto the unit before.
			units[len(units)-1][1] = i + 1
			from = i + 1
			continue
		}
		// An apostrophe or hyphen joins the word to the next, as in It’s.
		last, _ := utf8.DecodeLastRuneInString(w.Text)
		if i+1 == len(words) || isMark(last) && !strings.ContainsRune("'’-", last) {
			phrase(from, i+1)
			from = i + 1
		}
	}
	return units
}

// isMark says r breaks a line: a space or punctuation.
func isMark(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }

// wordsLength is how many characters words have, not counting spaces.
func wordsLength(words []lyrics.Word) int {
	n := 0
	for _, w := range words {
		n += utf8.RuneCountInString(strings.TrimSpace(w.Text))
	}
	return n
}

// partitaRow is a row of a line on 云阶: its words, [from, to), and where
// it sits, its guide on its left or right.
type partitaRow struct {
	from, to    int
	x, y, scale float32
	left        bool
}

// partitaRows breaks l into rows as 云阶 does, unevenly, as a hand would:
// as many as the units of its words, up to as many as fit in view.
func partitaRows(l *lyrics.Line, h float32) []partitaRow {
	units := stageUnits(l.Words)
	if len(units) == 0 {
		return nil
	}
	seed, k := float64(l.At/time.Millisecond)/1000, 0.0
	random := func() float32 { k++; return lineRandom(seed, k) }
	count := min(len(units), max(1, int(h*0.65/100)))
	rows := make([]partitaRow, 0, count)
	left, unit := len(units), 0
	for i := range count {
		n := left
		if i < count-1 {
			avg := float32(left) / float32(count-i)
			n = min(int(math.Ceil(float64(avg*1.5))), max(1, int(math.Round(float64(avg+(random()-0.5)*avg)))))
		}
		n = max(1, min(n, left-(count-i-1)))
		stagger := 20 + random()*80
		r := partitaRow{
			from:  units[unit][0],
			to:    units[unit+n-1][1],
			x:     stagger,
			y:     (float32(i) - float32(count-1)/2) * 2.5,
			scale: 0.8 + random()*0.9,
			left:  i%2 == 0,
		}
		if r.left {
			r.x = -stagger
		}
		rows = append(rows, r)
		unit += n
		left -= n
	}
	return rows
}

// partitaLine shows l on 云阶 as it is at pos: its rows stacked in the
// middle, each sliding in from the side of its guide as its first word is
// due, the words in it springing in as classic's do. The whole line is at
// opacity fade, swelling a little as it goes.
func (view *Home) partitaLine(c *ui.Context, l *lyrics.Line, pos time.Duration, f, h, fade float32, accent ui.Color) {
	const lookahead = 80 * time.Millisecond
	rows := partitaRows(l, h)
	if wordsLength(l.Words) > 40 {
		f *= 0.8
	}
	f *= 0.8
	grow := 1 + 0.05*(1-fade)
	ui.Column(c.Key(l.At)).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().
		Opacity(fade).Scale(grow, grow).PassThrough().Children(func() {
		for ri, row := range rows {
			words := l.Words[row.from:row.to]
			since := float32((pos - (words[0].Start - lookahead)).Seconds())
			end := float32((pos - max(words[len(words)-1].End, words[len(words)-1].Start+150*time.Millisecond)).Seconds())
			side := float32(40)
			if row.left {
				side = -40
			}
			x, scale, opacity := row.x+side, float32(0.85), float32(0)
			if since > 0 {
				e := springStep(since)
				x, scale, opacity = lerp(row.x+side, row.x, e), lerp(0.85, 1, e), clamp01(since/0.1)
			}
			// The guide, a corner under the row's end, drawn in as it
			// comes, lit while it is sung.
			guide, color, alpha := easeOutCubic(since/0.4), white, float32(0.14)
			switch {
			case end > 0:
				alpha = 0.22
			case since > 0:
				color, alpha = accent, 1
			}

			shapes := make([]wordShape, len(words))
			widths := make([]float32, len(words))
			seed := float64(words[0].Start/time.Millisecond) / 1000
			for i, w := range words {
				widths[i], _ = c.MeasureText(0, ui.Span{Text: w.Text, Size: f, Weight: 800})
				ws := seed + float64(i)*7.13
				shapes[i] = wordShape{
					x:      (lineRandom(ws, 1) - 0.5) * 12,
					y:      (lineRandom(ws, 2) - 0.5) * 12,
					rotate: (lineRandom(ws, 3) - 0.5) * 6,
					scale:  row.scale,
					drift:  (lineRandom(ws, 8) - 0.5) * 20,
				}
			}
			spaceWords(shapes, widths, f*0.1)
			shapes[len(shapes)-1].margin = 0
			shapes[len(shapes)-1].last = ri == len(rows)-1

			pad := f * (row.scale*1.4 - 1) / 2
			ui.Row(c.Key(ri)).AlignItems(ui.Center).Padding(3, 8).Margin(max(0, pad), 0, max(0, pad)+10, 0).
				Translate(x, row.y).Scale(scale, scale).Opacity(opacity).
				Draw(func(p *ui.Painter, r ui.Rect) {
					if guide <= 0 {
						return
					}
					c := color.Alpha(alpha * guide)
					bottom := r.Y + r.H
					if row.left {
						p.Line(r.X-8, bottom+16, r.X-8, bottom+16-32*guide, 1, c)
						p.Line(r.X-16, bottom+8, r.X-16+(r.W+36)*guide, bottom+8, 1, c)
					} else {
						p.Line(r.X+r.W+8, bottom+16, r.X+r.W+8, bottom+16-32*guide, 1, c)
						p.Line(r.X+r.W+16, bottom+8, r.X+r.W+16-(r.W+36)*guide, bottom+8, 1, c)
					}
				}).
				Children(func() {
					for i, w := range words {
						view.stageWord(c.Key(i), w, shapes[i], pos, f, accent)
					}
				})
		}
	})
}

// tiltLead is how long before its first word 倾诉 shows a part of a line.
const tiltLead = 250 * time.Millisecond

// tiltPart is a part of a line on 倾诉, its words [from, to): set
// upright, or leaning, larger when it is a short end of a line.
type tiltPart struct {
	from, to   int
	lean, tail bool
}

// tiltParts breaks l into the parts 倾诉 sets it in: from one to four,
// more the longer it is, splitting it where its words break nearest an
// even share of its characters, and one part in about three leaning.
// fits says whether the widest of some parts is as wide as can show:
// while it is not, the line breaks into more, up to four.
func tiltParts(l *lyrics.Line, fits func([]tiltPart) bool) []tiltPart {
	units := stageUnits(l.Words)
	if len(units) == 0 {
		return nil
	}
	seed := float64(l.At/time.Millisecond) / 1000
	chars := wordsLength(l.Words)
	score := math.Log(float64(chars)+4) / math.Log(24) * float64(lineRandom(seed, 1)*0.6+0.7) * 0.75
	n := 4
	for i, at := range []float64{0.45, 1.05, 1.7} {
		if score < at {
			n = i + 1
			break
		}
	}
	var parts []tiltPart
	for {
		parts = splitParts(l.Words, units, min(n, len(units)))
		var lean []int
		for i := range parts {
			if lineRandom(seed, 100+float64(i)) < 0.35 {
				lean = append(lean, i)
			}
		}
		if len(lean) > 0 {
			parts[lean[int(lineRandom(seed, 200)*float32(len(lean)))]].lean = true
		}
		if n >= 4 || n >= len(units) || fits(parts) {
			break
		}
		n++
	}
	if k := len(parts); k > 1 && !parts[k-1].lean {
		last, before := wordsLength(l.Words[parts[k-1].from:parts[k-1].to]), wordsLength(l.Words[parts[k-2].from:parts[k-2].to])
		parts[k-1].tail = last <= 2 && last*2 <= before
	}
	return parts
}

// splitParts splits words into n parts at the ends of units, each break
// where the share of characters before it is nearest k/n, rather after
// punctuation than a space.
func splitParts(words []lyrics.Word, units [][2]int, n int) []tiltPart {
	total := float32(wordsLength(words))
	before := make([]float32, len(units)+1) // characters before each unit
	for i, u := range units {
		before[i+1] = before[i] + float32(wordsLength(words[u[0]:u[1]]))
	}
	parts := make([]tiltPart, 0, n)
	from := 0 // the unit the next part starts at
	for k := 1; k < n; k++ {
		at, best := -1, float32(0)
		for cut := from + 1; cut <= len(units)-(n-k); cut++ {
			d := float32(math.Abs(float64(before[cut] - total*float32(k)/float32(n))))
			if last, _ := utf8.DecodeLastRuneInString(strings.TrimSpace(words[units[cut-1][1]-1].Text)); unicode.IsPunct(last) {
				d -= total * 0.15
			}
			if at < 0 || d < best {
				at, best = cut, d
			}
		}
		parts = append(parts, tiltPart{from: units[from][0], to: units[at-1][1]})
		from = at
	}
	return append(parts, tiltPart{from: units[from][0], to: units[len(units)-1][1]})
}

// tiltLine shows l on 倾诉 as it is at pos, its parts one under another in
// the middle of the stage, w wide: each rises in as it is due, its
// characters fading in one after another, swelling as each is sung and
// staying a little larger. The leaning part is lighter, spaced wider, in
// the accent, its characters up and down by turns. The whole line is at
// opacity fade, rising a little as it goes; with sparks, each word throws
// them as it is sung.
func tiltLine(c *ui.Context, l *lyrics.Line, pos time.Duration, w, fade float32, accent ui.Color, sparks bool) {
	size := min(w*0.06875, 90)
	avail := w * 0.85
	style := func(p tiltPart) (weight int, spacing float32) {
		if p.lean {
			return 300, 0.15
		}
		return 400, 0.08
	}
	width := func(p tiltPart) float32 {
		weight, spacing := style(p)
		text := ""
		for _, w := range l.Words[p.from:p.to] {
			text += w.Text
		}
		text = strings.TrimSpace(text)
		tw, _ := c.MeasureText(0, ui.Span{Text: text, Size: size, Weight: weight})
		return tw + float32(utf8.RuneCountInString(text))*size*spacing
	}
	widest := func(parts []tiltPart) float32 {
		m := float32(0)
		for _, p := range parts {
			m = max(m, width(p))
		}
		return m
	}
	parts := tiltParts(l, func(parts []tiltPart) bool { return widest(parts) <= avail })
	if m := widest(parts); m > avail {
		size *= max(0.5, avail/m)
	}

	ui.Column(c.Key(l.At)).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(14).
		Translate(0, -12*(1-fade)).Opacity(fade).PassThrough().Children(func() {
		for pi, part := range parts {
			tiltPartView(c.Key(pi), l.Words[part.from:part.to], part, pos, size, accent, sparks, pi == len(parts)-1)
		}
	})
}

// tiltPartView shows a part of a line on 倾诉, its words, as they are at
// pos, last when the part ends the line.
func tiltPartView(c *ui.Context, words []lyrics.Word, part tiltPart, pos time.Duration, size float32, accent ui.Color, sparks, last bool) {
	ease := func(t float32) float32 { t = clamp01(t); return 1 - (1-t)*(1-t) }
	since := float32((pos - (words[0].Start - tiltLead)).Seconds())
	weight, spacing, gap, color, pulse := 400, float32(0.08), float32(0.25), white, float32(0.15)
	in := ease(since / 0.55)
	rise, scale, stagger := 20*(1-in), float32(1), float32(0.04)
	if part.lean {
		weight, spacing, gap, color, pulse = 300, 0.15, 0.35, accent, 0.18
		in = ease(since / 0.6)
		rise, scale, stagger = 24*(1-in), 0.92+0.08*in, 0.05
	} else if part.tail {
		size *= 1.18
	}
	row := ui.Row(c).AlignItems(ui.Center).Translate(0, rise).Scale(scale, scale).Opacity(in)
	row.Children(func() {
		k := 0 // characters so far, not counting spaces
		for wi, word := range words {
			text := strings.TrimRightFunc(word.Text, unicode.IsSpace)
			n := utf8.RuneCountInString(text)
			each := (word.End - word.Start) / time.Duration(max(n, 1))
			i := 0
			for _, r := range text {
				if unicode.IsSpace(r) {
					ui.Box(c.Key([2]int{wi, i})).Width(size * gap)
					i++
					continue
				}
				// Swelling as it is sung, for at least a fifth of a second
				// and at most nine tenths, then settling a little larger.
				start := word.Start + time.Duration(i)*each
				d := float32(min(max(each.Seconds(), 0.2), 0.9))
				t := float32((pos - start).Seconds())
				swell := float32(0)
				switch {
				case t < 0:
				case t <= d:
					swell = float32(math.Sin(float64(t / d * math.Pi)))
				default:
					swell = 0.25 * clamp01((t-d)/(d*1.2))
				}
				s := 1 + swell*pulse
				shown := ease((since - float32(k)*stagger) / 0.5)
				ch := ui.Text(c.Key([2]int{wi, i}), string(r)).FontSize(size).FontWeight(weight).NoWrap().
					TextColor(color).MarginEnd(size*spacing).Opacity(shown).Scale(s, s)
				if part.lean {
					// Up and down by turns, from twice as far.
					y := size / 6 * (2 - shown)
					if k%2 == 0 {
						y = -y
					}
					ch.Italic().LineHeight(1.25).Translate(0, y)
				} else {
					ch.LineHeight(1.35)
				}
				if sparks && i == 0 {
					stageSparks(ch, t, float64(word.Start/time.Millisecond), last && wi == len(words)-1, accent)
				}
				i++
				k++
			}
			if text != word.Text && wi < len(words)-1 {
				ui.Box(c.Key([2]int{wi, -1})).Width(size * gap)
			}
		}
	})
}
