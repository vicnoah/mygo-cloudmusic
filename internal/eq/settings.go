package eq

// Settings is the equalizer's part of settings.json. Older versions kept
// their equalizer under other keys, which are ignored: the equalizer
// starts again from Default.
type Settings struct {
	Enabled bool      `json:"enabled"`
	Type    string    `json:"type"`
	Preamp  float64   `json:"preamp"`
	Bands   []float64 `json:"bands"`
	// Preset is the built-in preset's name, or a user preset's, or empty
	// for a curve of the listener's own.
	Preset string `json:"preset,omitempty"`
	// AutoEQPath and AutoEQName remember the headphone profile applied,
	// if any.
	AutoEQPath string `json:"autoEQPath,omitempty"`
	AutoEQName string `json:"autoEQName,omitempty"`
}

// Default returns the equalizer as it starts: off, flat, ISO 15 bands.
func Default() Settings {
	return Settings{
		Type:   TypeISO15,
		Bands:  make([]float64, BandCount(TypeISO15)),
		Preset: "Flat",
	}
}

// Normalize resets a curve that is incomplete or of an unknown type, as
// an older settings file's might be.
func (s *Settings) Normalize() {
	if (s.Type != TypeISO10 && s.Type != TypeISO15) || len(s.Bands) != BandCount(s.Type) {
		*s = Default()
	}
}
