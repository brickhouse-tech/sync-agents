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
}

var Buckets = []Bucket{
	{Dir: "rules", Artifact: ArtifactRule, InInit: true, NewTemplate: templates.Rule},
	{Dir: "skills", Artifact: ArtifactSkill, DirPerArtifact: true, InInit: true, NewTemplate: templates.Skill},
	{Dir: "workflows", Artifact: ArtifactWorkflow, InInit: true, NewTemplate: templates.Workflow},
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
