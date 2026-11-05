package netease

// Quality describes one stream level NetEase serves, and what it takes to
// play it. The levels are kept here, rather than in the app, because the
// cache, the player and the settings all need the same order.
type Quality struct {
	// Level is the value to send as level to the eapi.
	Level string
	// Label is what the settings show.
	Label string
	// Short is the badge shown beside a playing track.
	Short string
	// EncodeType is the value to send as encodeType.
	EncodeType string
	// Lossless reports whether the stream is FLAC (or another lossless
	// container).
	Lossless bool
	// Spatial reports whether the stream carries more than stereo, or a
	// spatial rendering baked into stereo.
	Spatial bool
	// Requires is "vip" or "svip" when the level needs a subscription.
	Requires string
}

// Qualities are the NetEase stream levels, from the lowest to the highest.
// The order is also the cache's order: a request may use a song kept at the
// same or a later level.
var Qualities = []Quality{
	{Level: "standard", Label: "标准（128k）", Short: "", EncodeType: "mp3"},
	{Level: "higher", Label: "较高（192k）", Short: "", EncodeType: "mp3"},
	{Level: "exhigh", Label: "极高（320k）", Short: "", EncodeType: "mp3"},
	{Level: "lossless", Label: "无损（FLAC）", Short: "无损", EncodeType: "flac", Lossless: true, Requires: "vip"},
	{Level: "hires", Label: "Hi-Res", Short: "Hi-Res", EncodeType: "flac", Lossless: true, Requires: "vip"},
	{Level: "jyeffect", Label: "高清臻音（96kHz/24bit）", Short: "高清臻音", EncodeType: "flac", Lossless: true, Spatial: true, Requires: "vip"},
	{Level: "sky", Label: "沉浸环绕声（5.1）", Short: "沉浸环绕声", EncodeType: "flac", Lossless: true, Spatial: true, Requires: "svip"},
	{Level: "jymaster", Label: "超清母带（192kHz/24bit）", Short: "超清母带", EncodeType: "flac", Lossless: true, Requires: "svip"},
	{Level: "dolby", Label: "杜比全景声（E-AC-3）", Short: "杜比全景声", EncodeType: "mp4", Spatial: true, Requires: "svip"},
	{Level: "vivid", Label: "臻音全景声（Audio Vivid 7.1.4）", Short: "臻音全景声", EncodeType: "mp4", Spatial: true, Requires: "svip"},
}

// Levels are the stream levels, from the lowest. Kept as a slice of the
// names for the cache and for code that only needs the ordering.
var Levels = func() []string {
	levels := make([]string, len(Qualities))
	for i, q := range Qualities {
		levels[i] = q.Level
	}
	return levels
}()

// QualityOf returns the description of level and whether level is known.
func QualityOf(level string) (Quality, bool) {
	for _, q := range Qualities {
		if q.Level == level {
			return q, true
		}
	}
	return Quality{}, false
}

// LevelLabel returns the settings label of level, or the level itself when
// it is not known.
func LevelLabel(level string) string {
	if q, ok := QualityOf(level); ok {
		return q.Label
	}
	return level
}

// ShortLabel returns the badge label of level, or "" when it needs none.
func ShortLabel(level string) string {
	if q, ok := QualityOf(level); ok {
		return q.Short
	}
	return ""
}

// EncodeType returns the container to ask the eapi for at level.
func EncodeType(level string) string {
	if q, ok := QualityOf(level); ok {
		return q.EncodeType
	}
	return "flac"
}

// Lossless reports whether level streams FLAC. It is kept as a function
// because the player and the badge both use it.
func Lossless(level string) bool {
	q, ok := QualityOf(level)
	return ok && q.Lossless
}

// Spatial reports whether level carries a spatial (multichannel or
// spatialised stereo) stream.
func Spatial(level string) bool {
	q, ok := QualityOf(level)
	return ok && q.Spatial
}

// FallbackLevels are the levels to try, in order, when level cannot be
// played. A server may answer a high level with a lower one, in which case
// the caller should use the answer rather than ask again.
func FallbackLevels(level string) []string {
	switch level {
	case "vivid":
		return []string{"vivid", "dolby", "sky", "jyeffect", "hires", "lossless", "exhigh", "higher", "standard"}
	case "dolby":
		return []string{"dolby", "sky", "jyeffect", "hires", "lossless", "exhigh", "higher", "standard"}
	case "sky":
		return []string{"sky", "jyeffect", "hires", "lossless", "exhigh", "higher", "standard"}
	case "jymaster":
		return []string{"jymaster", "hires", "lossless", "exhigh", "higher", "standard"}
	case "jyeffect":
		return []string{"jyeffect", "hires", "lossless", "exhigh", "higher", "standard"}
	case "hires":
		return []string{"hires", "lossless", "exhigh", "higher", "standard"}
	case "lossless":
		return []string{"lossless", "exhigh", "higher", "standard"}
	case "exhigh":
		return []string{"exhigh", "higher", "standard"}
	case "higher":
		return []string{"higher", "standard"}
	case "standard":
		return []string{"standard"}
	default:
		return []string{level, "lossless", "exhigh", "higher", "standard"}
	}
}
