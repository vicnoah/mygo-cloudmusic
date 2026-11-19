package netease

type platformPreferences struct {
	Quality     string `json:"quality"`
	Normalize   bool   `json:"normalize"`
	Captions    bool   `json:"captions"`
	HotComments bool   `json:"hotComments"`
}
