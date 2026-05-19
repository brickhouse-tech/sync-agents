package version

import "runtime/debug"

// Version is overridden at build time via -ldflags. For `go install` builds
// where ldflags are not injected, init() falls back to debug.ReadBuildInfo
// which embeds the module version from the module proxy.
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			Version = v
		}
	}
}
