package network

import (
	"fmt"
	"strings"
)

// SizedArtworkURL keeps the cache and request format of the original artwork
// references. Plugin references carry their rendering size in the size query.
func SizedArtworkURL(pic string, n int) string {
	if pic == "" {
		return ""
	}
	if !strings.HasPrefix(pic, "http") {
		return fmt.Sprintf("%s?size=%d", pic, n)
	}
	pic = strings.Replace(pic, "http://", "https://", 1)
	return fmt.Sprintf("%s?param=%dy%d", pic, n, n)
}
