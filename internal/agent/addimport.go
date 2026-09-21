package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AddOpts carries the optional import behavior for CmdAdd
// (SPEC-011 Part C). The zero value means "scaffold from the bucket
// template", which is `add`'s original and still-default behavior.
type AddOpts struct {
	// From is a reference to an existing artifact to seed the new one
	// from, instead of the bucket template. Today this is a
	// filesystem path; "~" is expanded and relative paths resolve
	// against the process working directory.
	From string

	// Link makes the canonical path a symlink to the From artifact
	// rather than a copy. The source keeps ownership of its content:
	// edits there are live everywhere the artifact syncs, and
	// sync-agents never rewrites it.
	Link bool
}

// resolveAddSource turns an --from reference into an absolute path to
// an existing file or directory.
//
// A reference containing ':' before any path separator looks like the
// "<source>:<path>" form. That form is deliberately NOT supported:
// registered sources already stage their artifacts directly into the
// bucket tree via `source pull`, and a second import path would
// duplicate that while bypassing the quarantine gate that makes
// pulling third-party content safe (SPEC-005). The error points at
// the supported route rather than failing with "no such file".
func (a *App) resolveAddSource(ref string) (string, error) {
	// A single character before the colon is a Windows drive letter
	// (`C:\foo`), not a source name — require at least two so
	// `<source>:<path>` still matches while drive-lettered absolute
	// paths fall through to normal path handling.
	if i := strings.IndexByte(ref, ':'); i > 1 && !strings.ContainsAny(ref[:i], `/\`) {
		return "", fmt.Errorf(
			"%q looks like a source reference; registered sources deliver artifacts through `sync-agents source add %s` + `sync-agents pull`, which applies the quarantine gate that --from cannot",
			ref, ref[:i])
	}

	path := ref
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand %q: %w", ref, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", ref, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("--from %s: %w", ref, err)
	}
	return abs, nil
}

// normalizeImportedFrontmatter rewrites an imported artifact's
// frontmatter so it is addressable under the name it was imported as,
// and rejects content that would sync into a broken state.
//
// Two rules, both narrow on purpose:
//
//   - `name:` is set to the artifact's canonical name. An artifact's
//     name is its path in the tree; a stale `name:` from wherever the
//     file came from would make it un-delegatable under the name the
//     user just chose. A rewrite is reported, never silent.
//   - `description:` must be non-empty for agents. Every harness's
//     delegation logic keys on it — a subagent without one is
//     installed but unreachable, which is worse than a failed import
//     because nothing surfaces the problem later.
//
// Every other key survives byte-for-byte, including harness-specific
// ones this project does not model (Claude's `tools:`/`model:`,
// Cursor's `readonly:`/`is_background:`). Frontmatter is NOT
// translated between dialects — see the agents bucket comment.
func (a *App) normalizeImportedFrontmatter(content, name string, typ ArtifactType, srcLabel string) (string, error) {
	block, err := parseFMBlock(content)
	if err != nil {
		return "", fmt.Errorf("%s: %w", srcLabel, err)
	}
	if !block.present {
		return "", fmt.Errorf("%s: no YAML frontmatter; a %s needs at least name: and description:", srcLabel, typ)
	}

	if typ == ArtifactAgent {
		if desc, _ := block.get("description"); strings.TrimSpace(desc) == "" {
			return "", fmt.Errorf("%s: frontmatter has no description:; every harness keys subagent delegation on it", srcLabel)
		}
	}

	if existing, _ := block.get("name"); existing != name {
		if existing != "" {
			a.Warn(fmt.Sprintf("%s: rewriting name: %q -> %q to match the imported artifact's name", srcLabel, existing, name))
		}
		block.set("name", name)
	}
	return block.render(), nil
}

// validateLinkedFrontmatter applies the same frontmatter invariants as
// normalizeImportedFrontmatter, but read-only: link mode does not own
// the source and cannot rewrite it, so any discrepancy that copy mode
// would silently fix becomes a hard error here. An artifact linked in
// violation of these rules is silently unreachable in at least one
// harness (agents key delegation on name+description), which is worse
// than refusing the import.
func validateLinkedFrontmatter(content, name string, typ ArtifactType, srcLabel string) error {
	block, err := parseFMBlock(content)
	if err != nil {
		return fmt.Errorf("%s: %w", srcLabel, err)
	}
	if !block.present {
		return fmt.Errorf("%s: no YAML frontmatter; a %s needs at least name: and description:", srcLabel, typ)
	}
	if typ == ArtifactAgent {
		if desc, _ := block.get("description"); strings.TrimSpace(desc) == "" {
			return fmt.Errorf("%s: frontmatter has no description:; every harness keys subagent delegation on it, and --link cannot add one — re-run without --link to import a normalized copy", srcLabel)
		}
	}
	if existing, _ := block.get("name"); existing != "" && existing != name {
		return fmt.Errorf(
			"%s declares name: %q but is being added as %q; --link cannot rewrite the source — re-run without --link to import a normalized copy, or add it as %q",
			srcLabel, existing, name, existing)
	}
	return nil
}

// importByCopy reads the source artifact, normalizes its frontmatter,
// and writes it to the canonical path. The source is never modified.
func (a *App) importByCopy(srcPath, destPath, name string, bucket Bucket) error {
	fi, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	// Directory-per-artifact buckets accept either the artifact
	// directory or the SKILL.md inside it. A directory import copies
	// the supporting files too, since they are part of the artifact.
	if fi.IsDir() {
		if !bucket.DirPerArtifact {
			return fmt.Errorf("--from %s is a directory, but a %s is a single file", srcPath, bucket.Artifact)
		}
		return a.importDirByCopy(srcPath, filepath.Dir(destPath), name, bucket)
	}

	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	out := string(raw)
	if bucket.FileExt() == ".md" {
		if out, err = a.normalizeImportedFrontmatter(out, name, bucket.Artifact, srcPath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, []byte(out), 0o644)
}

// importDirByCopy copies a whole artifact directory, normalizing only
// its entrypoint file. Supporting files are copied verbatim — they are
// the artifact's payload, not its metadata.
func (a *App) importDirByCopy(srcDir, destDir, name string, bucket Bucket) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(destDir, e.Name())
		switch {
		case e.IsDir():
			if err := a.importDirByCopy(src, dst, name, Bucket{}); err != nil {
				return err
			}
		case e.Name() == "SKILL.md" && bucket.DirPerArtifact:
			raw, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			out, err := a.normalizeImportedFrontmatter(string(raw), name, bucket.Artifact, src)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, []byte(out), 0o644); err != nil {
				return err
			}
		default:
			if err := copyFileContents(src, dst); err != nil {
				return err
			}
		}
	}
	return nil
}

// importByLink points the canonical path at the source instead of
// copying it.
//
// Normalization is skipped because the file is not ours to rewrite —
// which makes a `name:` mismatch a hard error rather than a warning.
// Copy mode can fix that discrepancy; link mode can only propagate
// it, and an agent whose frontmatter name disagrees with its path is
// silently unreachable in at least one harness.
//
// The link is relative when the source sits under the project root so
// the repo stays portable across clones, and absolute otherwise
// (the common case being a personas directory in $HOME).
func (a *App) importByLink(srcPath, destPath, name string, bucket Bucket) error {
	agentsDir, err := filepath.Abs(filepath.Join(a.ProjectRoot, ".agents"))
	if err != nil {
		return err
	}
	if srcPath == agentsDir || strings.HasPrefix(srcPath, agentsDir+string(filepath.Separator)) {
		return fmt.Errorf("--from %s is already inside .agents/; linking it to itself would create a cycle", srcPath)
	}

	fi, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	// For a dir-per-artifact bucket, the artifact IS the directory, so
	// link at the directory level rather than at its entrypoint file.
	linkAt := destPath
	if fi.IsDir() {
		if !bucket.DirPerArtifact {
			return fmt.Errorf("--from %s is a directory, but a %s is a single file", srcPath, bucket.Artifact)
		}
		linkAt = filepath.Dir(destPath)

		// The artifact is the directory, but its identity lives in the
		// SKILL.md entrypoint. Copy mode normalizes that file; link
		// mode must reject a mismatch the same way it does for a
		// single-file artifact.
		entry := filepath.Join(srcPath, "SKILL.md")
		raw, err := os.ReadFile(entry)
		if err != nil {
			return fmt.Errorf("--from %s: %w", srcPath, err)
		}
		if err := validateLinkedFrontmatter(string(raw), name, bucket.Artifact, entry); err != nil {
			return err
		}
	}

	if bucket.FileExt() == ".md" && !fi.IsDir() {
		raw, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := validateLinkedFrontmatter(string(raw), name, bucket.Artifact, srcPath); err != nil {
			return err
		}
	}

	target := srcPath
	projectRoot, err := filepath.Abs(a.ProjectRoot)
	if err == nil && strings.HasPrefix(srcPath, projectRoot+string(filepath.Separator)) {
		if rel, rerr := filepath.Rel(filepath.Dir(linkAt), srcPath); rerr == nil {
			target = rel
		}
	}

	if err := os.MkdirAll(filepath.Dir(linkAt), 0o755); err != nil {
		return err
	}
	if a.Force {
		if err := os.RemoveAll(linkAt); err != nil {
			return err
		}
	}
	return os.Symlink(target, linkAt)
}

// copyFileContents copies one regular file, preserving its mode.
func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
