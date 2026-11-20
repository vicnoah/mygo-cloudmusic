package app

import (
	"encoding/json"

	"github.com/egoist/mygo"
	appplatform "github.com/waygu-music-plugin/sdk/pluginapi"
)

// platformSettings holds plugin records without interpreting their schema.
type platformSettings struct {
	Plugins                  map[appplatform.ID]json.RawMessage `json:"plugins,omitempty"`
	Platform                 string                             `json:"platform,omitempty"`
	PluginCatalogs           map[string]pluginCatalogSettings   `json:"pluginCatalogs,omitempty"`
	PluginCatalogURL         string                             `json:"pluginCatalogURL,omitempty"`
	LastPluginEntries        map[string]string                  `json:"lastPluginEntries,omitempty"`
	DisablePluginGitHubProxy bool                               `json:"disablePluginGitHubProxy,omitempty"`
	LegacySubsonicSession    *legacySubsonicSession             `json:"legacySubsonicSession,omitempty"`

	legacyNamespaces map[string]json.RawMessage
}

type pluginCatalogSettings struct {
	URL       string `json:"url"`
	LegacyKey string `json:"key,omitempty"`
}

// These fields predate plugin installation and must survive while their owner is absent.
var historicalSettingsFields = []string{
	"fnmusic", "navidrome", "jellyfin", "gonic", "subsonic", "songloft", "qqmusic",
	"captions", "hotComments",
}

func (s *platformSettings) loadLegacyNamespaces(data []byte, registrations []appplatform.Registration) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return
	}
	s.legacyNamespaces = make(map[string]json.RawMessage)
	for _, key := range historicalSettingsFields {
		if raw, ok := fields[key]; ok {
			s.legacyNamespaces[key] = raw
		}
	}
	for _, entry := range registrations {
		if key := entry.LegacySettingsKey; key != "" {
			if raw := fields[key]; raw != nil {
				s.legacyNamespaces[key] = raw
			}
		}
		for _, key := range entry.LegacySettingsFields {
			if raw := fields[key]; raw != nil {
				s.legacyNamespaces[key] = raw
			}
		}
	}
}

type pluginSettingsStore struct {
	a     *app
	entry appplatform.Registration
}

func (store pluginSettingsStore) Load(dst any) error {
	data, err := store.raw()
	if err != nil || data == nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func (store pluginSettingsStore) raw() (json.RawMessage, error) {
	s := &store.a.settings
	var data json.RawMessage
	switch {
	case store.entry.LegacySettingsKey != "":
		data = s.legacyNamespaces[store.entry.LegacySettingsKey]
	case len(store.entry.LegacySettingsFields) != 0:
		fields := make(map[string]json.RawMessage)
		for _, key := range store.entry.LegacySettingsFields {
			if raw := s.legacyNamespaces[key]; raw != nil {
				fields[key] = raw
			}
		}
		var err error
		data, err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
	default:
		data = s.Plugins[store.entry.ID]
	}
	return data, nil
}

func (store pluginSettingsStore) Save(src any) error {
	_, guard := (platformHost{a: store.a, id: store.entry.ID}).authorization()
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	var flat map[string]json.RawMessage
	if len(store.entry.LegacySettingsFields) != 0 {
		if err := json.Unmarshal(data, &flat); err != nil {
			return err
		}
	}
	if store.a.win != nil {
		store.a.win.Update(func() {
			if allowedWrite(guard) {
				_ = store.commit(data, flat)
			}
		})
		return nil
	}
	if allowedWrite(guard) || store.a.platformContext == nil {
		return store.commit(data, flat)
	}
	return appplatform.ErrUnavailable
}

func (store pluginSettingsStore) commit(data json.RawMessage, flat map[string]json.RawMessage) error {
	s := &store.a.settings
	switch {
	case store.entry.LegacySettingsKey != "":
		s.keepLegacyField(store.entry.LegacySettingsKey, data)
	case len(store.entry.LegacySettingsFields) != 0:
		fields := make(map[string]json.RawMessage)
		for _, key := range store.entry.LegacySettingsFields {
			if raw, ok := flat[key]; ok {
				s.keepLegacyField(key, raw)
				fields[key] = raw
			}
		}
		// Legacy flat preferences that were shared with playback retain
		// their effect on adapters which use the application's defaults.
		projected, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(projected, s); err != nil {
			return err
		}
	default:
		if s.Plugins == nil {
			s.Plugins = make(map[appplatform.ID]json.RawMessage)
		}
		s.Plugins[store.entry.ID] = data
	}
	return s.save()
}

func (s *platformSettings) keepLegacyField(key string, data json.RawMessage) {
	if s.legacyNamespaces == nil {
		s.legacyNamespaces = make(map[string]json.RawMessage)
	}
	s.legacyNamespaces[key] = data
}

// setPlatform switches the complete platform context and its auxiliary windows.
func (a *app) setPlatform(id appplatform.ID) {
	if a.localPlugins[id] != nil {
		a.startLocalPlugin(id)
		return
	}
	if !a.hasPlatform(id) {
		return
	}
	a.cancelLocalRequest()
	a.commitPlatform(id, true)
}

func (a *app) commitPlatform(id appplatform.ID, persist bool) {
	if a.platformID == id {
		return
	}
	starting := a.platformID == ""
	wasRemote := a.localPlugins[a.platformID] != nil
	if a.pl != nil {
		a.saveSession()
	}
	if a.win != nil {
		a.unmountPlatformWindows()
	}
	a.deactivatePlatformView(true)
	if a.win != nil {
		a.win.SetWindowButtonsTheme(mygo.ThemeSystem)
	}
	if wasRemote {
		a.revokeLocalPlugin()
	}
	if a.pl != nil {
		a.pl.Reset()
	}
	a.clearSessionState()
	a.toast = ""
	a.platformID = id
	a.platformGeneration++
	if persist {
		a.settings.Platform = string(id)
	}
	a.pluginStartup.failure = ""
	a.dismissStartupNotice()
	if id != "" {
		a.activatePlatformView()
	}

	if persist {
		a.settings.save()
	}
	if a.win != nil {
		a.syncPlatformWindows()
		a.mountPlatformWindows()
		a.setMenu()
		a.platformReady(starting)
		if id != "" && (starting || wasRemote) && a.remoteActive() == nil {
			a.showMain()
		}
	}
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
	a.redraw()
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}
