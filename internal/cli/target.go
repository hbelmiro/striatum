package cli

import (
	"fmt"
	"slices"
	"strings"
)

var validTargets = []string{"cursor", "claude", "codex"}

func validateTarget(raw string) (string, error) {
	t := strings.TrimSpace(raw)
	if slices.Contains(validTargets, t) {
		return t, nil
	}
	return "", fmt.Errorf("--target must be cursor, claude, or codex, got %q", t)
}
