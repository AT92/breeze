package image

import "strings"

// isTransientError returns true for errors that are worth retrying:
// timeouts, rate limits (429), server errors (5xx), and connection resets.
// Non-transient errors (auth failures, not found) return false immediately.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	upperMessage := strings.ToUpper(message)
	lowerMessage := strings.ToLower(message)

	for _, keyword := range nonTransientKeywords {
		if strings.Contains(upperMessage, keyword) {
			return false
		}
	}

	for _, keyword := range transientKeywords {
		if strings.Contains(lowerMessage, keyword) {
			return true
		}
	}

	// Default: retry unknown errors (network issues often have varied messages)
	return true
}

var nonTransientKeywords = []string{
	"UNAUTHORIZED", "DENIED", "403", "401",
	"404", "NOT FOUND", "NAME_UNKNOWN", "MANIFEST_UNKNOWN",
}

var transientKeywords = []string{
	"timeout", "429", "500", "502", "503", "504",
	"connection reset", "connection refused", "eof",
	"tls handshake", "broken pipe", "no such host",
}
