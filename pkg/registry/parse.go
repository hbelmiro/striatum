package registry

import (
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// ParseReference parses a CLI reference string into a Locator.
//
// Supported formats:
//   - "host/repo/name:tag"       -> *artifact.OCIDependency
//   - "oci:path:tag"             -> *OCILayoutLocator
//   - "git:URL@ref"              -> *artifact.GitDependency
//   - "git:URL@ref#path"         -> *artifact.GitDependency (with sub-path)
func ParseReference(ref string) (Locator, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty reference")
	}
	if strings.HasPrefix(ref, "git:") {
		return parseGitReference(ref)
	}
	if strings.HasPrefix(ref, "oci:") {
		return parseOCILayoutReference(ref)
	}
	return parseRemoteOCIReference(ref)
}

func parseGitReference(ref string) (*artifact.GitDependency, error) {
	rest := ref[len("git:"):]
	atIdx := strings.LastIndex(rest, "@")
	if atIdx < 0 {
		return nil, fmt.Errorf("invalid git reference %q: missing @ref", ref)
	}
	url := rest[:atIdx]
	refAndPath := rest[atIdx+1:]

	gitRef := refAndPath
	path := ""
	if hashIdx := strings.Index(refAndPath, "#"); hashIdx >= 0 {
		gitRef = refAndPath[:hashIdx]
		path = refAndPath[hashIdx+1:]
	}

	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("invalid git reference %q: empty URL", ref)
	}
	if strings.TrimSpace(gitRef) == "" {
		return nil, fmt.Errorf("invalid git reference %q: empty ref", ref)
	}
	return &artifact.GitDependency{URL: url, Ref: gitRef, Path: path}, nil
}

func parseOCILayoutReference(ref string) (*OCILayoutLocator, error) {
	rest := ref[len("oci:"):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return nil, fmt.Errorf("invalid oci layout reference %q: missing tag", ref)
	}
	// Windows drive letter: "C:\path:tag" — first colon is the drive separator.
	if colonIdx == 1 && len(rest) > 2 && (rest[2] == '\\' || rest[2] == '/') {
		colonIdx = strings.LastIndex(rest, ":")
	}
	layoutPath := rest[:colonIdx]
	tag := rest[colonIdx+1:]

	if strings.TrimSpace(layoutPath) == "" {
		return nil, fmt.Errorf("invalid oci layout reference %q: empty path", ref)
	}
	if strings.TrimSpace(tag) == "" {
		return nil, fmt.Errorf("invalid oci layout reference %q: empty tag", ref)
	}
	return &OCILayoutLocator{LayoutPath: layoutPath, Tag: tag}, nil
}

func parseRemoteOCIReference(ref string) (*artifact.OCIDependency, error) {
	lastSlash := strings.LastIndex(ref, "/")
	if lastSlash < 0 {
		return nil, fmt.Errorf("invalid reference %q: missing repository path (expected host/repo:tag)", ref)
	}

	// Tag colon must come after the last slash to avoid confusing host:port with tag.
	tagColon := strings.LastIndex(ref[lastSlash:], ":")
	if tagColon < 0 {
		return nil, fmt.Errorf("invalid reference %q: missing tag", ref)
	}
	tagColon += lastSlash

	repoWithHost := strings.TrimSpace(ref[:tagColon])
	tag := strings.TrimSpace(ref[tagColon+1:])

	if repoWithHost == "" || tag == "" {
		return nil, fmt.Errorf("invalid reference %q: empty host/repository or tag", ref)
	}

	firstSlash := strings.Index(repoWithHost, "/")
	if firstSlash < 0 {
		return nil, fmt.Errorf("invalid reference %q: missing repository path (expected host/repo:tag)", ref)
	}

	host := repoWithHost[:firstSlash]
	repository := repoWithHost[firstSlash+1:]
	if host == "" || repository == "" {
		return nil, fmt.Errorf("invalid reference %q: empty host or repository", ref)
	}

	return &artifact.OCIDependency{
		RegistryHost: host,
		Repository:   repository,
		Tag:          tag,
	}, nil
}
