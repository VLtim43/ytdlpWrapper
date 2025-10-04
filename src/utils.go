package src

import (
	"strings"
)

// CleanChannelURL removes common suffixes and query parameters from channel URLs
// Returns empty string only if input is empty or "NA"
func CleanChannelURL(urlStr string) string {
	if urlStr == "" || urlStr == "NA" {
		return ""
	}

	original := urlStr

	// Remove common suffixes like /videos, /shorts, /streams, /playlists
	suffixes := []string{"/videos", "/shorts", "/streams", "/playlists", "/community", "/about"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(urlStr, suffix) {
			urlStr = strings.TrimSuffix(urlStr, suffix)
			break // Only remove one suffix
		}
	}

	// Remove query parameters (e.g., ?feature=..., ?view=...)
	if idx := strings.Index(urlStr, "?"); idx != -1 {
		urlStr = urlStr[:idx]
	}

	// Remove trailing slash
	urlStr = strings.TrimSuffix(urlStr, "/")

	// If cleaning resulted in empty string, return original
	if urlStr == "" {
		return original
	}

	return urlStr
}

// IsChannelURL checks if a URL is a channel URL
func IsChannelURL(urlStr string) bool {
	return strings.Contains(urlStr, "/channel/") ||
		strings.Contains(urlStr, "/@") ||
		strings.Contains(urlStr, "/c/") ||
		strings.Contains(urlStr, "/user/")
}

// IsPlaylistURL checks if a URL is a playlist or channel URL
func IsPlaylistURL(urlStr string) bool {
	// Check for common playlist and channel indicators
	return strings.Contains(urlStr, "/playlist") ||
		strings.Contains(urlStr, "list=") ||
		strings.Contains(urlStr, "/playlists/") ||
		IsChannelURL(urlStr)
}
