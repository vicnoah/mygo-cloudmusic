package subsonic

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (h *Home) searchField(c *ui.Context, placeholder string, submit func(string)) {
	p := paletteFor(c.Theme())
	history := h.host.GlobalSearchHistory()
	field := ui.Row(c).Height(32).PaddingX(10).Gap(7).Radius(9).AlignItems(ui.Center).Margin(0, 0, 14, 0)
	if c.Focused(h.searchInput) {
		field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent.Alpha(0.6))
	} else {
		field.Background(p.hover).Border(1, p.hairline)
	}
	field.Children(func() {
		ui.Icon(c, iconSearchField).Size(14, 14).TextColor(p.muted)
		// Use the same base widget as the upstream search field. A plain
		// TextInput looks close, but does not keep the combobox focus,
		// keyboard and popup state that gives the search field its behavior.
		cb := ui.ComboboxBase(c.Key("search"), &h.search)
		in := cb.Input.Bind(&h.searchInput).Placeholder(placeholder).Label("搜索").FontSize(13).Grow(1).MinWidth(0)
		q := strings.TrimSpace(h.search)
		if q == "" && len(history) == 0 {
			cb.SetOpen(false)
		}
		cb.Popup(func(panel ui.Element) {
			panel.AttachTo(field, ui.AnchorBottomStart, ui.AnchorTopStart).
				Width(360).Padding(6).Margin(8, 0, 0, 0).Radius(12).
				Background(p.bg).Border(1, p.hairline).
				Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2))
			option := func(key, label string) ui.Element {
				row := cb.Item(key).MinHeight(36).Padding(6, 10).Gap(10).Radius(7).AlignItems(ui.Center).Label(label)
				if row.Highlighted() {
					row.Background(p.hover)
				}
				return row
			}
			if q == "" {
				panel.Label("搜索历史")
				ui.Row(c).Padding(2, 4, 2, 10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "搜索历史").FontSize(12).TextColor(p.muted).Grow(1)
					iconButton(c.Key("platform-clear-search-history"), iconClearHistory, 14, p.muted, "清空搜索历史").KeepFocus().OnClick(h.host.ClearSearchHistory)
				})
				for _, entry := range history {
					option("history:"+entry, entry).Children(func() {
						ui.Icon(c, iconSearchField).Size(13, 13).TextColor(p.muted)
						ui.Text(c, entry).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
					})
				}
			} else {
				panel.Label("搜索建议")
				option("query", "搜索“"+q+"”").Children(func() {
					ui.Icon(c, iconSearchField).Size(13, 13).TextColor(p.muted)
					ui.Text(c, "搜索“"+q+"”").FontSize(13).FontWeight(500).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
		if key, ok := cb.Chosen(); ok {
			cb.SetOpen(false)
			if history, found := strings.CutPrefix(key, "history:"); found {
				h.search = history
				q = history
			}
			if key == "query" || strings.HasPrefix(key, "history:") {
				if q != "" {
					h.host.RememberGlobalSearch(q)
					submit(q)
				}
			}
		} else if in.Submitted() && q != "" {
			cb.SetOpen(false)
			h.host.RememberGlobalSearch(q)
			submit(q)
		}
	})
}
