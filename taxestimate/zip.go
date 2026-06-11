package taxestimate

import (
	"regexp"
	"strings"
)

// zipPattern matches a US ZIP-5 or ZIP+4. We cache and look up rates at ZIP-5
// granularity, so a ZIP+4 is accepted but truncated to its first five digits.
var zipPattern = regexp.MustCompile(`^\d{5}(-\d{4})?$`)

// NormalizeZip validates a raw ZIP and normalizes it to a 5-digit key. It is a
// port of the prototype normalizeZip: it trims surrounding whitespace, accepts a
// ZIP-5 or ZIP+4, and returns the leading five digits. The second return is
// false when the input is not a valid US ZIP; callers treat that as a missing
// location and return a flagged, non-blocking estimate rather than guessing.
func NormalizeZip(raw string) (string, bool) {
	z := strings.TrimSpace(raw)
	if !zipPattern.MatchString(z) {
		return "", false
	}
	return z[:5], true
}
