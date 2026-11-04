package songloft

import (
	backend "netease-music-mygo/internal/platform/songloft"

	"github.com/egoist/mygo/ui"
)

type sortField struct {
	field string
	label string
}

type sortControl struct {
	fields  []sortField
	current string
	onSort  func(string)
}

func (s sortControl) normalized() (sortField, string) {
	field, order, ok := backend.SplitSort(s.current)
	if !ok {
		field, order = "added_at", "desc"
	}
	for _, opt := range s.fields {
		if opt.field == field {
			return opt, order
		}
	}
	if len(s.fields) > 0 {
		return s.fields[0], order
	}
	return sortField{field: field, label: field}, order
}

func (s sortControl) button(c *ui.Context, key string) ui.Element {
	field, order := s.normalized()
	arrow := "↑"
	if order == "desc" {
		arrow = "↓"
	}
	b := ui.ButtonBase(c.Key(key)).Height(30).PaddingX(12).Radius(999).Gap(6).AlignItems(ui.Center).FocusRing(false).Label("排序")
	if b.Hovered() {
		b.Background(c.Theme().Text.Alpha(0.06))
	}
	b.Children(func() {
		ui.Text(c, "排序 · "+field.label+" "+arrow).FontSize(12.5).TextColor(c.Theme().TextMuted)
	})
	b.Menu(func(m *ui.Menu) {
		for _, option := range s.fields {
			if m.Item(option.label).Checked(option.field == field.field).Chosen() && option.field != field.field {
				s.onSort(option.field + ":" + order)
			}
		}
		m.Separator()
		for _, choice := range []struct {
			order string
			label string
		}{
			{"asc", "升序 ↑"},
			{"desc", "降序 ↓"},
		} {
			if m.Item(choice.label).Checked(order == choice.order).Chosen() && order != choice.order {
				s.onSort(field.field + ":" + choice.order)
			}
		}
	})
	return b
}

var (
	songSortFields = []sortField{
		{"added_at", "添加时间"},
		{"title", "歌曲名"},
		{"artist", "歌手"},
		{"album", "专辑"},
		{"duration", "时长"},
		{"year", "年份"},
	}
	albumTrackSortFields = []sortField{
		{"title", "歌曲名"},
		{"duration", "时长"},
		{"added_at", "添加时间"},
	}
	artistTrackSortFields = []sortField{
		{"added_at", "添加时间"},
		{"title", "歌曲名"},
		{"album", "专辑"},
		{"duration", "时长"},
	}
)

func (h *Home) listSort(l *List) (sortControl, bool) {
	if l == nil {
		return sortControl{}, false
	}
	cat := h.state.Catalog()
	if cat == nil {
		return sortControl{}, false
	}
	switch l.Spec.Kind {
	case ListLibrary:
		return sortControl{
			fields:  songSortFields,
			current: l.Spec.Sort,
			onSort: func(v string) {
				cat.LibrarySort = v
				l.SetSort(v)
				h.host.Invalidate()
			},
		}, true
	case ListFavorites:
		return sortControl{
			fields:  songSortFields,
			current: l.Spec.Sort,
			onSort: func(v string) {
				cat.FavoriteSort = v
				l.SetSort(v)
				h.host.Invalidate()
			},
		}, true
	case ListPlaylist:
		return sortControl{
			fields:  songSortFields,
			current: l.Spec.Sort,
			onSort: func(v string) {
				l.SetSort(v)
				h.host.Invalidate()
			},
		}, true
	case ListAlbum:
		return sortControl{
			fields:  albumTrackSortFields,
			current: l.Spec.Sort,
			onSort: func(v string) {
				cat.AlbumTrackSort = v
				l.SetSort(v)
				h.host.Invalidate()
			},
		}, true
	case ListArtist:
		return sortControl{
			fields:  artistTrackSortFields,
			current: l.Spec.Sort,
			onSort: func(v string) {
				cat.ArtistTrackSort = v
				l.SetSort(v)
				h.host.Invalidate()
			},
		}, true
	default:
		return sortControl{}, false
	}
}
