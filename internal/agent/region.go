package agent

import (
	"bytes"
	"fmt"
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

// ClaudeImportsRegion carries the @-import lines that make Claude load
// passive rules (CLAUDE.md globally, AGENTS.md per project).
var ClaudeImportsRegion = ManagedRegion{Name: "claude-imports"}

// OpenClawRulesRegion carries the inlined passive rule text in the
// OpenClaw workspace AGENTS.md. OpenClaw follows neither @-imports nor
// links, so the rule bodies themselves must live in the file.
var OpenClawRulesRegion = ManagedRegion{Name: "openclaw-rules"}

// regionMarkerPrefix is shared by every region's markers; index uses
// it to recognize regions written by writers it does not know about.
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

// regionHeadroomPct is how much of a host's cap fitting aims to leave
// unused, so a small edit to the host or a rule does not immediately
// push the file back over.
const regionHeadroomPct = 10

// RegionOverBudgetError reports a region that cannot fit its host's
// size cap even with every entry reduced to a summary.
type RegionOverBudgetError struct {
	Chars, Limit int
}

func (e *RegionOverBudgetError) Error() string {
	return fmt.Sprintf("host file would be %d chars with every entry summarized, over the %d-char cap", e.Chars, e.Limit)
}

// renderRegion returns existing with r's region regenerated from
// entries: banner, then one `## <name>` section per entry, the same
// body concat files carry. Pure apart from reading the entry sources,
// so status compares against exactly what RegenerateRegion writes.
//
// A positive limit caps the resulting host file in characters. While
// the file is over limit minus headroom, the entry with the largest
// body is reduced to a summary that points at its source file. If the
// file is still over limit with every entry summarized, it returns a
// *RegionOverBudgetError and no text, so callers never write a file
// the host program would truncate.
func renderRegion(existing string, r ManagedRegion, entries []ConcatEntry, limit int) (string, error) {
	fitted := make([]ConcatEntry, len(entries))
	copy(fitted, entries)
	render := func() (string, error) {
		body, err := buildEntriesBody(fitted)
		if err != nil {
			return "", err
		}
		block := r.Start() + "\n" + regionBanner + "\n" + string(body) + r.End() + "\n"
		return spliceRegion(existing, r, block), nil
	}

	out, err := render()
	if err != nil || limit <= 0 {
		return out, err
	}
	target := limit - limit*regionHeadroomPct/100

	sizes := make([]int, len(fitted))
	for i, e := range fitted {
		b, err := readArtifactBody(e.SourcePath)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", e.SourcePath, err)
		}
		sizes[i] = len(b)
	}
	for utf8.RuneCountInString(out) > target {
		largest := -1
		for i, e := range fitted {
			if e.Summary {
				continue
			}
			if largest < 0 || sizes[i] > sizes[largest] || (sizes[i] == sizes[largest] && e.Name < fitted[largest].Name) {
				largest = i
			}
		}
		if largest < 0 {
			break
		}
		fitted[largest].Summary = true
		if out, err = render(); err != nil {
			return "", err
		}
	}
	if n := utf8.RuneCountInString(out); n > limit {
		return "", &RegionOverBudgetError{Chars: n, Limit: limit}
	}
	return out, nil
}

// RegenerateRegion rewrites r's region in the host file at path from
// entries, leaving every byte outside the markers alone. The host file
// must already exist: it belongs to another program, and creating it
// could stop that program from seeding its own default. An unchanged
// result is not written (mtime preserved). limit is the host's size
// cap in characters, 0 for none; see renderRegion. Returns the
// resulting file length in characters alongside the changed flag.
func RegenerateRegion(path string, r ManagedRegion, entries []ConcatEntry, limit int) (changed bool, chars int, err error) {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, 0, err
	}
	out, err := renderRegion(string(existing), r, entries, limit)
	if err != nil {
		return false, 0, err
	}
	changed, err = writeIfChanged(path, []byte(out))
	return changed, utf8.RuneCountInString(out), err
}

// regionStartPattern finds any region's start marker and captures its
// name.
var regionStartPattern = regexp.MustCompile(regexp.QuoteMeta(regionMarkerPrefix) + `([A-Za-z0-9_.-]+):start -->`)

// foreignRegions returns every well-formed sync-agents region in text
// other than own, verbatim (markers and one trailing newline included)
// and in document order. A start marker with no matching end is not a
// region and is skipped.
func foreignRegions(text string, own ManagedRegion) []string {
	var out []string
	for pos := 0; pos < len(text); {
		m := regionStartPattern.FindStringSubmatchIndex(text[pos:])
		if m == nil {
			break
		}
		r := ManagedRegion{Name: text[pos+m[2] : pos+m[3]]}
		start, end, ok := r.locate(text[pos+m[0]:])
		if !ok {
			pos += m[1]
			continue
		}
		if r != own {
			out = append(out, text[pos+m[0]+start:pos+m[0]+end])
		}
		pos += m[0] + end
	}
	return out
}

// writeIfChanged atomically replaces path with content unless the file
// already holds exactly those bytes, in which case nothing is written
// and mtime is preserved. The parent directory must exist.
//
// A symlink at path is resolved first so the write lands on its target
// instead of replacing the link with a regular file. An existing
// file's permission bits are kept; a new file is 0644.
func writeIfChanged(path string, content []byte) (bool, error) {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o644)
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, content) {
			return false, nil
		}
		if fi, err := os.Stat(path); err == nil {
			mode = fi.Mode().Perm()
		}
	} else if !os.IsNotExist(err) {
		return false, err
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
	if err := os.Rename(tmpName, path); err != nil {
		return false, err
	}
	return true, nil
}
