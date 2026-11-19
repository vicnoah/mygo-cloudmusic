package pluginapi

// SettingsStore loads and saves a plugin's serializable preferences in its
// own namespace. Credentials have a separate store.
type SettingsStore interface {
	// Load overlays stored data on the caller's defaults during Start.
	Load(dst any) error
	// Save captures a snapshot before returning. During normal desktop
	// operation the host writes it on the main thread.
	Save(src any) error
}

// BindSettings loads the caller's schema over its defaults and returns a
// callback that saves the same value after changes.
func BindSettings[T any](store SettingsStore, defaults T) (*T, func()) {
	if store == nil {
		return &defaults, func() {}
	}
	_ = store.Load(&defaults)
	return &defaults, func() { _ = store.Save(&defaults) }
}
