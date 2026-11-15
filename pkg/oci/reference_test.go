package oci

import (
	"strings"
	"testing"
)

func TestSplitReference(t *testing.T) {
	tests := []struct {
		name       string
		reference  string
		wantRepo   string
		wantTag    string
		wantDigest string
		wantErr    string
	}{
		// --- Remote OCI: existing behavior (regression) ---
		{
			name:      "remote basic",
			reference: "localhost:5000/skills/my-skill:1.0.0",
			wantRepo:  "localhost:5000/skills/my-skill", wantTag: "1.0.0",
		},
		{
			name:      "remote deep path",
			reference: "reg.io/a/b/c:latest",
			wantRepo:  "reg.io/a/b/c", wantTag: "latest",
		},
		{
			name:      "remote host with port",
			reference: "localhost:5050/repo:tag",
			wantRepo:  "localhost:5050/repo", wantTag: "tag",
		},

		// --- Remote OCI: with @digest (new) ---
		{
			name:       "remote with digest",
			reference:  "localhost:5050/skills/my-skill:1.0.0@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantRepo:   "localhost:5050/skills/my-skill",
			wantTag:    "1.0.0",
			wantDigest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:       "remote deep path with digest",
			reference:  "reg.io/a/b:latest@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantRepo:   "reg.io/a/b",
			wantTag:    "latest",
			wantDigest: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},

		// --- OCI layout: existing behavior (regression) ---
		{
			name:      "oci layout basic",
			reference: "oci:./build:1.0.0",
			wantRepo:  "./build", wantTag: "1.0.0",
		},
		{
			name:      "oci layout absolute path",
			reference: "oci:/tmp/layout:latest",
			wantRepo:  "/tmp/layout", wantTag: "latest",
		},
		{
			name:      "oci layout compound tag",
			reference: "oci:/path/layout:my-skill:1.0.0",
			wantRepo:  "/path/layout", wantTag: "my-skill:1.0.0",
		},
		{
			name:      "oci layout windows drive letter",
			reference: `oci:C:\Users\me\build:1.0.0`,
			wantRepo:  `C:\Users\me\build`, wantTag: "1.0.0",
		},

		// --- OCI layout: digest-shaped suffixes are validated ---
		{
			name:      "oci layout with invalid digest suffix in tag rejected",
			reference: "oci:./build:tag@sha256:abc",
			wantErr:   "invalid digest",
		},

		// --- Remote OCI: edge cases ---
		{
			name:       "remote trailing @ returns empty digest",
			reference:  "host/repo:tag@",
			wantRepo:   "host/repo",
			wantTag:    "tag",
			wantDigest: "",
		},
		{
			name:       "remote multiple @ uses first",
			reference:  "host/repo:tag@sha256:abc@extra",
			wantRepo:   "host/repo",
			wantTag:    "tag",
			wantDigest: "sha256:abc@extra",
		},

		// --- Remote OCI: digest-only (no tag) ---
		{
			name:       "remote digest only",
			reference:  "host/repo@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantRepo:   "host/repo",
			wantTag:    "",
			wantDigest: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			name:       "remote digest only with port",
			reference:  "localhost:5000/skills/my-skill@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantRepo:   "localhost:5000/skills/my-skill",
			wantTag:    "",
			wantDigest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:       "remote digest only nested repo",
			reference:  "reg.io/a/b/c@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantRepo:   "reg.io/a/b/c",
			wantTag:    "",
			wantDigest: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},

		// --- OCI layout: digest forms ---
		{
			name:       "oci layout digest only",
			reference:  "oci:/tmp/layout@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantRepo:   "/tmp/layout",
			wantTag:    "",
			wantDigest: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			name:       "oci layout tag and digest",
			reference:  "oci:/tmp/layout:1.0.0@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantRepo:   "/tmp/layout",
			wantTag:    "1.0.0",
			wantDigest: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			name:      "oci layout @ in path is not a digest",
			reference: "oci:/tmp/odd@path:1.0.0",
			wantRepo:  "/tmp/odd@path", wantTag: "1.0.0",
		},
		{
			name:      "oci layout invalid digest suffix rejected",
			reference: "oci:/tmp/layout@sha256:abc",
			wantErr:   "invalid digest",
		},
		{
			name:      "oci layout non-digest colon suffix rejected",
			reference: "oci:/tmp/layout@md5:abcdef0123456789abcdef0123456789",
			wantErr:   "invalid digest",
		},

		// --- Errors ---
		{
			name:      "remote missing tag",
			reference: "no-colon-at-all",
			wantErr:   "missing tag",
		},
		{
			name:      "remote missing tag and digest",
			reference: "host/repo",
			wantErr:   "missing tag",
		},
		{
			name:      "remote digest only invalid digest short",
			reference: "host/repo@sha256:abc",
			wantErr:   "invalid digest",
		},
		{
			name:      "remote digest only not sha256",
			reference: "host/repo@md5:abcdef0123456789abcdef0123456789",
			wantErr:   "invalid digest",
		},
		{
			name:      "remote digest only uppercase hex",
			reference: "host/repo@sha256:ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789",
			wantErr:   "invalid digest",
		},
		{
			name:      "remote empty digest and no tag",
			reference: "host/repo@",
			wantErr:   "missing tag",
		},
		{
			name:      "oci layout missing tag",
			reference: "oci:no-colon",
			wantErr:   "missing tag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tag, digest, err := SplitReference(tt.reference)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("SplitReference(%q) err = nil, want error containing %q", tt.reference, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("SplitReference(%q) err = %q, want error containing %q", tt.reference, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SplitReference(%q) err = %v", tt.reference, err)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
			if digest != tt.wantDigest {
				t.Errorf("digest = %q, want %q", digest, tt.wantDigest)
			}
		})
	}
}
