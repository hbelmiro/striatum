package selfcontained

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

const (
	kindDirSkill  = "skills"
	kindDirPrompt = "prompts"
)

// kindDirFor maps an artifact kind to the installer target subdirectory name
// (pkg/installer/targets.go): Skill -> "skills", Prompt -> "prompts".
func kindDirFor(kind string) (string, bool) {
	switch kind {
	case "Skill":
		return kindDirSkill, true
	case "Prompt":
		return kindDirPrompt, true
	}
	return "", false
}

// TreeArtifact describes one artifact of the self-contained image closure for link
// rewriting. Files are the declared spec.files (slash paths).
type TreeArtifact struct {
	Kind   string
	Name   string
	Files  []string
	IsRoot bool // the root artifact's files live at the image root
}

// virtualRoot is the artifact's location in the virtual install tree
// (e.g. "skills/my-skill"), matching pkg/installer targets.
func (a TreeArtifact) virtualRoot() string {
	kd, _ := kindDirFor(a.Kind)
	return path.Join(kd, a.Name)
}

// virtualPath is the install-coordinate location of one declared file.
func (a TreeArtifact) virtualPath(file string) string {
	return path.Join(a.virtualRoot(), file)
}

// imagePath is the file's location in the self-contained image.
func (a TreeArtifact) imagePath(file string) string {
	if a.IsRoot {
		return path.Clean(file)
	}
	kd, _ := kindDirFor(a.Kind)
	return path.Join("deps", kd, a.Name, file)
}

// IsMarkdown reports whether the file should be scanned for Markdown links.
func IsMarkdown(file string) bool {
	lower := strings.ToLower(file)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// RewriteLinks rewrites Markdown link destinations in content (a file named
// ownerFile owned by artifact owner) so that references to artifact-owned
// files in the closure resolve inside the self-contained image tree.
//
// Destinations that resolve into a closure artifact's virtual root but do not
// match a declared file produce an error: exporting would otherwise produce an
// image whose references point at missing content. Destinations outside the
// closure (URLs, anchors, absolute paths, user-project references) are left
// byte-for-byte unchanged. Code fences and inline code spans are not scanned.
func RewriteLinks(content []byte, owner TreeArtifact, ownerFile string, tree []TreeArtifact) ([]byte, error) {
	if !IsMarkdown(ownerFile) {
		return content, nil
	}
	lines := strings.Split(string(content), "\n")
	var fenceMarker byte
	var fenceLen int
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if fenceMarker != 0 {
			// A fence closes only on a marker-only line at least as long as
			// the opening fence, so a shorter fence inside the block (a
			// Markdown example) does not close it.
			if isClosingFence(trimmed, fenceMarker, fenceLen) {
				fenceMarker, fenceLen = 0, 0
			}
			continue
		}
		if m, l := openingFence(trimmed); m != 0 {
			fenceMarker, fenceLen = m, l
			continue
		}
		rewritten, err := rewriteLine(line, owner, ownerFile, tree)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ownerFile, err)
		}
		lines[i] = rewritten
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// openingFence returns the fence marker byte and its run length when the
// line opens a fenced code block, otherwise (0, 0). An opening fence may be
// followed by an info string.
func openingFence(trimmed string) (byte, int) {
	if len(trimmed) < 3 {
		return 0, 0
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0
	}
	return c, n
}

// isClosingFence reports whether the line is a marker-only run of c, at
// least openLen long, that closes an open fence. CommonMark permits
// trailing whitespace on closing fences.
func isClosingFence(trimmed string, c byte, openLen int) bool {
	trimmed = strings.TrimRight(trimmed, " \t")
	if len(trimmed) < openLen {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != c {
			return false
		}
	}
	return true
}

// linkEdit is a replacement of line[start:end] with text.
type linkEdit struct {
	start, end int
	text       string
}

// rewriteLine rewrites all link destinations on a single line of Markdown.
func rewriteLine(line string, owner TreeArtifact, ownerFile string, tree []TreeArtifact) (string, error) {
	var edits []linkEdit
	n := len(line)
	for i := 0; i < n; {
		c := line[i]
		switch {
		case c == '`':
			// Skip inline code span.
			j := strings.IndexByte(line[i+1:], '`')
			if j < 0 {
				i = n
			} else {
				i += j + 2
			}
		case c == ']' && i+1 < n && line[i+1] == '(':
			edit, next, err := parseInlineLink(line, i+2, owner, ownerFile, tree)
			if err != nil {
				return "", err
			}
			if edit != nil {
				edits = append(edits, *edit)
			}
			i = next
		case (c == 's' || c == 'h') && i+4 <= n:
			// HTML img src / a href attributes in Markdown.
			edit, next, err := parseHTMLAttribute(line, i, owner, ownerFile, tree)
			if err != nil {
				return "", err
			}
			if edit != nil {
				edits = append(edits, *edit)
			}
			if next > i {
				i = next
			} else {
				i++
			}
		default:
			i++
		}
	}
	// Reference-style definitions at the start of the line.
	if edit, err := parseReferenceDefinition(line, owner, ownerFile, tree); err != nil {
		return "", err
	} else if edit != nil {
		edits = append(edits, *edit)
	}
	return applyEdits(line, edits), nil
}

// parseHTMLAttribute recognizes a quoted src= or href= attribute at index i
// and returns an edit when its artifact-owned value was rewritten, plus the
// index just after the attribute. Returns (nil, i, nil) when nothing
// matched.
func parseHTMLAttribute(line string, i int, owner TreeArtifact, ownerFile string, tree []TreeArtifact) (*linkEdit, int, error) {
	nameLen := 0
	if strings.HasPrefix(line[i:], "src=") {
		nameLen = len("src=")
	} else if strings.HasPrefix(line[i:], "href=") {
		nameLen = len("href=")
	}
	if nameLen == 0 || i+nameLen >= len(line) {
		return nil, i, nil
	}
	// Only rewrite attributes inside HTML tags: prose quoting "src=..."
	// must be left alone. Scan backwards for the enclosing context; an
	// attribute belongs to a tag when a '<' opens it with no '>' in between.
	if !insideHTMLTag(line, i) {
		return nil, i, nil
	}
	q := line[i+nameLen]
	if q != '"' && q != '\'' {
		return nil, i, nil
	}
	end := strings.IndexByte(line[i+nameLen+1:], q)
	if end < 0 {
		return nil, i, nil
	}
	valueStart := i + nameLen + 1
	valueEnd := valueStart + end
	dest := line[valueStart:valueEnd]
	if dest == "" {
		return nil, valueEnd + 1, nil
	}
	newDest, changed, err := resolveDest(owner, ownerFile, tree, dest)
	if err != nil {
		return nil, valueEnd + 1, err
	}
	if !changed {
		return nil, valueEnd + 1, nil
	}
	return &linkEdit{start: valueStart, end: valueEnd, text: newDest}, valueEnd + 1, nil
}

// parseInlineLink parses the destination starting at index start (just after
// the opening '(' of a "](" sequence) and returns an edit when the
// destination was rewritten, plus the index just after the link.
func parseInlineLink(line string, start int, owner TreeArtifact, ownerFile string, tree []TreeArtifact) (*linkEdit, int, error) {
	n := len(line)
	i := start
	for i < n && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= n {
		return nil, n, nil
	}
	if line[i] == '<' {
		end := strings.IndexByte(line[i+1:], '>')
		if end < 0 {
			return nil, n, nil
		}
		destStart, destEnd := i+1, i+1+end
		after := destEnd + 1
		for after < n && (line[after] == ' ' || line[after] == '\t') {
			after++
		}
		if after < n && line[after] == '"' {
			if q := strings.IndexByte(line[after+1:], '"'); q >= 0 {
				after = after + 1 + q + 1
			}
		}
		if after >= n || line[after] != ')' {
			return nil, n, nil
		}
		return editForDest(line, destStart, destEnd, after+1, owner, ownerFile, tree)
	}
	// Bare destination: ends at the ')' closing the link (tracking balanced
	// parentheses, which are legal in destinations) or before a quoted
	// title. Backslash escapes skip the next character.
	destStart := i
	destEnd := -1
	depth := 0
	for j := i; j < n; j++ {
		c := line[j]
		if c == '\\' && j+1 < n {
			j++
			continue
		}
		if c == '(' {
			depth++
			continue
		}
		if c == ')' {
			if depth == 0 {
				destEnd = j
				break
			}
			depth--
			continue
		}
		if depth == 0 && (c == ' ' || c == '\t') && j+1 < n && line[j+1] == '"' {
			destEnd = j
			break
		}
	}
	if destEnd < 0 {
		return nil, n, nil
	}
	closing := destEnd
	if line[closing] != ')' {
		// A quoted title follows the destination: dest "title").
		if closing+1 >= n || line[closing+1] != '"' {
			return nil, n, nil
		}
		q := strings.IndexByte(line[closing+2:], '"')
		if q < 0 || closing+2+q+1 >= n || line[closing+2+q+1] != ')' {
			return nil, n, nil
		}
		closing = closing + 2 + q + 1
	}
	return editForDest(line, destStart, destEnd, closing+1, owner, ownerFile, tree)
}

// editForDest computes the replacement for the destination at
// line[destStart:destEnd]; the link ends at index endExclusive.
func editForDest(line string, destStart, destEnd, endExclusive int, owner TreeArtifact, ownerFile string, tree []TreeArtifact) (*linkEdit, int, error) {
	dest := line[destStart:destEnd]
	if dest == "" {
		return nil, endExclusive, nil
	}
	newDest, changed, err := resolveDest(owner, ownerFile, tree, dest)
	if err != nil {
		return nil, endExclusive, err
	}
	if !changed {
		return nil, endExclusive, nil
	}
	return &linkEdit{start: destStart, end: destEnd, text: newDest}, endExclusive, nil
}

// parseReferenceDefinition detects a reference-style link definition at the
// start of the line ("[label]: dest") and returns an edit for its destination.
func parseReferenceDefinition(line string, owner TreeArtifact, ownerFile string, tree []TreeArtifact) (*linkEdit, error) {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 4 {
		i++
	}
	if i >= len(line) || line[i] != '[' {
		return nil, nil
	}
	closeBrackets := strings.IndexByte(line[i+1:], ']')
	if closeBrackets < 0 {
		return nil, nil
	}
	j := i + 1 + closeBrackets + 1
	if j >= len(line) || line[j] != ':' {
		return nil, nil
	}
	j++
	sawSpace := false
	for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
		sawSpace = true
		j++
	}
	if !sawSpace || j >= len(line) {
		return nil, nil
	}
	var destStart, destEnd int
	if line[j] == '<' {
		end := strings.IndexByte(line[j+1:], '>')
		if end < 0 {
			return nil, nil
		}
		destStart, destEnd = j+1, j+1+end
	} else {
		destStart = j
		k := j
		for k < len(line) && line[k] != ' ' && line[k] != '\t' {
			k++
		}
		destEnd = k
	}
	if destEnd <= destStart {
		return nil, nil
	}
	dest := line[destStart:destEnd]
	newDest, changed, err := resolveDest(owner, ownerFile, tree, dest)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return &linkEdit{start: destStart, end: destEnd, text: newDest}, nil
}

func applyEdits(line string, edits []linkEdit) string {
	if len(edits) == 0 {
		return line
	}
	// Edits are collected out of order (inline links first, then the
	// reference definition at line start); apply left to right.
	slices.SortFunc(edits, func(a, b linkEdit) int { return a.start - b.start })
	var b strings.Builder
	prev := 0
	for _, e := range edits {
		if e.start < prev {
			continue // overlapping edit; keep the first
		}
		b.WriteString(line[prev:e.start])
		b.WriteString(e.text)
		prev = e.end
	}
	b.WriteString(line[prev:])
	return b.String()
}

// relSlash returns a slash-separated relative path from directory fromDir to
// toFile (both slash paths), like filepath.Rel but portable across platforms.
func relSlash(fromDir, toFile string) (string, error) {
	if fromDir == "" || fromDir == "." {
		return toFile, nil
	}
	fromParts := strings.Split(path.Clean(fromDir), "/")
	toParts := strings.Split(path.Clean(toFile), "/")
	i := 0
	for i < len(fromParts) && i < len(toParts) && fromParts[i] == toParts[i] {
		i++
	}
	var ups []string
	for j := i; j < len(fromParts); j++ {
		if fromParts[j] == ".." {
			return "", fmt.Errorf("cannot compute relative path from %q", fromDir)
		}
		ups = append(ups, "..")
	}
	rel := strings.Join(append(ups, toParts[i:]...), "/")
	if rel == "" {
		return ".", nil
	}
	return rel, nil
}

// hasScheme reports whether dest is a URL (scheme:path).
func hasScheme(dest string) bool {
	for i := 0; i < len(dest); i++ {
		c := dest[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			if i == 0 {
				return false // scheme must start with a letter
			}
		case c == ':':
			return i > 0
		default:
			return false
		}
	}
	return false
}

// resolveDest maps one link destination to its exported-image equivalent.
// changed is false when the destination is not artifact-owned (or needs no
// change). An error is returned for artifact-owned destinations that do not
// match a declared file of a closure artifact.
func resolveDest(owner TreeArtifact, ownerFile string, tree []TreeArtifact, dest string) (string, bool, error) {
	if dest == "" || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") || hasScheme(dest) {
		return dest, false, nil
	}
	// Unescape Markdown-escaped parentheses so "file\(1\).md" resolves as
	// the declared file "file(1).md".
	dest = strings.ReplaceAll(strings.ReplaceAll(dest, `\(`, "("), `\)`, ")")
	filePart := dest
	fragment := ""
	if i := strings.Index(filePart, "#"); i >= 0 {
		fragment = filePart[i:]
		filePart = filePart[:i]
	}
	if filePart == "" {
		return dest, false, nil
	}
	srcVirtualDir := path.Dir(owner.virtualPath(ownerFile))
	resolved := path.Clean(path.Join(srcVirtualDir, filePart))
	for _, a := range tree {
		root := a.virtualRoot()
		if resolved != root && !strings.HasPrefix(resolved, root+"/") {
			continue
		}
		if resolved == root {
			return "", false, fmt.Errorf("link %q references the directory of artifact %s/%s; link to a declared file instead", dest, a.Kind, a.Name)
		}
		rel := strings.TrimPrefix(resolved, root+"/")
		if !slices.Contains(a.Files, rel) {
			return "", false, fmt.Errorf("link %q targets %q of artifact %s/%s, which is not declared in its spec.files; add the file to the artifact or remove the reference", dest, rel, a.Kind, a.Name)
		}
		newDest, err := relSlash(path.Dir(owner.imagePath(ownerFile)), a.imagePath(rel))
		if err != nil {
			return "", false, fmt.Errorf("relink %q: %w", dest, err)
		}
		return newDest + fragment, true, nil
	}
	// Outside every closure artifact: user-project or other external content.
	return dest, false, nil
}

// insideHTMLTag reports whether index i in line falls inside an HTML tag:
// scanning backwards, a '<' (followed by a letter or '/') is reached before
// any '>'.
func insideHTMLTag(line string, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch line[j] {
		case '>':
			return false
		case '<':
			return j+1 < len(line) && (isAlphaByte(line[j+1]) || line[j+1] == '/')
		}
	}
	return false
}

func isAlphaByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
