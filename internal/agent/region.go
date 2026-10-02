package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ManagedRegion names one marker-delimited block that sync-agents owns
// inside a file some other writer (a user, another program, the
// project index) also edits. It is the single owner of the marker
// grammar:
//
//	<!-- sync-agents:<name>:start -->
//	...regenerated wholesale on every write...
//	<!-- sync-agents:<name>:end -->
//
// Everything outside a region's markers belongs to someone else and is
// preserved byte for byte.
type ManagedRegion struct {
	// Name is the region identifier between "sync-agents:" and
	// ":start". Distinct writers must use distinct names so each can
	// find and preserve the others' blocks.
	Name string
}

// OpenClawRulesRegion carries the inlined passive rule text in the
// OpenClaw workspace AGENTS.md. OpenClaw follows neither @-imports nor
// links, so the rule bodies themselves must live in the file.
var OpenClawRulesRegion = ManagedRegion{Name: "openclaw-rules"}

// regionMarkerPrefix is shared by every region's markers; the AGENTS.md
// migration uses it to keep regions written by any writer intact.
const regionMarkerPrefix = "<!-- sync-agents:"

// Start returns the opening marker line (without newline).
func (r ManagedRegion) Start() string { return regionMarkerPrefix + r.Name + ":start -->" }

// End returns the closing marker line (without newline).
func (r ManagedRegion) End() string { return regionMarkerPrefix + r.Name + ":end -->" }

// locate returns the byte span [start, end) of the first well-formed
// region in text, where end includes one trailing newline after the end
// marker when present. ok is false when either marker is missing.
func (r ManagedRegion) locate(text string) (start, end int, ok bool) {
	start = strings.Index(text, r.Start())
	if start < 0 {
		return 0, 0, false
	}
	rel := strings.Index(text[start:], r.End())
	if rel < 0 {
		return 0, 0, false
	}
	end = start + rel + len(r.End())
	if end < len(text) && text[end] == '\n' {
		end++
	}
	return start, end, true
}

// spliceRegion returns existing with r's region replaced by block
// (which must carry both markers and its own trailing newline).
//
// Without a well-formed region (no markers, or a start with no end),
// block is appended after one blank line. Only the first region is
// replaced; a pathological duplicate is left verbatim.
func spliceRegion(existing string, r ManagedRegion, block string) string {
	start, end, ok := r.locate(existing)
	if !ok {
		if existing == "" {
			return block
		}
		sep := "\n"
		if !strings.HasSuffix(existing, "\n") {
			sep = "\n\n"
		}
		return existing + sep + block
	}
	return existing[:start] + block + existing[end:]
}

// stripRegion removes r's region from existing, reporting whether one
// was found. When the region ends the file, the blank separator line
// spliceRegion added in front of it goes too, so splice-then-strip
// restores the original bytes of a newline-terminated file.
func stripRegion(existing string, r ManagedRegion) (string, bool) {
	start, end, ok := r.locate(existing)
	if !ok {
		return existing, false
	}
	head, tail := existing[:start], existing[end:]
	if tail == "" && strings.HasSuffix(head, "\n\n") {
		head = head[:len(head)-1]
	}
	return head + tail, true
}

// regionBanner opens every entries region so a reader of the host file
// knows where the text comes from and where to change it.
const regionBanner = "<!-- managed by sync-agents global sync from ~/.agents/; do not edit between the markers -->"

// renderRegion returns existing with r's region regenerated from
// entries: banner, then one `## <name>` section per entry, the same
// body concat files carry. Pure apart from reading the entry sources,
// so status compares against exactly what RegenerateRegion writes.
func renderRegion(existing string, r ManagedRegion, entries []ConcatEntry) (string, error) {
	body, err := buildEntriesBody(entries)
	if err != nil {
		return "", err
	}
	block := r.Start() + "\n" + regionBanner + "\n" + string(body) + r.End() + "\n"
	return spliceRegion(existing, r, block), nil
}

// RegenerateRegion rewrites r's region in the host file at path from
// entries, leaving every byte outside the markers alone. The host file
// must already exist: it belongs to another program, and creating it
// could stop that program from seeding its own default. An unchanged
// result is not written (mtime preserved). Returns the resulting file
// length in characters alongside the changed flag so callers can check
// it against a size cap.
func RegenerateRegion(path string, r ManagedRegion, entries []ConcatEntry) (changed bool, chars int, err error) {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, 0, err
	}
	out, err := renderRegion(string(existing), r, entries)
	if err != nil {
		return false, 0, err
	}
	changed, err = writeIfChanged(path, []byte(out))
	return changed, utf8.RuneCountInString(out), err
}

// regionStartPattern finds any region's start marker and captures its
// name.
var regionStartPattern = regexp.MustCompile(regexp.QuoteMeta(regionMarkerPrefix) + `([A-Za-z0-9_.-]+):start -->`)

// writeIfChanged atomically replaces path with content unless the file
// already holds exactly those bytes, in which case nothing is written
// and mtime is preserved. The parent directory must exist.
//
// A symlink at path is resolved first so the write lands on its target
// instead of replacing the link with a regular file. An existing
// file's permission bits are kept; a new file is 0644.
func writeIfChanged(path string, content []byte) (bool, error) {
	return writeAtomic(path, content, nil)
}

// writeAtomic is writeIfChanged with an optional precondition. When
// check is non-nil it is called with the file's current bytes (nil
// when the file does not exist) before any write and again just
// before the rename; an error from it aborts the write and is
// returned unchanged. writeIfUnchanged uses it for compare-and-swap.
func writeAtomic(path string, content []byte, check func(current []byte) error) (bool, error) {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o644)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if check != nil {
		if err := check(existing); err != nil {
			return false, err
		}
	}
	if err == nil {
		if bytes.Equal(existing, content) {
			return false, nil
		}
		if fi, err := os.Stat(path); err == nil {
			mode = fi.Mode().Perm()
		}
	}

	// Same directory keeps the rename on one filesystem, so it stays
	// atomic.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if check != nil {
		current, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if err := check(current); err != nil {
			return false, err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return false, err
	}
	return true, nil
}
