package speech

import (
	"math"
	"slices"
	"testing"
	"time"
)

// A segment's sentences become lines of their own, long ones cut at a
// comma, short ones kept with the line before, with no full stop at
// their ends.
func TestSplit(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"有些话题是绕不开的。比如说，我的婚姻结束了。", []string{"有些话题是绕不开的", "比如说，我的婚姻结束了"}},
		{"那一批的话，主要就不是说像前一批的这些商人啊，然后富甲一方的这些有影响力的人，多数都是出身于最底层的这些苦力。",
			[]string{"那一批的话，主要就不是说像前一批的这些商人啊", "然后富甲一方的这些有影响力的人", "多数都是出身于最底层的这些苦力"}},
		{"看不到它的关联的原因，但是它可能跟就是潮汕的发音是有关联的。哦。", []string{"看不到它的关联的原因，但是它可能跟就是潮汕的发音是有关联的。哦"}},
		{"那能不能就别去假装真？我们就探讨一下情吧。", []string{"那能不能就别去假装真？", "我们就探讨一下情吧"}},
		{"他说：“我要节食啊。”然后就走了。", []string{"他说：“我要节食啊。”", "然后就走了"}},
	} {
		var got []string
		for _, l := range Lines([]Segment{{Start: 0, End: 10 * time.Second, Text: c.in}}) {
			got = append(got, l.Text)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("Lines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A segment's lines share its time by how much each says, end to end.
func TestLinesTimed(t *testing.T) {
	seg := Segment{Start: 10 * time.Second, End: 16 * time.Second, Text: "有些话题是绕不开的。比如说，我的婚姻结束了。her edition。"}
	lines := Lines([]Segment{seg})
	if len(lines) != 3 {
		t.Fatalf("%d lines", len(lines))
	}
	if lines[0].At != seg.Start || lines[2].End != seg.End {
		t.Errorf("lines span %v–%v, want %v–%v", lines[0].At, lines[2].End, seg.Start, seg.End)
	}
	for i := 1; i < len(lines); i++ {
		if lines[i].At != lines[i-1].End {
			t.Errorf("line %d starts at %v, after %v", i, lines[i].At, lines[i-1].End)
		}
	}
	// 9, 10 and 4 syllables of 23.
	if d := lines[0].End - lines[0].At; math.Abs(d.Seconds()-6*9.0/23) > 0.01 {
		t.Errorf("first line lasts %v", d)
	}
	if d := lines[2].End - lines[2].At; math.Abs(d.Seconds()-6*4.0/23) > 0.01 {
		t.Errorf("last line lasts %v", d)
	}
}
