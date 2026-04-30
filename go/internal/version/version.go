package version

// Version is overridden at build time via -ldflags from package.json.
// The fallback is only used when running `go run` without ldflags.
var Version = "dev"
