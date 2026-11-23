package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DestinationStrategy is the kind of filesystem operation `global
// sync` performs for a given (tool, artifact, semantic) tuple.
//
// Most tool/artifact pairs resolve to StrategySymlink — a per-artifact
// symlink at a per-tool path. Passive content for a tool that reads
// aggregated rule text resolves to StrategyChannel and is delivered by
// the channel layer (SPEC-013). StrategySkip is for cases that can't be
// cleanly represented in a tool's filesystem layout.
type DestinationStrategy int

const (
	// StrategySymlink: create an absolute symlink at the resolved path
	// pointing at the canonical artifact under ~/.agents/.
	StrategySymlink DestinationStrategy = iota

	// StrategySkip: the artifact cannot be routed to this tool.
	// SkipReason explains why (multi-file invocable skill targeting
	// Windsurf, etc.). The sync loop logs the reason and continues
	// to other tools.
	StrategySkip

	// StrategyChannel: the artifact is passive content for a tool that
	// has a global row in channelSpecs (or a gap in channelGaps). The
	// per-artifact loop does nothing with it: deliverChannels renders
	// every passive artifact into the tool's channel once per run, and
	// global status reports the channel (or the gap) as one row.
	StrategyChannel
)

// Destination is the resolved location for one (tool, artifact,
// semantic) tuple. Strategy determines how the orchestration layer
// handles it; Path is the absolute symlink path.
type Destination struct {
	// Strategy is how to write this destination — see
	// DestinationStrategy.
	Strategy DestinationStrategy

	// Path is the absolute symlink to create (or repair). Empty unless
	// Strategy == StrategySymlink.
	Path string

	// SkipReason is set only when Strategy == StrategySkip. Used by
	// the orchestration layer to print a per-skip warning.
	SkipReason string
}

// TargetDestination resolves one (tool, artifact) pair to its
// Destination, given the artifact's already-resolved Semantic and
// the location of the canonical artifact under ~/.agents/.
//
// Parameters:
//
//   - tool: the destination tool (from the Tools registry).
//   - typ: artifact bucket type (rule / skill / workflow).
//   - name: artifact name (without extension or directory).
//   - sem: resolved semantic — caller has already inspected frontmatter
//     and bucket defaults via ResolveSemantic.
//   - artifactSourcePath: absolute path to the artifact under
//     ~/.agents/ (the file for rules/workflows; the directory for
//     skills). Used to detect multi-file skills, which Windsurf can't
//     accept as workflows.
//   - globalRootParent: parent dir of the resolved global root (used
//     to compute per-tool dirs like ~/.claude/).
//
// Returns a Destination that the orchestration layer can act on. This
// function is pure — no filesystem writes — so it's safe for
// dry-run plans and for tests that don't want to populate every
// per-tool dir.
//
// See SPEC-002 §Semantic-aware routing and
// docs/architecture/semantic-routing.md for the routing rules this
// implements.
func TargetDestination(
	tool Tool,
	typ ArtifactType,
	name string,
	sem Semantic,
	artifactSourcePath string,
	globalRootParent string,
) Destination {
	// Agents (subagent definitions) route independently of semantic,
	// into each tool's native subagent directory. Routing them through
	// the per-tool semantic tables would mislabel them as commands
	// (Claude), workflows (Windsurf), or channel content (every tool
	// with a channel) — an agent body inlined into an always-on
	// instructions file is the worst of those outcomes.
	//
	// Which tools qualify is the agents bucket's Tools restriction
	// (SPEC-011 Part A), so this branch and local sync can never
	// disagree about who has a subagent surface. The directory name is
	// the bucket's own ("agents"), which all three qualifying tools
	// happen to share; a harness that named it differently would need
	// a per-tool override here.
	if typ == ArtifactAgent {
		bucket, _ := BucketForArtifact(typ)
		if bucket.SyncsToTool(tool.ID) {
			toolDir := tool.DirForScope(ScopeGlobal, globalRootParent)
			if toolDir == "" {
				return Destination{
					Strategy:   StrategySkip,
					SkipReason: fmt.Sprintf("%s has no global scope", tool.ID),
				}
			}
			return Destination{
				Strategy: StrategySymlink,
				Path:     filepath.Join(toolDir, bucket.Dir, name+".md"),
			}
		}
		if why, ok := agentSkips[tool.ID]; ok {
			return Destination{Strategy: StrategySkip, SkipReason: why}
		}
		return Destination{
			Strategy:   StrategySkip,
			SkipReason: fmt.Sprintf("%s has no subagent surface", tool.ID),
		}
	}

	// Reference docs (plans/specs, SPEC-004 Part D) also route
	// independently of semantic: symlinked under .claude/ so they're
	// @-mentionable, skipped everywhere else — never inlined into
	// always-on instructions (that would preload reference material
	// into baseline context).
	if typ == ArtifactPlan || typ == ArtifactSpec {
		bucket, _ := BucketForArtifact(typ)
		if tool.ID == "claude" {
			return Destination{
				Strategy: StrategySymlink,
				Path:     filepath.Join(globalRootParent, ".claude", bucket.Dir, name+".md"),
			}
		}
		return Destination{
			Strategy:   StrategySkip,
			SkipReason: fmt.Sprintf("%s reference docs are Claude-only; other tools open them from ~/.agents/%s/ when asked", bucket.Dir, bucket.Dir),
		}
	}

	// Hooks (SPEC-004 Part C) are never synced one-by-one — they
	// are collected and merged into .claude/settings.json in one
	// batch pass after the per-artifact loop. The per-artifact
	// routing always skips; the batch pass handles everything.
	if typ == ArtifactHook {
		return Destination{
			Strategy:   StrategySkip,
			SkipReason: "hooks are batch-merged into settings.json (see MergeHooks)",
		}
	}

	// Passive content for a tool that reads aggregated rule text is
	// the channel layer's (SPEC-013 §Per-tool delivery): delivered by
	// deliverChannels, or reported once by global status when the tool
	// has no file to deliver to (channelGaps).
	if sem == Passive && hasGlobalChannel(tool.ID) {
		return Destination{Strategy: StrategyChannel}
	}

	if b, ok := BucketForArtifact(typ); ok && slices.Contains(shallowBuckets[tool.ID], b.Dir) {
		return Destination{
			Strategy: StrategySymlink,
			Path:     filepath.Join(tool.DirForScope(ScopeGlobal, globalRootParent), b.Dir, flatName(name)+".md"),
		}
	}

	switch tool.ID {
	case "claude":
		return claudeLayoutDestination(tool.ID, typ, name, sem, tool.DirForScope(ScopeGlobal, globalRootParent))
	case "codeium":
		return codeiumDestination(typ, name, sem, artifactSourcePath, globalRootParent)
	case "antigravity":
		return antigravityDestination(typ, name, sem, tool.DirForScope(ScopeGlobal, globalRootParent))
	}
	if _, known := ResolveTool(tool.ID); !known {
		return Destination{
			Strategy:   StrategySkip,
			SkipReason: fmt.Sprintf("unknown tool %q", tool.ID),
		}
	}
	return invocableSkip(tool.ID, typ, name)
}

// hasGlobalChannel reports whether toolID's passive content is handled
// by the channel layer at global scope: a channelSpecs row, or a
// channelGaps entry explaining why there is none.
func hasGlobalChannel(toolID string) bool {
	if _, ok := channelSpecs[toolID][ScopeGlobal]; ok {
		return true
	}
	_, ok := channelGaps[toolID][ScopeGlobal]
	return ok
}

// skillsNative lists the tools that load ~/.agents/skills themselves
// (SPEC-013 §Per-tool delivery), so a link into their own tree would
// register each skill twice.
var skillsNative = map[string]string{
	"cline":    "Cline",
	"codex":    "Codex",
	"cursor":   "Cursor",
	"devin":    "Devin",
	"gemini":   "Gemini CLI",
	"opencode": "opencode",
	"openclaw": "OpenClaw",
}

// agentSkips gives the skip reason for an agent sent to a tool whose
// subagent surface sync-agents does not write, where the generic "no
// subagent surface" would be wrong.
var agentSkips = map[string]string{
	"cline": "Cline's agents/ format is undocumented",
}

// commandSkips gives the skip reason for an invocable rule or workflow
// sent to a tool that has a better place for it than the generic
// "no command surface".
var commandSkips = map[string]string{
	"devin":       "Devin has no workflow surface; make it a skill",
	"antigravity": "Antigravity workflows are deprecated; make it a skill",
	"gemini":      "TOML commands are not generated",
}

// invocableSkip is the Skip for an invocable artifact sent to a tool
// with no user-scope command or skill surface that sync-agents writes:
// every tool except Claude. Passive content never reaches
// here (StrategyChannel); agents and reference docs are routed earlier.
func invocableSkip(toolID string, typ ArtifactType, name string) Destination {
	if typ == ArtifactSkill {
		if brand, ok := skillsNative[toolID]; ok {
			return Destination{Strategy: StrategySkip, SkipReason: brand + " loads ~/.agents/skills natively"}
		}
		return Destination{Strategy: StrategySkip, SkipReason: fmt.Sprintf("%s has no user-scope skill surface", toolID)}
	}
	if why, ok := commandSkips[toolID]; ok {
		return Destination{Strategy: StrategySkip, SkipReason: why}
	}
	return Destination{
		Strategy:   StrategySkip,
		SkipReason: fmt.Sprintf("%s has no user-scope command surface; invocable %s %q is not delivered", toolID, typ, name),
	}
}

// claudeLayoutDestination implements the Claude routing row under
// home, the tool's global dir:
//
//   - Invocable skill (dir): <home>/skills/<name>/SKILL.md
//   - Invocable rule/workflow (single file): <home>/commands/<name>.md
//   - Passive (any bucket): <home>/rules/<name>.md (single file)
//
// The "passive multi-file" case (a skill explicitly marked
// invocable: false) is awkward — the passive surface is a single
// rules/*.md file, not a directory. We currently treat it as
// StrategySkip with a warning, leaving the artifact untouched in this
// tool. A future revision could flatten the skill's SKILL.md into
// rules/. Tracking that as a non-blocking decision.
func claudeLayoutDestination(toolID string, typ ArtifactType, name string, sem Semantic, home string) Destination {
	switch sem {
	case Invocable:
		if typ == ArtifactSkill {
			return Destination{
				Strategy: StrategySymlink,
				Path:     filepath.Join(home, "skills", name, "SKILL.md"),
			}
		}
		// Single-file invocable (rule or workflow marked
		// invocable: true) — lands in the commands/ surface.
		return Destination{
			Strategy: StrategySymlink,
			Path:     filepath.Join(home, "commands", name+".md"),
		}
	case Passive:
		if typ == ArtifactSkill {
			// Multi-file artifact (skill dir) explicitly marked
			// passive. No clean single-file destination — skip and
			// warn so the user can decide whether to split the
			// skill or accept the gap.
			return Destination{
				Strategy:   StrategySkip,
				SkipReason: fmt.Sprintf("passive skill %q has no single-file %s destination", name, toolID),
			}
		}
		return Destination{
			Strategy: StrategySymlink,
			Path:     filepath.Join(home, "rules", name+".md"),
		}
	}
	return Destination{Strategy: StrategySkip, SkipReason: "unknown semantic"}
}

// antigravityDestination implements the Antigravity row (SPEC-015
// A2-A5) under home, ~/.gemini/config:
//
//   - Skill: skills/<name>/SKILL.md (the App does not read
//     ~/.agents/skills)
//   - Passive rule or workflow: rules/<flat name>.md
//   - Invocable single-file: skipped (commandSkips)
//
// Antigravity scans only the top level of rules/, so an OS-scoped rule
// ("macos/brew") is linked as "macos-brew.md".
func antigravityDestination(typ ArtifactType, name string, sem Semantic, home string) Destination {
	switch {
	case typ == ArtifactSkill:
		return Destination{Strategy: StrategySymlink, Path: filepath.Join(home, "skills", name, "SKILL.md")}
	case sem == Passive:
		return Destination{Strategy: StrategySymlink, Path: filepath.Join(home, "rules", strings.ReplaceAll(name, "/", "-")+".md")}
	}
	return invocableSkip("antigravity", typ, name)
}

// codeiumDestination implements the Codeium/Windsurf row:
//
//   - Invocable single-file: ~/.codeium/windsurf/global_workflows/<name>.md
//   - Invocable multi-file skill: SKIP (Windsurf workflows are single
//     .md files; a skill dir with supporting files can't be a
//     workflow without flattening)
//
// Passive content never reaches here: it is the codeium-rules region
// of global_rules.md (StrategyChannel).
//
// Multi-file detection: a skill is "multi-file" if its source dir
// contains anything besides SKILL.md. SkillIsMultiFile inspects the
// directory; if the inspection fails we fall back to treating the
// skill as single-file (safer to attempt the workflow placement and
// let the symlink layer either succeed or fail loudly).
func codeiumDestination(typ ArtifactType, name string, sem Semantic, artifactSourcePath, parent string) Destination {
	windsurfBase := filepath.Join(parent, ".codeium", "windsurf")

	switch sem {
	case Invocable:
		if typ == ArtifactSkill && SkillIsMultiFile(artifactSourcePath) {
			return Destination{
				Strategy:   StrategySkip,
				SkipReason: fmt.Sprintf("Windsurf workflows are single-file; skill %q has supporting files", name),
			}
		}
		// Single-file invocable: workflow file, single-file skill,
		// or rule marked invocable: true.
		invocableSource := name + ".md"
		if typ == ArtifactSkill {
			// A single-file skill is still a directory containing
			// only SKILL.md. The workflow file gets the skill name
			// as its identifier; the symlink points at SKILL.md
			// inside the source dir.
			invocableSource = name + ".md"
		}
		return Destination{
			Strategy: StrategySymlink,
			Path:     filepath.Join(windsurfBase, "global_workflows", invocableSource),
		}
	}
	return Destination{Strategy: StrategySkip, SkipReason: "unknown semantic"}
}

// SkillIsMultiFile reports whether the skill directory at the given
// path contains any files besides SKILL.md (or subdirectories of
// any kind).
//
// A skill is considered single-file if SKILL.md is the only entry
// inside its directory. Anything else — supporting Python scripts,
// example files, sub-directories — makes it multi-file.
//
// Returns false (single-file) when:
//
//   - The path doesn't exist or isn't a directory. We can't say
//     definitively, but treating it as single-file lets the symlink
//     layer either succeed (if SKILL.md is actually a single file at
//     that path) or fail with a clearer error than a fabricated
//     warning here would produce.
//   - The directory contains only SKILL.md.
//
// Returns true otherwise.
func SkillIsMultiFile(skillDir string) bool {
	fi, err := os.Stat(skillDir)
	if err != nil || !fi.IsDir() {
		return false
	}
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != "SKILL.md" {
			return true
		}
	}
	return false
}
