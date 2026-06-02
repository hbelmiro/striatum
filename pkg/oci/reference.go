package oci

import (
	"fmt"
	"strings"
)

// SplitReference splits a reference into (repo-or-path, tag, digest).
// For "oci:" references, the tag is after the first colon in the rest (or last colon
// for Windows drive letters like "oci:C:\path:tag"); digest is always empty.
// For remote references ("host/repo/name:tag" or "host/repo/name:tag@digest"),
// the optional @digest suffix is stripped before splitting on the last colon for the tag.
func SplitReference(reference string) (repo, tag, digest string, err error) {
	if strings.HasPrefix(reference, "oci:") {
		rest := reference[len("oci:"):]
		i := strings.Index(rest, ":")
		if i < 0 {
			return "", "", "", fmt.Errorf("invalid oci reference %q: missing tag", reference)
		}
		// Windows drive letter: "C:\path:tag" — first colon is the drive separator, not the tag delimiter.
		if i == 1 && len(rest) > 2 && (rest[2] == '\\' || rest[2] == '/') {
			i = strings.LastIndex(rest, ":")
		}
		return rest[:i], rest[i+1:], "", nil
	}
	if atIdx := strings.Index(reference, "@"); atIdx >= 0 {
		digest = reference[atIdx+1:]
		reference = reference[:atIdx]
	}
	i := strings.LastIndex(reference, ":")
	if i < 0 {
		return "", "", "", fmt.Errorf("invalid reference %q: missing tag", reference)
	}
	return reference[:i], reference[i+1:], digest, nil
}
