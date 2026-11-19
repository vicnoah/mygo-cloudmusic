package songloft

type platformPreferences struct {
	Config connectionConfig  `json:"config,omitempty"`
	Sorts  map[string]string `json:"sorts,omitempty"`
}

type connectionConfig struct {
	Address  string `json:"address,omitempty"`
	Username string `json:"username,omitempty"`
	PassHash string `json:"-"`
	DeviceID string `json:"deviceID,omitempty"`
}
