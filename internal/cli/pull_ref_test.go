package cli

import "testing"

// pullRefFor decides what reference the self-contained flows pull with: the
// immutable digest for registry refs, the tag for oci: layout refs (layout
// stores are tag-indexed). A regression to tag-based pulling reintroduces
// the tag-movement TOCTOU.
func TestPullRefFor(t *testing.T) {
	digest := "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	tests := []struct {
		reference, ref, want string
	}{
		{"localhost:5000/skills/x:1.0.0", "1.0.0", digest},
		{"localhost:5000/skills/x:1.0.0@" + digest, "1.0.0", digest},
		{"localhost:5000/skills/x@" + digest, digest, digest},
		{"oci:/path/to/layout", "x:1.0.0", "x:1.0.0"},
	}
	for _, tt := range tests {
		if got := pullRefFor(tt.reference, tt.ref, digest); got != tt.want {
			t.Errorf("pullRefFor(%q, %q) = %q, want %q", tt.reference, tt.ref, got, tt.want)
		}
	}
}
