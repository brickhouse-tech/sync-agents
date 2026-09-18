package agent

import (
	"strings"

	"github.com/brickhouse-tech/sync-agents/internal/agent/templates"
)

// Bucket describes one asset bucket under .agents/. Command loops
// (init, add, sync, status, clean, fix, import, global init/sync)
// iterate the Buckets registry instead of hardcoding directory
// names, so adding a bucket is one entry here plus its routing in
// destination.go (SPEC-004 Part A).
type Bucket struct {
	// Dir is the plural directory name under .agents/ ("rules").
	Dir string

	// Artifact is the canonical singular type ("rule") used in CLI
	// args and log messages. See ArtifactType in promote.go.
	Artifact ArtifactType

	// DirPerArtifact means each artifact is a directory
	// (skills/<name>/SKILL.md) rather than a flat <name>.md file.
	DirPerArtifact bool

	// InInit means `sync-agents init` and `global init` create the
	// bucket directory up front. Buckets without it are created on
	// demand by `add`.
	InInit bool

	// NewTemplate returns the scaffold content used by `add`, with
	// ${NAME} placeholders. Nil means `add` does not support the
	// bucket.
	NewTemplate func() string

	// NewSubdir, when set, is the subdirectory under the bucket dir
	// where `add` scaffolds new artifacts (ADRs start life in
	// adrs/proposed/). Empty means the bucket root.
	NewSubdir string

	// Ext is the artifact file extension for flat buckets. Empty
	// means ".md"; hooks use ".json".
	Ext string

	// Tools restricts which sync targets receive this bucket's
	// artifacts. Empty means every active target (the behavior of the
	// classic three buckets). Names match canonical Tool IDs
	// ("claude", "cursor", "codeium", "copilot", "codex",
	// "opencode").
	//
	// The restriction applies at BOTH scopes (SPEC-011 Part A): local
	// sync consults it before creating the bucket's directory
	// symlink, and TargetDestination consults it before routing an
	// individual artifact into a per-tool global dir. Keeping one
	// field for both is deliberate — a bucket that means nothing to a
	// tool means nothing to it at either scope.
	Tools []string
}

// FileExt returns the bucket's artifact extension (".md" unless the
// bucket overrides it, e.g. hooks → ".json").
func (b Bucket) FileExt() string {
	if b.Ext == "" {
		return ".md"
	}
	return b.Ext
}

// SyncsToTool reports whether this bucket's artifacts belong in the
// given tool's tree, at either scope. An unrestricted bucket (empty
// Tools) syncs everywhere; a restricted one syncs only to the listed
// canonical Tool IDs.
func (b Bucket) SyncsToTool(toolID string) bool {
	if len(b.Tools) == 0 {
		return true
	}
	for _, t := range b.Tools {
		if t == toolID {
			return true
		}
	}
	return false
}

// subagentTools lists the canonical Tool IDs that expose a native
// subagent surface. Declared once so the agents bucket restriction and
// TargetDestination's agent branch cannot drift apart; adding a
// harness with subagents is a one-line change here plus its directory
// mapping in the Tools registry.
var subagentTools = []string{"claude", "cursor", "opencode"}

var Buckets = []Bucket{
	{Dir: "rules", Artifact: ArtifactRule, InInit: true, NewTemplate: templates.Rule},
	{Dir: "skills", Artifact: ArtifactSkill, DirPerArtifact: true, InInit: true, NewTemplate: templates.Skill},
	{Dir: "workflows", Artifact: ArtifactWorkflow, InInit: true, NewTemplate: templates.Workflow},
	// Subagent definitions (SPEC-004 Part B, widened by SPEC-011
	// Part A). Restricted to the tools that have a native subagent
	// surface reading markdown + YAML frontmatter of the same shape:
	// Claude (.claude/agents/), Cursor (.cursor/agents/, since Cursor
	// 2.4), and opencode (.opencode/agents/, ~/.config/opencode/agents/
	// at user scope). Windsurf, Copilot, and Codex have no subagent
	// concept and consume agents only through the AGENTS.md index.
	//
	// Frontmatter is NOT translated between harness dialects — Claude's
	// `tools:`/`model:` and Cursor's `readonly:`/`is_background:` pass
	// through verbatim and each tool ignores what it does not know
	// (SPEC-004 Part E). The bucket is not created by init; it
	// activates when `add agent` (or the user) creates the directory.
	{Dir: "agents", Artifact: ArtifactAgent, NewTemplate: templates.Agent, Tools: subagentTools},
	// Reference-doc buckets (SPEC-004 Part D). plans = per-effort
	// how/when working documents; specs = durable what/why design
	// docs. Same plumbing, different lifecycle. Claude-only local
	// symlinks (@-mentionable); other tools consume them via the
	// AGENTS.md index. Not created by init.
	{Dir: "plans", Artifact: ArtifactPlan, NewTemplate: templates.Plan, Tools: []string{"claude"}},
	{Dir: "specs", Artifact: ArtifactSpec, NewTemplate: templates.Spec, Tools: []string{"claude"}},
	// Claude hook fragments (SPEC-004 Part C). Flat JSON files
	// merged into .claude/settings.json rather than symlinked;
	// routing handled separately in hooks.go. Not created by init.
	{Dir: "hooks", Artifact: ArtifactHook, NewTemplate: templates.Hook, Tools: []string{"claude"}, Ext: ".json"},
	// Architecture Decision Records (SPEC-004 Part F). Status is
	// encoded by subdirectory: proposed/, accepted/, denied/. Only
	// accepted + proposed are indexed in AGENTS.md; denied ADRs are
	// kept (and pointed at from the index) so past rejections aren't
	// re-proposed. `add adr` scaffolds into proposed/; the `adr`
	// command moves records between statuses.
	{Dir: "adrs", Artifact: ArtifactADR, NewTemplate: templates.ADR, Tools: []string{"claude"}, NewSubdir: "proposed"},
}

// BucketDirs returns the directory names of all registered buckets,
// in registry order.
func BucketDirs() []string {
	dirs := make([]string, len(Buckets))
	for i, b := range Buckets {
		dirs[i] = b.Dir
	}
	return dirs
}

// InitBucketDirs returns the directory names of buckets that init
// commands create up front.
func InitBucketDirs() []string {
	var dirs []string
	for _, b := range Buckets {
		if b.InInit {
			dirs = append(dirs, b.Dir)
		}
	}
	return dirs
}

// ArtifactNames returns the canonical singular names ("rule, skill,
// workflow") for usage/error messages, in registry order.
func ArtifactNames() []string {
	names := make([]string, len(Buckets))
	for i, b := range Buckets {
		names[i] = string(b.Artifact)
	}
	return names
}

// BucketForDir looks a bucket up by its directory name ("rules").
func BucketForDir(dir string) (Bucket, bool) {
	for _, b := range Buckets {
		if b.Dir == dir {
			return b, true
		}
	}
	return Bucket{}, false
}

// BucketForArtifact looks a bucket up by its canonical ArtifactType.
func BucketForArtifact(typ ArtifactType) (Bucket, bool) {
	for _, b := range Buckets {
		if b.Artifact == typ {
			return b, true
		}
	}
	return Bucket{}, false
}

// BucketForTypeString resolves user-supplied type strings — singular
// or plural, any case — to their bucket. Returns false for
// unrecognised input; callers should surface that as a "type must be
// one of rule, skill, workflow" error built from ArtifactNames.
func BucketForTypeString(s string) (Bucket, bool) {
	s = strings.ToLower(s)
	for _, b := range Buckets {
		if s == string(b.Artifact) || s == b.Dir {
			return b, true
		}
	}
	return Bucket{}, false
}
