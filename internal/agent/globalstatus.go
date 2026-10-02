package agent

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// DestinationState classifies the filesystem state of one per-artifact
// destination from a previously-run (or never-run) `global sync`.
//
// The four-state vocabulary maps to SPEC-002's "[synced] / [missing] /
// [not a symlink]" output, plus a [drifted] state for symlinks whose
// target moved out from under them.
type DestinationState string

const (
	// StateSynced: a symlink exists at the destination and points at
	// the canonical artifact under ~/.agents/. No action needed.
	StateSynced DestinationState = "synced"

	// StateDrifted: a symlink exists but its target is different
	// from what a fresh sync would create. Re-running `global sync`
	// will repair it.
	StateDrifted DestinationState = "drifted"

	// StateNotSymlink: a regular file or directory occupies the
	// destination. `global sync` will skip it unless --force is set.
	StateNotSymlink DestinationState = "not-a-symlink"

	// StateMissing: nothing exists at the destination. `global sync`
	// will create the symlink.
	StateMissing DestinationState = "missing"
)

// StatusEntry is one row of `status` or `global status` output. Each
// entry maps to a single (tool, artifact) destination OR a single
// delivery channel.
type StatusEntry struct {
	// Tool is the destination tool ID (claude, codeium, …).
	Tool string

	// ArtifactType is the source bucket; empty for channel rows, where
	// every passive artifact contributes to the same file.
	ArtifactType ArtifactType

	// ArtifactName is the source artifact's name; empty for channel
	// rows.
	ArtifactName string

	// DestinationPath is the absolute path being reported on.
	DestinationPath string

	// State is exactly one of {synced, drifted, not-a-symlink,
	// missing} for symlink destinations, or a ChannelState (plus
	// "skipped", "gap", "error") for channel rows. Stored as string
	// so callers don't have to type-switch when rendering.
	State string

	// IsChannel is true for delivery-channel rows (one per tool), so
	// they print as "tool -> path" rather than "tool/type/name".
	IsChannel bool

	// Detail is an optional human-readable note (e.g. the actual
	// vs expected symlink target for drifted state).
	Detail string
}

// GlobalStatusOpts holds the options for CmdGlobalStatus. The zero
// value is "all registered tools, plain text output."
type GlobalStatusOpts struct {
	// Targets, when non-empty, filters which tools are reported on.
	// Aliases honored via ResolveTool.
	Targets []string
}

// CmdGlobalStatus inspects the user's global per-tool directories
// and reports the state of every destination that `global sync`
// would manage.
//
// The function is read-only: no filesystem writes occur. It walks
// the same artifact set DiscoverArtifacts produces and resolves the
// same destinations TargetDestination would, then stats each one.
//
// Output: one line per (tool, artifact) destination plus one line
// per delivery channel (SPEC-013), from channelRows. Format:
//
//	[STATE] tool/typ/name -> path  (detail)
//	[STATE] tool -> path  (detail)
//
// A channel is one line per tool, not one per contributing artifact,
// and Cursor's missing user-rules file is one "gap" line. Empty global
// root produces a clear error pointing at `global init`.
//
// See SPEC-002 §Requirement: Global status and
// docs/commands/global-status.md for the user-facing contract.
func (a *App) CmdGlobalStatus(opts GlobalStatusOpts) error {
	root := a.ResolveGlobalRoot()
	parent := a.ResolveGlobalRootParent()

	if _, err := os.Stat(root); err != nil {
		a.Error(fmt.Sprintf("global root %s does not exist; run `sync-agents global init` first", root))
		return err
	}

	tools, err := a.resolveSyncTools(opts.Targets)
	if err != nil {
		return err
	}

	artifacts, err := DiscoverArtifacts(root)
	if err != nil {
		return err
	}

	run := ChannelRun{Scope: ScopeGlobal, Explicit: opts.Targets, Tools: tools}
	chans, _, err := a.bindChannels(run)
	if err != nil {
		return err
	}
	entries, expected := computeStatus(artifacts, tools, parent, artifactGates(chans))
	channelRows, err := a.channelRows(run)
	if err != nil {
		return err
	}

	a.Info(fmt.Sprintf("global status (%d artifacts, %d tool(s)):", len(artifacts), len(tools)))

	for _, e := range entries {
		a.printStatusEntry(e)
	}
	for _, e := range channelRows {
		a.printStatusEntry(e)
	}

	// SPEC-010 Phase 1: reverse sweep — classify what occupies the
	// managed subdirs beyond what the .agents/ tree claims. Read-only;
	// foreign entries are reported and never touched.
	sweepRows := sweepUnmanaged(tools, parent, root, expected)
	for _, e := range sweepRows {
		a.printStatusEntry(e)
	}

	all := append(append(append([]StatusEntry{}, entries...), channelRows...), sweepRows...)
	a.Info(auditSummary(all))
	return nil
}

// computeStatus is the per-artifact logic of `global status`: given
// the discovered artifacts, tool set, and the per-tool gates sync
// applies (artifactGates), return the per-destination entries and the
// set of symlink destination paths the .agents/ tree claims (consumed
// by the SPEC-010 reverse sweep to separate managed entries from
// foreign/orphaned ones). Channels are reported separately, by
// channelRows.
//
// Split out from CmdGlobalStatus so tests can exercise the state
// classification without going through the App + filesystem-write
// surface.
func computeStatus(artifacts []Artifact, tools []Tool, parent string, gates artifactGateMap) (perDestination []StatusEntry, expected map[string]bool) {
	expected = map[string]bool{}
	for _, art := range artifacts {
		sem, err := ResolveSemantic(art.SourcePath, art.Type)
		if err != nil {
			// Treat a semantic error as "we don't know how to
			// route this" — surface as a synthetic row.
			perDestination = append(perDestination, StatusEntry{
				ArtifactType:    art.Type,
				ArtifactName:    art.Name,
				DestinationPath: art.SourcePath,
				State:           "frontmatter-error",
				Detail:          err.Error(),
			})
			continue
		}
		for _, tool := range tools {
			gate := gates.of(tool.ID)
			dest := TargetDestination(tool, art.Type, art.Name, sem, art.SourcePath, parent)
			switch dest.Strategy {
			case StrategySkip:
				if !gate.warn {
					continue
				}
				perDestination = append(perDestination, StatusEntry{
					Tool:         tool.ID,
					ArtifactType: art.Type,
					ArtifactName: art.Name,
					State:        "skipped",
					Detail:       dest.SkipReason,
				})
			case StrategySymlink:
				if !gate.link {
					continue
				}
				expected[dest.Path] = true
				state, detail := classifySymlinkDestination(dest.Path, symlinkTarget(art))
				perDestination = append(perDestination, StatusEntry{
					Tool:            tool.ID,
					ArtifactType:    art.Type,
					ArtifactName:    art.Name,
					DestinationPath: dest.Path,
					State:           string(state),
					Detail:          detail,
				})
			}
		}
	}

	// Sort the per-destination rows for deterministic output.
	sort.Slice(perDestination, func(i, j int) bool {
		if perDestination[i].Tool != perDestination[j].Tool {
			return perDestination[i].Tool < perDestination[j].Tool
		}
		if perDestination[i].ArtifactType != perDestination[j].ArtifactType {
			return perDestination[i].ArtifactType < perDestination[j].ArtifactType
		}
		return perDestination[i].ArtifactName < perDestination[j].ArtifactName
	})
	return perDestination, expected
}

// classifySymlinkDestination inspects a symlink destination and
// returns its state.
func classifySymlinkDestination(destPath, wantTarget string) (DestinationState, string) {
	info, err := os.Lstat(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			return StateMissing, ""
		}
		return StateNotSymlink, err.Error()
	}
	if info.Mode()&os.ModeSymlink == 0 {
		// Not a symlink itself — but Lstat follows intermediate
		// components, so the path may be reached through a folded
		// ancestor symlink (SPEC-010): .claude/skills/<name> ->
		// .agents/skills/<name> makes <name>/SKILL.md resolve to the
		// canonical file with no per-file link. That's conformant,
		// not a conflict.
		if foldedResolves(destPath, wantTarget) {
			return DestinationState(StateFolded), "resolves via ancestor symlink"
		}
		return StateNotSymlink, ""
	}
	current, err := os.Readlink(destPath)
	if err != nil {
		return StateDrifted, "readlink failed: " + err.Error()
	}
	if current != wantTarget {
		return StateDrifted, fmt.Sprintf("points at %s, want %s", current, wantTarget)
	}
	return StateSynced, ""
}

// printStatusEntry formats one StatusEntry as a single text line. The
// format is intentionally regex-friendly: a state bracketed at the
// start, then the tool/type/name (just the tool for a channel row),
// then `->` and the path when there is one, optionally a parenthetical
// detail.
func (a *App) printStatusEntry(e StatusEntry) {
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(e.State)
	sb.WriteString("] ")

	if e.IsChannel {
		sb.WriteString(e.Tool)
		if e.DestinationPath != "" {
			sb.WriteString(" -> ")
			sb.WriteString(e.DestinationPath)
		}
	} else {
		if e.Tool != "" {
			sb.WriteString(e.Tool)
			sb.WriteString("/")
		}
		if e.ArtifactType != "" {
			sb.WriteString(string(e.ArtifactType))
			sb.WriteString("/")
		}
		sb.WriteString(e.ArtifactName)
		if e.DestinationPath != "" {
			sb.WriteString(" -> ")
			sb.WriteString(e.DestinationPath)
		}
	}

	if e.Detail != "" {
		sb.WriteString("  (")
		sb.WriteString(e.Detail)
		sb.WriteString(")")
	}
	fmt.Fprintln(a.Stdout, sb.String())
}
