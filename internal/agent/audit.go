package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// audit.go implements the SPEC-010 Phase 1 reverse sweep: enumerate
// what actually occupies the managed subdirs of each per-tool
// directory and classify every entry the forward pass (computeStatus)
// did not already claim.
//
// The sweep is read-only and bounded by toolSweepDirs — the tool root
// itself is application state (credentials, sessions, caches) and is
// NEVER enumerated. See SPEC-010 §Hard boundary.

const (
	// StateForeign: an entry in a managed subdir that no .agents/
	// artifact claims and that does not point into the .agents/ tree.
	// Hands-off by rule 2 of SPEC-010 — reported, never mutated.
	StateForeign = "foreign"

	// StateOrphaned: a symlink pointing into the .agents/ tree that no
	// current artifact claims — typically left behind by an artifact
	// removal or rename. Prune candidate.
	StateOrphaned = "orphaned"

	// StateFolded: the expected destination resolves to the canonical
	// artifact through an ancestor symlink (e.g. a dir-level
	// .claude/skills/<name> -> .agents/skills/<name> link). Conformant
	// at a coarser granularity than a fresh sync would create.
	StateFolded = "folded"
)

// toolSweepDirs returns the managed artifact subdirs the audit may
// enumerate for one tool at global scope. Anything not listed here is
// out of bounds — in particular the tool root, which holds the tool's
// own application state.
//
// Copilot and Codex have no per-artifact dirs (single concat file,
// already covered by concat states), so they sweep nothing.
func toolSweepDirs(tool Tool, parent string) []string {
	switch tool.ID {
	case "claude":
		base := filepath.Join(parent, ".claude")
		var dirs []string
		for _, d := range []string{"skills", "commands", "rules", "agents", "plans", "specs", "adrs"} {
			dirs = append(dirs, filepath.Join(base, d))
		}
		return dirs
	case "codeium":
		return []string{filepath.Join(parent, ".codeium", "windsurf", "global_workflows")}
	case "cursor":
		return []string{filepath.Join(parent, ".cursor", "rules")}
	default:
		return nil
	}
}

// sweepUnmanaged enumerates the managed subdirs of each tool and
// returns one StatusEntry per entry the forward pass did not claim,
// classified foreign or orphaned.
//
// expected is the set of destination paths the forward pass computed
// (symlink-strategy destinations only). A directory entry is claimed
// either directly (its own path is expected) or as skill scaffolding
// (its <path>/SKILL.md is expected — Claude skills link the SKILL.md
// inside a per-skill dir).
//
// agentsRoot is the canonical tree; a symlink resolving inside it but
// absent from expected is orphaned rather than foreign.
//
// The sweep is one level deep by design: a foreign directory is one
// row, not a recursive listing — the audit sizes the mess, it doesn't
// inventory other tools' internals.
func sweepUnmanaged(tools []Tool, parent, agentsRoot string, expected map[string]bool) []StatusEntry {
	var rows []StatusEntry
	agentsRootAbs, err := filepath.Abs(agentsRoot)
	if err != nil {
		agentsRootAbs = agentsRoot
	}

	for _, tool := range tools {
		for _, dir := range toolSweepDirs(tool, parent) {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue // absent dir: nothing to audit
			}
			for _, e := range entries {
				full := filepath.Join(dir, e.Name())
				if expected[full] || expected[filepath.Join(full, "SKILL.md")] {
					continue // claimed — the forward pass reports it
				}
				rows = append(rows, classifyUnclaimed(tool.ID, full, agentsRootAbs))
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Tool != rows[j].Tool {
			return rows[i].Tool < rows[j].Tool
		}
		return rows[i].DestinationPath < rows[j].DestinationPath
	})
	return rows
}

// classifyUnclaimed decides foreign vs orphaned for one unclaimed
// entry. Only a symlink whose target lies inside the .agents/ tree is
// orphaned; everything else — real files, real dirs, links elsewhere —
// is foreign and hands-off.
func classifyUnclaimed(toolID, path, agentsRootAbs string) StatusEntry {
	row := StatusEntry{
		Tool:            toolID,
		ArtifactName:    filepath.Base(path),
		DestinationPath: path,
		State:           StateForeign,
		Detail:          "untracked — left alone",
	}

	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return row
	}
	target, err := os.Readlink(path)
	if err != nil {
		return row
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	target = filepath.Clean(target)
	if target == agentsRootAbs || strings.HasPrefix(target, agentsRootAbs+string(filepath.Separator)) {
		row.State = StateOrphaned
		row.Detail = fmt.Sprintf("points at %s but no artifact claims it", target)
	}
	return row
}

// foldedResolves reports whether destPath, after resolving every
// symlink along it, lands on the same file as wantTarget — i.e. an
// ancestor symlink (a fold) already routes the destination to the
// canonical artifact even though destPath itself is not a link. A
// plain real file at destPath resolves to itself and can never equal
// a distinct wantTarget, so conflicts are not misclassified.
func foldedResolves(destPath, wantTarget string) bool {
	rd, err := filepath.EvalSymlinks(destPath)
	if err != nil {
		return false
	}
	rw, err := filepath.EvalSymlinks(wantTarget)
	if err != nil {
		return false
	}
	return rd == rw
}

// auditSummary renders the one-line count summary for the full row
// set (forward pass + sweep), keyed and ordered by state.
func auditSummary(rows []StatusEntry) string {
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.State]++
	}
	states := make([]string, 0, len(counts))
	for s := range counts {
		states = append(states, s)
	}
	sort.Strings(states)
	parts := make([]string, 0, len(states))
	for _, s := range states {
		parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
	}
	return "audit: " + strings.Join(parts, ", ")
}
