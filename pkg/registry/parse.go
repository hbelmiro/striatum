package registry

import (
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// ParseReference parses a CLI reference string into a Locator.
//
// Supported formats:
//   - "host/repo/name:tag"              -> *artifact.OCIDependency
//   - "host/repo/name:tag@digest"       -> *artifact.OCIDependency (with digest)
//   - "oci:path:tag"                    -> *OCILayoutLocator
//   - "git:URL@ref"                     -> *artifact.GitDependency
//   - "git:URL@ref#path"               -> *artifact.GitDependency (with sub-path)
//   - "git:URL@ref!commit"             -> *artifact.GitDependency (with commit)
//   - "git:URL@ref#path!commit"        -> *artifact.GitDependency (with sub-path and commit)
//
// The characters '#' and '!' are reserved delimiters in git references.
// Git refs or sub-paths containing these characters cannot be represented
// in this format.
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
	commit := ""
	if hashIdx := strings.Index(refAndPath, "#"); hashIdx >= 0 {
		gitRef = refAndPath[:hashIdx]
		path = refAndPath[hashIdx+1:]
		if bangIdx := strings.Index(path, "!"); bangIdx >= 0 {
			commit = path[bangIdx+1:]
			path = path[:bangIdx]
		}
	} else if bangIdx := strings.Index(refAndPath, "!"); bangIdx >= 0 {
		gitRef = refAndPath[:bangIdx]
		commit = refAndPath[bangIdx+1:]
	}

	url = strings.TrimSpace(url)
	gitRef = strings.TrimSpace(gitRef)
	path = strings.TrimSpace(path)
	commit = strings.TrimSpace(commit)

	if url == "" {
		return nil, fmt.Errorf("invalid git reference %q: empty URL", ref)
	}
	if gitRef == "" {
		return nil, fmt.Errorf("invalid git reference %q: empty ref", ref)
	}
	if strings.Contains(gitRef, "!") {
		return nil, fmt.Errorf("invalid git reference %q: commit delimiter '!' must appear after ref (and optional #path), not before '#'", ref)
	}
	if strings.ContainsRune(refAndPath, '#') && path == "" {
		return nil, fmt.Errorf("invalid git reference %q: empty path after '#'", ref)
	}
	if commit == "" && strings.ContainsRune(refAndPath, '!') {
		return nil, fmt.Errorf("invalid git reference %q: empty commit after '!'", ref)
	}
	if commit != "" && !artifact.IsValidCommitSHA(commit) {
		return nil, fmt.Errorf("invalid git reference %q: commit must be a 40-character lowercase hex SHA", ref)
	}
	return &artifact.GitDependency{URL: url, Ref: gitRef, Path: path, Commit: commit}, nil
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
	originalRef := ref
	digest := ""
	if atIdx := strings.Index(ref, "@"); atIdx >= 0 {
		digest = strings.TrimSpace(ref[atIdx+1:])
		if digest == "" {
			return nil, fmt.Errorf("invalid reference %q: empty digest after @", originalRef)
		}
		if !artifact.IsValidDigest(digest) {
			return nil, fmt.Errorf("invalid reference %q: digest must match sha256:<64 lowercase hex chars>", originalRef)
		}
		ref = ref[:atIdx]
	}

	lastSlash := strings.LastIndex(ref, "/")
	if lastSlash < 0 {
		return nil, fmt.Errorf("invalid reference %q: missing repository path (expected host/repo:tag)", originalRef)
	}

	// Search from lastSlash to avoid confusing host:port with the tag colon.
	tagColon := strings.LastIndex(ref[lastSlash:], ":")
	if tagColon < 0 {
		return nil, fmt.Errorf("invalid reference %q: missing tag", originalRef)
	}
	tagColon += lastSlash

	repoWithHost := strings.TrimSpace(ref[:tagColon])
	tag := strings.TrimSpace(ref[tagColon+1:])

	if repoWithHost == "" || tag == "" {
		return nil, fmt.Errorf("invalid reference %q: empty host/repository or tag", originalRef)
	}

	firstSlash := strings.Index(repoWithHost, "/")
	if firstSlash < 0 {
		return nil, fmt.Errorf("invalid reference %q: missing repository path (expected host/repo:tag)", originalRef)
	}

	host := repoWithHost[:firstSlash]
	repository := repoWithHost[firstSlash+1:]
	if host == "" || repository == "" {
		return nil, fmt.Errorf("invalid reference %q: empty host or repository", originalRef)
	}

	return &artifact.OCIDependency{
		RegistryHost: host,
		Repository:   repository,
		Tag:          tag,
		Digest:       digest,
	}, nil
}
