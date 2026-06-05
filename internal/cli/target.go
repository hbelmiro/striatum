package cli

import (
	"fmt"
	"slices"
	"strings"
)

var validTargets = []string{"cursor", "claude"}

func validateTarget(raw string) (string, error) {
	t := strings.TrimSpace(raw)
	if slices.Contains(validTargets, t) {
		return t, nil
	}
	return "", fmt.Errorf("--target must be cursor or claude, got %q", t)
}
