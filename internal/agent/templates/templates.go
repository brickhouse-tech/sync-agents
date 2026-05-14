// Package templates owns the markdown templates emitted by `sync-agents init`
// and `sync-agents add`. Each template lives in its own .md file and is
// embedded into the binary via go:embed so the templates ship as part of the
// executable rather than being read from disk at runtime.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed *.md
var embedded embed.FS

// FS wraps an fs.FS and exposes DemandFile, a read that treats a missing
// file as a build invariant violation rather than a recoverable runtime
// error. Use it for files declared via go:embed: their absence means the
// binary was built wrong and there is nothing the caller can usefully do.
type FS struct {
	fs fs.FS
}

// New wraps an arbitrary fs.FS. Tests can pass an in-memory FS to
// override the embedded set.
func New(f fs.FS) FS { return FS{fs: f} }

// Default returns an FS backed by the embedded markdown files.
func Default() FS { return FS{fs: embedded} }

// DemandFile reads name from the underlying fs and panics if the read
// fails. The panic is intentional: a missing embedded file indicates a
// broken build, not user input we should tolerate.
func (f FS) DemandFile(name string) string {
	data, err := fs.ReadFile(f.fs, name)
	if err != nil {
		panic(fmt.Sprintf("templates: required embedded file %q missing: %v", name, err))
	}
	return string(data)
}

var defaultFS = Default()

func Rule() string     { return defaultFS.DemandFile("rule.md") }
func Skill() string    { return defaultFS.DemandFile("skill.md") }
func Workflow() string { return defaultFS.DemandFile("workflow.md") }
func State() string    { return defaultFS.DemandFile("state.md") }
