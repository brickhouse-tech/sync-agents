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

	// LocalTools restricts which local sync targets receive this
	// bucket's directory symlink. Empty means every active target
	// (the behavior of the classic three buckets). Names match the
	// legacy AllTargets IDs ("claude", "windsurf", "cursor",
	// "copilot").
	LocalTools []string
}

// SyncsToLocalTarget reports whether local sync should link this
// bucket into the given target's directory.
func (b Bucket) SyncsToLocalTarget(target string) bool {
	if len(b.LocalTools) == 0 {
		return true
	}
	for _, t := range b.LocalTools {
		if t == target {
			return true
		}
	}
	return false
}

var Buckets = []Bucket{
	{Dir: "rules", Artifact: ArtifactRule, InInit: true, NewTemplate: templates.Rule},
	{Dir: "skills", Artifact: ArtifactSkill, DirPerArtifact: true, InInit: true, NewTemplate: templates.Skill},
	{Dir: "workflows", Artifact: ArtifactWorkflow, InInit: true, NewTemplate: templates.Workflow},
	// Subagent definitions (SPEC-004 Part B). Claude-only: no other
	// registered tool has a subagent surface, and the bucket is not
	// created by init — it activates when `add agent` (or the user)
	// creates the directory.
	{Dir: "agents", Artifact: ArtifactAgent, NewTemplate: templates.Agent, LocalTools: []string{"claude"}},
	// Reference-doc buckets (SPEC-004 Part D). plans = per-effort
	// how/when working documents; specs = durable what/why design
	// docs. Same plumbing, different lifecycle. Claude-only local
	// symlinks (@-mentionable); other tools consume them via the
	// AGENTS.md index. Not created by init.
	{Dir: "plans", Artifact: ArtifactPlan, NewTemplate: templates.Plan, LocalTools: []string{"claude"}},
	{Dir: "specs", Artifact: ArtifactSpec, NewTemplate: templates.Spec, LocalTools: []string{"claude"}},
	// Claude hook fragments (SPEC-004 Part C). Flat JSON files
	// merged into .claude/settings.json rather than symlinked;
	// routing handled separately in hooks.go. Not created by init.
	{Dir: "hooks", Artifact: ArtifactHook, NewTemplate: templates.Hook, LocalTools: []string{"claude"}},
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
