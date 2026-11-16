package oci

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// digestShapedPattern matches "algorithm:hexchars" (algorithm per the OCI
// digest spec, hex of any length): candidates that look like a digest but
// fail validation get a clear error rather than silent path reinterpretation.
var digestShapedPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.+_-][a-z0-9]+)*:[0-9a-fA-F]*$`)

// SplitReference splits a reference into (repo-or-path, tag, digest).
// For "oci:" references ("oci:/path:tag" or "oci:/path@sha256:<64hex>" for
// digest-only), the tag is after the first colon in the rest (or last colon
// for Windows drive letters like "oci:C:\path:tag").
// For remote references ("host/repo/name:tag" or "host/repo/name:tag@digest"),
// the optional @digest suffix is stripped before splitting on the tag colon, which
// must come after the last slash so registry ports ("localhost:5000/repo") are not
// mistaken for a tag. Digest-only references ("host/repo@sha256:<64hex>") are
// accepted and return an empty tag with the validated digest.
func SplitReference(reference string) (repo, tag, digest string, err error) {
	if strings.HasPrefix(reference, "oci:") {
		rest := reference[len("oci:"):]
		// A trailing "@sha256:<64hex>" is a digest suffix; a bare "@" inside
		// the layout path is left alone (paths may legitimately contain it).
		// A digest-shaped but invalid suffix (e.g. a typo'd "sha256:abc") is
		// rejected with a clear error instead of being reparsed as a path.
		if atIdx := strings.LastIndex(rest, "@"); atIdx >= 0 {
			candidate := rest[atIdx+1:]
			if artifact.IsValidDigest(candidate) {
				digest = candidate
				rest = rest[:atIdx]
			} else if digestShapedPattern.MatchString(candidate) {
				return "", "", "", fmt.Errorf("invalid oci reference %q: invalid digest %q", reference, candidate)
			}
		}
		i := strings.Index(rest, ":")
		if i < 0 {
			if digest != "" {
				return rest, "", digest, nil
			}
			return "", "", "", fmt.Errorf("invalid oci reference %q: missing tag", reference)
		}
		// Windows drive letter: "C:\path:tag" — first colon is the drive separator, not the tag delimiter.
		if i == 1 && len(rest) > 2 && (rest[2] == '\\' || rest[2] == '/') {
			if j := strings.LastIndex(rest, ":"); j > i {
				i = j
			} else {
				// The drive colon is the only one: there is no tag. With a
				// digest the identity is carried by the digest; otherwise
				// the tag is genuinely missing.
				if digest != "" {
					return rest, "", digest, nil
				}
				return "", "", "", fmt.Errorf("invalid oci reference %q: missing tag", reference)
			}
		}
		return rest[:i], rest[i+1:], digest, nil
	}
	if atIdx := strings.Index(reference, "@"); atIdx >= 0 {
		digest = reference[atIdx+1:]
		reference = reference[:atIdx]
	}
	// The tag separator is the last colon after the last slash, so a host port
	// ("localhost:5000") is never parsed as a tag.
	lastSlash := strings.LastIndex(reference, "/")
	if i := strings.LastIndex(reference, ":"); i > lastSlash {
		return reference[:i], reference[i+1:], digest, nil
	}
	if digest != "" {
		if !artifact.IsValidDigest(digest) {
			return "", "", "", fmt.Errorf("invalid reference %q: invalid digest %q", reference, digest)
		}
		return reference, "", digest, nil
	}
	return "", "", "", fmt.Errorf("invalid reference %q: missing tag", reference)
}
