package pluginapi

// RecordSearch moves the submitted query to the front and keeps up to 20 entries.
func RecordSearch(history []string, query string) []string {
	recent := []string{query}
	for _, past := range history {
		if past != query {
			recent = append(recent, past)
			if len(recent) == 20 {
				break
			}
		}
	}
	return recent
}
