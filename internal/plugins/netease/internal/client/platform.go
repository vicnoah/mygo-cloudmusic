package netease

func usesNetEaseURL(t *Track) bool { return t == nil || t.Source == "" }
