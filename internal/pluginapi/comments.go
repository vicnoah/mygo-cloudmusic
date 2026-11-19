package pluginapi

// Comment is what a listener wrote about a song or an episode.
type Comment struct {
	ID   int64 `json:"commentId"`
	User struct {
		ID        int64  `json:"userId"`
		Nickname  string `json:"nickname"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"user"`
	Content    string `json:"content"`
	Time       int64  `json:"time"` // Unix milliseconds
	LikedCount int    `json:"likedCount"`
	Liked      bool   `json:"liked"` // by the listener signed in
	IPLocation struct {
		Location string `json:"location"`
	} `json:"ipLocation"`
	// BeReplied holds the comment this one replies to.
	BeReplied []struct {
		User struct {
			Nickname string `json:"nickname"`
		} `json:"user"`
		Content string `json:"content"`
	} `json:"beReplied"`
}

// Comments are a page of a song's or an episode's comments.
type Comments struct {
	// Hot are the comments most liked, on the first page only.
	Hot    []Comment `json:"hotComments"`
	Latest []Comment `json:"comments"`
	Total  int       `json:"total"`
	More   bool      `json:"more"`
}
