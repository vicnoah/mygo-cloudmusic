package subsonic

import (
	"github.com/egoist/mygo/ui"
)

func (h *Home) platformTrackSkeleton(c *ui.Context, view TrackView) {
	s := pageShimmer(c)
	h.platformTrackSkeletonOptions(c, s, true, view.ShowAlbum, func(c *ui.Context) {
		if view.HeaderSkeleton != nil {
			view.HeaderSkeleton(c)
		} else if view.Header != nil {
			h.playlistHeaderSkeleton(c, s)
		} else {
			platformHeadingSkeleton(c, s, view.Sub != "")
		}
	})
}

func (h *Home) platformTrackSkeletonOptions(c *ui.Context, s shimmer, covers, showAlbum bool, header func(c *ui.Context)) {
	w, height := c.Size()
	wide := w-sidebarWidth > 820
	top, bottom := h.insets(c)
	h.underTop, h.underBottom = false, true
	ui.Column(c.Key("skeleton")).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).Children(func() {
		h.startMark(c)
		header(c)
		for i := range max(8, int(height/54)) {
			h.platformTrackRowSkeleton(c.Key(i), s, i, wide, covers, showAlbum)
		}
	})
}

func (h *Home) platformTrackRowSkeleton(c *ui.Context, s shimmer, i int, wide, covers, showAlbum bool) {
	ui.Row(c).Height(54).Shrink(0).PaddingX(12).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(26).Center().Children(func() { s.text(c, 13, 1.3).Width(12) })
		if covers {
			s.bone(c, 6).Size(38, 38)
		}
		ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
			s.text(c, 14, 1.3).WidthPercent(boneWidth(i) * 0.8)
			s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+5) * 0.55)
		})
		if wide && showAlbum {
			ui.Box(c).Grow(2).Basis(0).MinWidth(0).Children(func() {
				s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+3) * 0.8)
			})
		}
		ui.Box(c).Size(32, 32)
		ui.Row(c).Width(44).Justify(ui.End).Children(func() { s.text(c, 12.5, 1.3).Width(30) })
	})
}

func platformHeadingSkeleton(c *ui.Context, s shimmer, sub bool) {
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		if sub {
			s.text(c, 12, 1.3).Width(52)
		}
		s.text(c, 30, 1.2).WidthPercent(36)
	})
}

func platformSectionSkeleton(c *ui.Context, s shimmer) {
	s.text(c, 19, 1.3).Width(76).Margin(34, 0, 14, 0)
}

func (h *Home) platformCardSkeleton(c *ui.Context, s shimmer, i int, meta bool) {
	ui.Column(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
		s.bone(c, 10).AspectRatio(1)
		ui.Column(c).Gap(0).Children(func() {
			s.text(c, 13, 1.35).WidthPercent(92)
			s.text(c, 13, 1.35).WidthPercent(boneWidth(i))
		})
		if meta {
			s.text(c, 12, 1.3).WidthPercent(40).Margin(-6, 0, 0, 0)
		}
	})
}

func (h *Home) platformCardsSkeleton(c *ui.Context, view CardView) {
	s := pageShimmer(c)
	cols, _ := h.cardWidth(c)
	top, bottom := h.insets(c)
	h.underTop, h.underBottom = false, true
	ui.Scroll(c.Key("platform-cards-loading")).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		if view.HeaderSkeleton != nil {
			view.HeaderSkeleton(c)
		} else {
			ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
				s.text(c, 30, 1.2).WidthPercent(36)
				if view.Sub != "" {
					s.text(c, 12, 1.3).Width(72)
				}
				s.text(c, 13, 1.3).Width(52)
			})
			if len(view.Sort) > 0 {
				ui.Row(c).Margin(-6, 0, 18, 0).AlignItems(ui.Center).Gap(10).Children(func() {
					s.text(c, 12, 1.3).Width(24)
					width := float32(0)
					for _, choice := range view.Sort {
						width = max(width, float32(len([]rune(choice.Label)))*13+26)
					}
					s.bone(c, 6).Height(24).Width(width*float32(len(view.Sort)) + 4)
				})
			}
		}
		ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
			for i := range cols * 2 {
				h.platformCardSkeleton(c.Key(i), s, i, view.Meta)
			}
		})
	})
}

func (h *Home) platformSearchSkeleton(c *ui.Context) {
	s := pageShimmer(c)
	h.platformTrackSkeletonOptions(c, s, true, true, func(c *ui.Context) {
		platformHeadingSkeleton(c, s, true)
		h.platformSearchCardStripSkeleton(c, s)
		h.platformSearchCardStripSkeleton(c, s)
		platformSectionSkeleton(c, s)
	})
}

func (h *Home) platformSearchCardStripSkeleton(c *ui.Context, s shimmer) {
	cols := h.columns(c, 168)
	ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		s.text(c, 19, 1.3).Width(40)
		ui.Box(c).Grow(1)
		s.text(c, 12.5, 1.3).Width(48)
	})
	ui.Row(c).Gap(20).Children(func() {
		for i := range cols {
			ui.Box(c.Key(i)).Width(168).Children(func() {
				h.platformCardSkeleton(c, s, i, false)
			})
		}
	})
}

func (h *Home) playlistHeaderSkeleton(c *ui.Context, s shimmer) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		s.bone(c, 14).Size(212, 212)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			s.text(c, 12, 1.3).Width(36)
			s.text(c, 30, 1.2).WidthPercent(48)
			s.text(c, 13, 1.3).Width(180)
			ui.Column(c).Margin(2, 0, 0, 0).Children(func() {
				s.text(c, 12.5, 1.45).WidthPercent(78)
				s.text(c, 12.5, 1.45).WidthPercent(52)
			})
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				s.bone(c, 18).Size(92, 36)
				s.bone(c, 18).Size(120, 36)
			})
		})
	})
}

var boneWidths = [...]float32{62, 44, 78, 53, 70, 38, 66, 49, 82, 57, 41, 73}

func boneWidth(i int) float32 { return boneWidths[i%len(boneWidths)] }
