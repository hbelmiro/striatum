package semver

import (
	"testing"
)

func TestFilterValid_RemovesNonSemver(t *testing.T) {
	tags := []string{"1.0.0", "999-SNAPSHOT", "2.0.0", "bad", "0.5.0"}
	got := FilterValid(tags)
	want := []string{"2.0.0", "1.0.0", "0.5.0"}
	if len(got) != len(want) {
		t.Fatalf("FilterValid() = %v, want %v", got, want)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("FilterValid()[%d] = %q, want %q", i, v, want[i])
		}
	}
}

func TestFilterValid_HandlesVPrefix(t *testing.T) {
	tags := []string{"v1.0.0", "v2.0.0"}
	got := FilterValid(tags)
	want := []string{"2.0.0", "1.0.0"}
	if len(got) != len(want) {
		t.Fatalf("FilterValid() = %v, want %v", got, want)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("FilterValid()[%d] = %q, want %q", i, v, want[i])
		}
	}
}

func TestFilterValid_EmptyInput(t *testing.T) {
	got := FilterValid(nil)
	if len(got) != 0 {
		t.Errorf("FilterValid(nil) = %v, want empty", got)
	}
}

func TestFilterValid_AllInvalid(t *testing.T) {
	got := FilterValid([]string{"SNAPSHOT", "latest", "dev"})
	if len(got) != 0 {
		t.Errorf("FilterValid(all invalid) = %v, want empty", got)
	}
}

func TestLatestVersion_ReturnsHighest(t *testing.T) {
	tags := []string{"1.0.0", "2.0.0", "1.5.0", "999-SNAPSHOT"}
	got := LatestVersion(tags)
	if got != "2.0.0" {
		t.Errorf("LatestVersion() = %q, want %q", got, "2.0.0")
	}
}

func TestLatestVersion_NoValidTags(t *testing.T) {
	got := LatestVersion([]string{"SNAPSHOT", "latest"})
	if got != "" {
		t.Errorf("LatestVersion(no valid) = %q, want empty", got)
	}
}

func TestLatestVersion_PrereleaseLowerThanSamePatch(t *testing.T) {
	tags := []string{"1.0.0-rc1", "1.0.0"}
	got := LatestVersion(tags)
	if got != "1.0.0" {
		t.Errorf("LatestVersion() = %q, want %q (prerelease of same version is lower)", got, "1.0.0")
	}
}

func TestLatestVersion_PrereleaseHigherThanLowerMajor(t *testing.T) {
	tags := []string{"1.0.0", "2.0.0-rc1"}
	got := LatestVersion(tags)
	if got != "2.0.0-rc1" {
		t.Errorf("LatestVersion() = %q, want %q (2.0.0-rc1 > 1.0.0)", got, "2.0.0-rc1")
	}
}

func TestFilterValid_WithBuildMetadata(t *testing.T) {
	tags := []string{"1.0.0+build123", "2.0.0"}
	got := FilterValid(tags)
	if len(got) != 2 {
		t.Fatalf("FilterValid() = %v, want 2 elements", got)
	}
}

func TestFilterValid_SortsPrereleasesCorrectly(t *testing.T) {
	tags := []string{"1.0.0", "2.0.0-rc1", "1.5.0"}
	got := FilterValid(tags)
	want := []string{"2.0.0-rc1", "1.5.0", "1.0.0"}
	if len(got) != len(want) {
		t.Fatalf("FilterValid() = %v, want %v", got, want)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("FilterValid()[%d] = %q, want %q", i, v, want[i])
		}
	}
}

func TestLatestVersion_EmptyInput(t *testing.T) {
	got := LatestVersion(nil)
	if got != "" {
		t.Errorf("LatestVersion(nil) = %q, want empty", got)
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.0", 1},
	}
	for _, tt := range tests {
		got := Compare(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
