package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// origin.go implements SPEC-003 §Per-artifact origin metadata.
//
// Every pulled artifact carries provenance next to it — inside the
// directory for dir-shaped artifacts (skills), as a sibling for flat
// files (rules/workflows) — so that a human reading the tree, or a
// downstream tool (promote, the AGENTS.md indexer), can see exactly
// where a file came from without consulting the lock. Origin files
// are meant to be committed; they are the provenance record that
// survives a git clone.

const (
	// originDirFileName is the metadata file placed INSIDE a
	// directory artifact (.agents/skills/<name>/_origin.json). The
	// leading underscore sorts it to the top of listings and keeps it
	// visually distinct from artifact content.
	originDirFileName = "_origin.json"

	// originFileSuffix is the suffix for a flat artifact's sibling
	// metadata (.agents/rules/security.origin.json for security.md).
	originFileSuffix = ".origin.json"
)

// Origin source values. "manifest" = governed by sources.yaml;
// "manual" = imported or hand-copied, not managed by pull/update.
const (
	SourceManifest = "manifest"
	SourceManual   = "manual"
)

// Origin is the fixed _origin.json schema from SPEC-003. Field order
// here controls emission order (encoding/json preserves struct
// order), which keeps diffs stable across rewrites.
type Origin struct {
	Schema      int    `json:"schema"`
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Ref         string `json:"ref"`
	SHA         string `json:"sha"`
	ContentHash string `json:"content_hash"`
	FetchedAt   string `json:"fetched_at"`
	Source      string `json:"source"`
}

// OriginPathFor returns where the origin metadata for the artifact at
// dest lives: inside dest for directory artifacts, as a sibling
// (<stem>.origin.json) for flat files.
func OriginPathFor(dest string, dirArtifact bool) string {
	if dirArtifact {
		return filepath.Join(dest, originDirFileName)
	}
	ext := filepath.Ext(dest)
	return strings.TrimSuffix(dest, ext) + originFileSuffix
}

// ReadOriginFor loads the origin metadata for the artifact at dest.
// A missing file returns os.ErrNotExist (callers branch on it for
// the "destination exists without origin" conflict case).
func ReadOriginFor(dest string, dirArtifact bool) (Origin, error) {
	return ReadOrigin(OriginPathFor(dest, dirArtifact))
}

// ReadOrigin loads and validates one origin file.
func ReadOrigin(path string) (Origin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Origin{}, err
	}
	var o Origin
	if err := json.Unmarshal(data, &o); err != nil {
		return Origin{}, fmt.Errorf("%s: invalid origin metadata: %w", path, err)
	}
	if o.Schema != 1 {
		return Origin{}, fmt.Errorf("%s: unsupported origin schema %d (this build understands 1)", path, o.Schema)
	}
	return o, nil
}

// WriteOriginFor writes the artifact's origin metadata atomically.
// Emission is deterministic (MarshalIndent, fixed field order,
// trailing newline) so re-pulling at the same SHA produces
// byte-identical files and no spurious git diffs.
func WriteOriginFor(dest string, dirArtifact bool, o Origin) error {
	o.Schema = 1
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(OriginPathFor(dest, dirArtifact), append(data, '\n'), 0o644)
}

// writeFileAtomic writes via temp file + rename in the destination's
// directory, mirroring internal/agent's helper (duplicated rather
// than exported from the agent package to keep this package's
// dependency direction: agent → source, never the reverse).
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".source-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
