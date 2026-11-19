package netease

import "github.com/egoist/mygo/ui"

func (a *app) platformTrackRowSkeleton(c *ui.Context, s shimmer, i int, wide, covers, showAlbum bool) {
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
