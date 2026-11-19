package app

import "waygu-music/internal/pluginapi"

// rememberSearch puts q first in the history of searches, once.
func (a *app) rememberSearch(q string) {
	a.settings.SearchHistory = pluginapi.RecordSearch(a.settings.SearchHistory, q)
	a.settings.save()
}

func (a *app) clearSearchHistory() {
	a.settings.SearchHistory = nil
	a.settings.save()
}
