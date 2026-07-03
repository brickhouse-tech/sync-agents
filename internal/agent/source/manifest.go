package source

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// manifest.go implements SPEC-003 §Manifest schema: parse/write of
// .agents/sources.yaml (the human-edited declaration) and
// .agents/sources.lock (the machine-written resolution record).
//
// Both files are YAML — the manifest because nested overrides are
// painful to hand-write in JSON, the lock for consistency with the
// file the user edits next to it. Writes are atomic and field order
// is fixed by struct order so regeneration produces reviewable diffs.

const (
	ManifestFileName = "sources.yaml"
	LockFileName     = "sources.lock"
)

// Manifest is the parsed .agents/sources.yaml.
type Manifest struct {
	Version int `yaml:"version"`
	// Sources holds the literal entry strings. Kept as strings (not
	// parsed Entries) because the file is user-owned: preserving the
	// exact text the user wrote is what keeps lock-file keys and
	// git diffs stable.
	Sources   []string   `yaml:"sources"`
	Overrides []Override `yaml:"overrides,omitempty"`
}

// Override is a per-entry tweak matched by glob against the entry
// string. v1 applies rename and pin_sha; exclude_paths is parsed and
// preserved but not yet applied (TODO: filter excluded paths during
// staging — accepted-but-inert so manifests written for a future
// version still parse today).
type Override struct {
	Match        string   `yaml:"match"`
	Rename       string   `yaml:"rename,omitempty"`
	PinSHA       string   `yaml:"pin_sha,omitempty"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty"`
}

// Matches reports whether the override's glob pattern matches the
// entry string. '*' matches any run of characters INCLUDING '/'
// (unlike path.Match) because entry strings are not paths — the
// spec's example pattern "skill:anthropic/skill-pack@*/skills/…"
// needs '*' to span a version segment.
func (o Override) Matches(entry string) bool {
	parts := strings.Split(o.Match, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	re, err := regexp.Compile("^" + strings.Join(parts, ".*") + "$")
	if err != nil {
		return false
	}
	return re.MatchString(entry)
}

// Lock is the parsed .agents/sources.lock.
type Lock struct {
	Version int         `yaml:"version"`
	Entries []LockEntry `yaml:"entries"`
}

// LockEntry records what one manifest entry resolved to at last pull.
type LockEntry struct {
	Entry       string `yaml:"entry"`
	ResolvedSHA string `yaml:"resolved_sha"`
	ContentHash string `yaml:"content_hash"`
	FetchedAt   string `yaml:"fetched_at"`
}

// Find returns the lock entry for the given manifest entry string,
// or nil when the entry has never been pulled.
func (l *Lock) Find(entry string) *LockEntry {
	for i := range l.Entries {
		if l.Entries[i].Entry == entry {
			return &l.Entries[i]
		}
	}
	return nil
}

// Set inserts or replaces the lock record for an entry.
func (l *Lock) Set(e LockEntry) {
	if existing := l.Find(e.Entry); existing != nil {
		*existing = e
		return
	}
	l.Entries = append(l.Entries, e)
}

// Remove drops the lock record for an entry (no-op when absent).
func (l *Lock) Remove(entry string) {
	for i := range l.Entries {
		if l.Entries[i].Entry == entry {
			l.Entries = append(l.Entries[:i], l.Entries[i+1:]...)
			return
		}
	}
}

// ManifestPath returns .agents/sources.yaml for the given agents dir.
func ManifestPath(agentsDir string) string { return filepath.Join(agentsDir, ManifestFileName) }

// LockPath returns .agents/sources.lock for the given agents dir.
func LockPath(agentsDir string) string { return filepath.Join(agentsDir, LockFileName) }

// LoadManifest reads sources.yaml. A missing file is not an error —
// it returns an empty version-1 manifest with exists=false so callers
// can distinguish "nothing declared yet" (source add creates the
// file) from "declared but empty".
func LoadManifest(agentsDir string) (Manifest, bool, error) {
	data, err := os.ReadFile(ManifestPath(agentsDir))
	if os.IsNotExist(err) {
		return Manifest{Version: 1}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, true, fmt.Errorf("%s: %w", ManifestPath(agentsDir), err)
	}
	if m.Version == 0 {
		m.Version = 1
	}
	if m.Version != 1 {
		return Manifest{}, true, fmt.Errorf("%s: unsupported manifest version %d (this build understands 1)", ManifestPath(agentsDir), m.Version)
	}
	return m, true, nil
}

// SaveManifest writes sources.yaml atomically.
func SaveManifest(agentsDir string, m Manifest) error {
	m.Version = 1
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return writeFileAtomic(ManifestPath(agentsDir), data, 0o644)
}

// LoadLock reads sources.lock; missing file yields an empty lock
// (first pull creates it).
func LoadLock(agentsDir string) (Lock, error) {
	data, err := os.ReadFile(LockPath(agentsDir))
	if os.IsNotExist(err) {
		return Lock{Version: 1}, nil
	}
	if err != nil {
		return Lock{}, err
	}
	var l Lock
	if err := yaml.Unmarshal(data, &l); err != nil {
		return Lock{}, fmt.Errorf("%s: %w", LockPath(agentsDir), err)
	}
	if l.Version == 0 {
		l.Version = 1
	}
	return l, nil
}

// SaveLock writes sources.lock atomically.
func SaveLock(agentsDir string, l Lock) error {
	l.Version = 1
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	return writeFileAtomic(LockPath(agentsDir), data, 0o644)
}

// applyOverrides resolves the effective artifact name and SHA pin for
// an entry after consulting the manifest's overrides. Later overrides
// win over earlier ones (last-writer-wins keeps the mental model
// simple: the file reads top-to-bottom).
func applyOverrides(e Entry, m Manifest) (name, pinSHA string) {
	name = e.Name()
	for _, o := range m.Overrides {
		if !o.Matches(e.Raw) {
			continue
		}
		if o.Rename != "" {
			name = o.Rename
		}
		if o.PinSHA != "" {
			pinSHA = o.PinSHA
		}
	}
	return name, pinSHA
}
