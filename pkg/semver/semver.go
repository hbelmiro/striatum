package semver

import (
	"sort"
	"strings"

	modsemver "golang.org/x/mod/semver"
)

func canon(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

func strip(v string) string {
	return strings.TrimPrefix(v, "v")
}

// FilterValid returns only the tags that are valid semver versions, sorted descending.
// Tags may optionally have a "v" prefix; the returned values never have one.
func FilterValid(tags []string) []string {
	var valid []string
	for _, t := range tags {
		if modsemver.IsValid(canon(t)) {
			valid = append(valid, strip(t))
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		return modsemver.Compare(canon(valid[i]), canon(valid[j])) > 0
	})
	return valid
}

// LatestVersion returns the highest semver tag from tags, or "" if none are valid.
// Pre-release versions are lower than their release counterpart.
func LatestVersion(tags []string) string {
	filtered := FilterValid(tags)
	if len(filtered) == 0 {
		return ""
	}
	return filtered[0]
}

// Compare returns -1 if a < b, 0 if a == b, 1 if a > b.
func Compare(a, b string) int {
	return modsemver.Compare(canon(a), canon(b))
}
