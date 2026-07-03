package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ArtifactType is the bucket an artifact belongs to. The string values
// are the canonical, lowercase, singular names used in CLI args and
// log messages. NormalizeArtifactType converts plurals and casing to
// these values.
type ArtifactType string

const (
	ArtifactRule     ArtifactType = "rule"
	ArtifactSkill    ArtifactType = "skill"
	ArtifactWorkflow ArtifactType = "workflow"
	ArtifactAgent    ArtifactType = "agent"
	ArtifactPlan     ArtifactType = "plan"
	ArtifactSpec     ArtifactType = "spec"
)

// NormalizeArtifactType maps user-supplied type strings to their
// canonical ArtifactType. Accepts singular, plural, and any case.
// Returns "" and false for unrecognised input; callers should surface
// that as a "type must be one of rule, skill, workflow" error.
//
// We accept plurals because the directories under .agents/ are
// plural (rules/, skills/, workflows/), so users often type the
// plural by reflex. We accept any case because the CLI flag value is
// a low-stakes string that's not worth bouncing on capitalization.
func NormalizeArtifactType(s string) (ArtifactType, bool) {
	b, ok := BucketForTypeString(s)
	if !ok {
		return "", false
	}
	return b.Artifact, true
}

// PromoteOpts holds the per-call options for CmdPromote. New fields
// should default to the zero-value-is-safe behavior so existing
// callers don't break when the struct grows.
type PromoteOpts struct {
	// Force, when true, allows the destination to be overwritten
	// if it already exists. Default false produces a fail-fast
	// behavior with a clear message.
	Force bool

	// DryRun, when true, prints the plan but performs no filesystem
	// writes. Idempotent regardless of the source/destination state.
	DryRun bool
}

// artifactRelPath returns the project-relative path to an artifact
// of the given type and name. The path is the *canonical* form under
// .agents/, identical at local and global scope:
//
//	rule     foo  → .agents/rules/foo.md
//	skill    foo  → .agents/skills/foo            (directory)
//	workflow foo  → .agents/workflows/foo.md
//
// Returns the empty string for an unrecognised type — callers should
// have validated via NormalizeArtifactType first.
func artifactRelPath(typ ArtifactType, name string) string {
	b, ok := BucketForArtifact(typ)
	if !ok {
		return ""
	}
	if b.DirPerArtifact {
		// Dir-per-artifact buckets (skills): the SKILL.md inside is
		// the metadata file but we copy the whole dir.
		return filepath.Join(".agents", b.Dir, name)
	}
	return filepath.Join(".agents", b.Dir, name+".md")
}

// DetectArtifact maps a project-relative path under .agents/ to the
// (type, name) pair that addresses it. Used to implement the
// path-form `promote <path>` invocation: `sync-agents promote
// .agents/skills/foo` is sugar for `sync-agents promote skill foo`.
//
// Detection rules (per SPEC-002 §Promote command, invocation form 2):
//
//  1. Path is under `.agents/skills/` → skill; name is the next path
//     segment (so .agents/skills/foo and .agents/skills/foo/SKILL.md
//     both resolve to skill foo).
//  2. Path is under `.agents/rules/` and ends in `.md` → rule; name
//     is the basename without `.md`.
//  3. Path is under `.agents/workflows/` and ends in `.md` →
//     workflow; name is the basename without `.md`.
//  4. Anything else → error.
//
// The input is treated as a forward-slash-or-OS-separator-tolerant
// relative path. Absolute paths and paths with leading ./ are
// normalized first.
func DetectArtifact(rel string) (ArtifactType, string, error) {
	// Normalize: strip leading "./", collapse separators. We don't
	// resolve symlinks or abs paths — the caller is expected to pass
	// a path relative to the project root.
	rel = filepath.Clean(rel)
	rel = strings.TrimPrefix(rel, "./")

	// filepath.Clean uses the OS separator. Normalize for the
	// prefix-match against forward-slash prefixes so this works on
	// both POSIX and Windows.
	slashed := filepath.ToSlash(rel)

	for _, b := range Buckets {
		prefix := ".agents/" + b.Dir + "/"
		if !strings.HasPrefix(slashed, prefix) {
			continue
		}
		if b.DirPerArtifact {
			after := strings.TrimPrefix(slashed, prefix)
			if after == "" {
				return "", "", fmt.Errorf("path %q points at the %s directory itself; specify a %s name", rel, b.Dir, b.Artifact)
			}
			// First path segment is the artifact name.
			name := strings.SplitN(after, "/", 2)[0]
			return b.Artifact, name, nil
		}
		if strings.HasSuffix(slashed, ".md") {
			name := strings.TrimSuffix(filepath.Base(slashed), ".md")
			return b.Artifact, name, nil
		}
	}

	return "", "", fmt.Errorf("path %q is not under .agents/{%s}/; pass `promote <type> <name>` explicitly", rel, strings.Join(BucketDirs(), ","))
}

// CmdPromote copies an artifact from the local project's .agents/
// tree to the user's global .agents/ tree.
//
// Behavior (per SPEC-002 §Requirement: Promote command):
//
//   - Copies content; does not symlink. The local copy is the source
//     of the promotion, not an ongoing reference. After promote, the
//     two trees are independent files. Future edits should happen at
//     whichever scope is authoritative for the artifact (typically
//     global, since promote is the act of saying "this is good
//     enough to apply everywhere").
//   - Skills are directories — the entire tree is deep-copied.
//   - Rules and workflows are files.
//   - If the destination exists and Force is false, the call fails
//     with a clear message and no filesystem change. With Force=true,
//     the destination is replaced.
//   - DryRun=true causes the planned operations to print and exits
//     without filesystem writes.
//   - Missing source returns an error. The destination directory's
//     parents are auto-created so a promote into a fresh ~/.agents/
//     works.
//
// The global root location is resolved via App.ResolveGlobalRoot
// (env / field / $HOME precedence).
//
// CmdPromote does NOT regenerate the global AGENTS.md or run global
// sync. Those responsibilities belong to subsequent commands (and
// AC-10 explicitly attaches index regen to `global sync`).
//
// See docs/commands/promote.md for user-facing documentation.
func (a *App) CmdPromote(typ ArtifactType, name string, opts PromoteOpts) error {
	if name == "" {
		return errors.New("promote: name is required")
	}
	rel := artifactRelPath(typ, name)
	if rel == "" {
		return fmt.Errorf("promote: unknown artifact type %q", typ)
	}

	src := filepath.Join(a.ProjectRoot, rel)
	dst := filepath.Join(a.ResolveGlobalRoot(), strings.TrimPrefix(rel, ".agents"+string(filepath.Separator)))

	// Verify source exists. The error message uses the relative
	// path because that's what the user typed (or auto-detected
	// from).
	srcInfo, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			a.Error(fmt.Sprintf("%s %q not found in %s", typ, name, rel))
			return err
		}
		return err
	}

	// Conflict check. The check is "destination exists at all" —
	// content-equality is not considered because the operation is
	// inherently destructive when the user passes --force, and the
	// not-forced branch should err on the side of caution.
	dstExists := false
	if _, err := os.Stat(dst); err == nil {
		dstExists = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if dstExists && !opts.Force {
		a.Warn(fmt.Sprintf("%s already exists; pass --force to overwrite", dst))
		return fmt.Errorf("destination exists: %s", dst)
	}

	if opts.DryRun {
		a.Info(fmt.Sprintf("[dry-run] would copy %s -> %s", src, dst))
		return nil
	}

	// Ensure parent dir of destination exists. MkdirAll is
	// idempotent and creates missing intermediate dirs (so a
	// promote into a fresh ~/.agents/ works without a separate
	// `global init`).
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	// Force overwrite: remove the existing destination first. For a
	// directory this is a recursive delete; the existence check above
	// already gated this behind --force.
	if dstExists && opts.Force {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}

	if srcInfo.IsDir() {
		if err := copyDir(src, dst); err != nil {
			return err
		}
	} else {
		if err := copyFile(src, dst, srcInfo.Mode()); err != nil {
			return err
		}
	}

	a.Info(fmt.Sprintf("Promoted %s %s to %s", typ, name, dst))
	return nil
}

// copyFile copies a single file from src to dst, creating dst with
// the given mode. The destination must not already exist (callers
// remove it first under --force). Returns the first error
// encountered; partial writes may leave a truncated dst on error.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// copyDir recursively copies the directory tree rooted at src into
// dst. dst must not already exist (the caller handled overwrite
// semantics). Symlinks within the source tree are skipped — they're
// a confusing concept inside an artifact that will itself be referenced
// via symlink at the per-tool fan-out stage, and SPEC-002 does not
// specify their handling. If we encounter one we log a warning and
// continue.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())

		// Resolve actual file type (DirEntry.Type only gives the
		// mode bits, not the lstat-derived info we want for
		// symlinks).
		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			// Skip symlinks. Promote is a copy operation; following
			// the link would leak content from outside the artifact,
			// and reproducing the link would create a hanging
			// reference inside the global tree.
			continue
		case info.IsDir():
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		default:
			if err := copyFile(srcPath, dstPath, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}
