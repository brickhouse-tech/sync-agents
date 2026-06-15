package version

import (
	"fmt"
	"testing"
)

// TestDefaultVersion confirms the package-level Version starts as "dev"
// before any ldflags or BuildInfo override is applied at link / init time.
// (In `go test`, BuildInfo returns synthetic data, so init() leaves the
// default in place — which is what we want to assert here.)
func TestDefaultVersion(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("default Version: got %q, want %q", Version, "dev")
	}
}

// TestPrintTemplateDefault confirms the `v%s` print template (used in
// main.go) renders the default "dev" value as "vdev" — single v, not "vvdev".
func TestPrintTemplateDefault(t *testing.T) {
	got := fmt.Sprintf("v%s", Version)
	if got != "vdev" {
		t.Fatalf("print template: got %q, want %q", got, "vdev")
	}
}

// TestPrintTemplateWithSemver simulates the ldflags-injected path
// (Version = "0.2.5") and confirms the print template renders a single-v
// "v0.2.5", guarding against the regression in issue #37.
func TestPrintTemplateWithSemver(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = "0.2.5"
	got := fmt.Sprintf("v%s", Version)
	if got != "v0.2.5" {
		t.Fatalf("print template with semver: got %q, want %q", got, "v0.2.5")
	}
}

// TestNormalizeStripsLeadingV is the core regression guard for issue #37.
// BuildInfo's info.Main.Version carries a leading "v"; normalize must strip
// exactly one so the `v%s` print template doesn't double the prefix.
func TestNormalizeStripsLeadingV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tagged release", "v0.2.5", "0.2.5"},
		{"pseudo-version", "v0.2.6-0.20260519194537-f3220db52fd5", "0.2.6-0.20260519194537-f3220db52fd5"},
		{"already bare", "0.2.5", "0.2.5"},
		{"empty", "", ""},
		{"only v", "v", ""},
		// Only the *first* leading v is stripped — defensive against
		// double-v inputs that would otherwise reintroduce the bug.
		{"double v stays once-stripped", "vv0.2.5", "v0.2.5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalize(tc.in); got != tc.want {
				t.Fatalf("normalize(%q): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestPrintTemplateAfterNormalize ties it all together: feed a BuildInfo-shaped
// "v0.2.5" through normalize, then render via the `v%s` template — must yield
// a single-v "v0.2.5".
func TestPrintTemplateAfterNormalize(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = normalize("v0.2.5")
	got := fmt.Sprintf("v%s", Version)
	if got != "v0.2.5" {
		t.Fatalf("post-normalize print: got %q, want %q", got, "v0.2.5")
	}
}
