package version

import (
	"runtime/debug"
	"strings"
)

// Version is overridden at build time via -ldflags. For `go install` builds
// where ldflags are not injected, init() falls back to debug.ReadBuildInfo
// which embeds the module version from the module proxy.
//
// Print sites in main.go format the version as `sync-agents v%s`, so we
// normalize here by stripping any leading "v" — info.Main.Version carries
// one (e.g. "v0.2.5" or a pseudo-version like
// "v0.2.6-0.20260519194537-f3220db52fd5"), and without the strip the
// `go install` path renders as `sync-agents vv0.2.5` (see issue #37).
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			Version = normalize(v)
		}
	}
}

// normalize strips a single leading "v" from a version string so it can be
// rendered with the `v%s` print template without doubling the prefix.
func normalize(v string) string {
	return strings.TrimPrefix(v, "v")
}
