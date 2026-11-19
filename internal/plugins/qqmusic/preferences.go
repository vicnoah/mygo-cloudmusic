package qqmusic

type platformPreferences struct {
	Quality       string   `json:"quality,omitempty"`
	Normalize     *bool    `json:"normalize,omitempty"`
	HotComments   bool     `json:"hotComments,omitempty"`
	SearchHistory []string `json:"searchHistory,omitempty"`
}
