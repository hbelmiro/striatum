package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// ValidateMemoryLinks scans files in memoryDir/<artifactName>/ for [[name]]
// patterns and checks if each referenced name matches a name: frontmatter
// field in any .md file under memoryDir/. Returns warning messages for broken links.
func ValidateMemoryLinks(memoryDir, artifactName string) []string {
	artDir := filepath.Join(memoryDir, artifactName)
	links := collectLinks(artDir)
	if len(links) == 0 {
		return nil
	}

	knownNames := collectKnownNames(memoryDir)

	var warnings []string
	for link := range links {
		if !knownNames[link] {
			warnings = append(warnings, fmt.Sprintf("broken [[%s]] link in %s", link, artifactName))
		}
	}
	return warnings
}

func collectLinks(dir string) map[string]bool {
	links := make(map[string]bool)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, match := range wikiLinkPattern.FindAllSubmatch(data, -1) {
			links[string(match[1])] = true
		}
		return nil
	})
	return links
}

func collectKnownNames(memoryDir string) map[string]bool {
	names := make(map[string]bool)
	_ = filepath.Walk(memoryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		name, _, fmErr := ParseMemoryFrontmatter(data)
		if fmErr == nil && name != "" {
			names[name] = true
		}
		return nil
	})
	return names
}
