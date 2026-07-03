// Package source implements SPEC-003: declarative installation of
// .agents/ artifacts (rules, skills, workflows, whole trees) from
// upstream GitHub repositories.
//
// The package is deliberately independent of internal/agent — it
// receives the bucket registry as plain data (BucketInfo) so that
// internal/agent can depend on it for CLI glue without an import
// cycle. Everything here operates on an explicit .agents/ directory
// path; scope resolution (project vs --global) is the caller's job.
package source

import (
	"fmt"
	"path"
	"strings"
)

// EntryType is the artifact kind declared by a source entry's prefix.
// String-valued (not iota) so error messages and JSON/YAML output can
// use the value directly without a name table.
type EntryType string

const (
	EntrySkill    EntryType = "skill"
	EntryRule     EntryType = "rule"
	EntryWorkflow EntryType = "workflow"
	EntryTree     EntryType = "tree"
)

// ValidEntryPrefixes lists the accepted entry prefixes for error
// messages, in the order they appear in the spec. Kept as a function
// (not a package var) so callers can't mutate the canonical list.
func ValidEntryPrefixes() []string {
	return []string{"skill:", "rule:", "workflow:", "tree:"}
}

// Entry is one parsed source declaration:
//
//	<type>:<owner>/<repo>[@<ref>][/<path>]
//
// Raw preserves the original string because sources.yaml and
// sources.lock key on the literal entry text — round-tripping through
// the parsed form could normalize away user formatting and orphan
// lock entries.
type Entry struct {
	Type  EntryType
	Owner string
	Repo  string
	// Ref is the tag, branch, or 40-hex commit SHA. Empty means the
	// repo's default branch (resolved as HEAD at fetch time).
	Ref string
	// Path is the in-repo path to the artifact. Empty means the
	// type-specific default (repo root for skill/tree). Always
	// non-empty for rule/workflow — the parser enforces it.
	Path string
	Raw  string
}

// PinnedSHA reports whether the entry's ref is a full 40-hex commit
// SHA. Pinned entries never need ref resolution and are skipped by
// `update` (there is nothing to advance).
func (e Entry) PinnedSHA() bool { return isHexSHA(e.Ref) }

// Name returns the local artifact name the entry installs as:
// the path basename (minus .md for flat files), falling back to the
// repo name for path-less skills. Tree entries have no single
// artifact, so their name is "owner/repo" — used only for --only /
// remove matching and report labels.
func (e Entry) Name() string {
	switch e.Type {
	case EntryTree:
		return e.Owner + "/" + e.Repo
	case EntrySkill:
		if e.Path == "" {
			return e.Repo
		}
		return path.Base(e.Path)
	default: // rule, workflow: path is guaranteed non-empty
		base := path.Base(e.Path)
		return strings.TrimSuffix(base, path.Ext(base))
	}
}

// ParseEntry parses the SPEC-003 entry grammar. Errors are written
// for humans typing entries at a shell: they name the valid prefixes
// and show the expected shape, because "parse error" alone sends the
// user back to the docs.
func ParseEntry(s string) (Entry, error) {
	raw := strings.TrimSpace(s)
	const shape = "<type>:<owner>/<repo>[@ref][/path]"

	colon := strings.Index(raw, ":")
	if colon <= 0 {
		return Entry{}, fmt.Errorf("invalid source entry %q: expected %s with type prefix one of %s",
			raw, shape, strings.Join(ValidEntryPrefixes(), ", "))
	}
	typ := EntryType(raw[:colon])
	switch typ {
	case EntrySkill, EntryRule, EntryWorkflow, EntryTree:
	default:
		return Entry{}, fmt.Errorf("invalid source entry %q: unknown type %q — valid prefixes are %s",
			raw, string(typ)+":", strings.Join(ValidEntryPrefixes(), ", "))
	}

	rest := raw[colon+1:]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return Entry{}, fmt.Errorf("invalid source entry %q: expected %s (missing <owner>/<repo>)", raw, shape)
	}
	owner := rest[:slash]
	rest = rest[slash+1:]

	// The repo segment runs to the first '@' (ref follows) or '/'
	// (path follows). A ref, when present, runs to the next '/'.
	// Branch names containing '/' therefore can't be expressed
	// inline — that limitation is inherited from skillfish's grammar
	// and documented rather than guessed around.
	var repo, ref, artPath string
	end := strings.IndexAny(rest, "@/")
	if end < 0 {
		repo = rest
	} else {
		repo = rest[:end]
		if rest[end] == '@' {
			after := rest[end+1:]
			if cut := strings.Index(after, "/"); cut >= 0 {
				ref = after[:cut]
				artPath = after[cut+1:]
			} else {
				ref = after
			}
		} else { // '/'
			artPath = rest[end+1:]
		}
	}

	if owner == "" || repo == "" || strings.ContainsAny(owner+repo, " \t") {
		return Entry{}, fmt.Errorf("invalid source entry %q: expected %s (bad <owner>/<repo>)", raw, shape)
	}
	if ref == "" && strings.Contains(rest, "@") && end >= 0 && rest[end] == '@' {
		return Entry{}, fmt.Errorf("invalid source entry %q: '@' present but ref is empty", raw)
	}
	artPath = strings.Trim(path.Clean("/"+artPath), "/")
	if artPath == "." {
		artPath = ""
	}
	if (typ == EntryRule || typ == EntryWorkflow) && artPath == "" {
		return Entry{}, fmt.Errorf("invalid source entry %q: %s entries require an in-repo path, e.g. %s:%s/%s@main/%ss/name.md",
			raw, typ, typ, owner, repo, typ)
	}

	return Entry{Type: typ, Owner: owner, Repo: repo, Ref: ref, Path: artPath, Raw: raw}, nil
}

// IsCommitSHA reports whether s is a full 40-hex commit SHA — the
// exported form of the pin check for callers outside this package
// (import's provenance inference).
func IsCommitSHA(s string) bool { return isHexSHA(s) }

// isHexSHA reports whether s is a full 40-character hex commit SHA.
// Case-insensitive: git prints lowercase but users paste from UIs
// that sometimes uppercase.
func isHexSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
