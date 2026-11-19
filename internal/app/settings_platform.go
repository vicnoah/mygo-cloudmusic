package app

import (
	"encoding/json"

	appplatform "waygu-music/internal/pluginapi"
)

// platformSettings holds plugin records without interpreting their schema.
type platformSettings struct {
	Plugins       map[appplatform.ID]json.RawMessage `json:"plugins,omitempty"`
	Platform      string                             `json:"platform,omitempty"`
	DeveloperMode bool                               `json:"developerMode,omitempty"`

	legacyNamespaces map[string]json.RawMessage
}

func (s *platformSettings) loadLegacyNamespaces(data []byte, registrations []appplatform.Registration) {
	if len(registrations) == 0 {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return
	}
	s.legacyNamespaces = make(map[string]json.RawMessage)
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
			return err
		}
	default:
		data = s.Plugins[store.entry.ID]
	}
	if data == nil {
		return nil
	}
	return json.Unmarshal(data, dst)
}

func (store pluginSettingsStore) Save(src any) error {
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
		store.a.win.Update(func() { _ = store.commit(data, flat) })
		return nil
	}
	return store.commit(data, flat)
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
	if !a.hasPlatform(id) {
		id = a.platformID
		for _, entry := range a.platformRegistrations {
			if entry.FallbackOnInvalidSwitch && a.hasPlatform(entry.ID) {
				id = entry.ID
				break
			}
		}
	}
	if a.platformID == id {
		return
	}
	if a.win != nil {
		a.unmountPlatformWindows()
	}
	a.deactivatePlatformView(true)
	if a.pl != nil {
		a.pl.Reset()
	}
	a.discardSession()
	a.toast = ""
	a.platformID = id
	a.platformGeneration++
	a.settings.Platform = string(id)
	a.activatePlatformView()

	a.settings.save()
	if a.win != nil {
		a.syncPlatformWindows()
		a.mountPlatformWindows()
		a.setMenu()
		a.platformReady(false)
	}
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
	a.redraw()
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}
