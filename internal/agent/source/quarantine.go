package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// quarantine.go implements SPEC-005 Part B: remotely-fetched artifacts
// land in .agents/.quarantine/ instead of the live tree, carrying a
// pending record (provenance + lock data + scan findings). A human
// promotes them with `approve` or discards them with `reject`. The
// dot-directory is invisible to sync and index by construction — both
// only walk registered bucket dirs.

// QuarantineDirName lives under the .agents/ root. Dot-prefixed so no
// bucket loop, glob, or index sweep ever picks it up.
const QuarantineDirName = ".quarantine"

// PendingArtifact is the sidecar record written next to a quarantined
// artifact. It carries everything approve needs to finish the install
// exactly as a direct pull would have: the origin file, the lock
// entry, and the findings a human is being asked to judge.
type PendingArtifact struct {
	// Entry is the manifest entry string that produced this artifact.
	// Tree entries quarantine several artifacts sharing one Entry;
	// the lock is applied when the LAST of them is approved.
	Entry string `json:"entry"`

	// Name is the artifact name used by approve/reject matching.
	Name string `json:"name"`

	// DestRel is the artifact's destination relative to the .agents/
	// root ("rules/security.md", "skills/grep-helper"). The same
	// relative path locates the quarantined copy under .quarantine/.
	DestRel string `json:"dest_rel"`

	DirArtifact bool `json:"dir_artifact"`

	Origin Origin `json:"origin"`

	// Lock is the entry-level lock record to apply on final approve.
	Lock LockEntry `json:"lock"`

	Findings []Finding `json:"findings,omitempty"`
}

func (p *Puller) quarantineRoot() string {
	return filepath.Join(p.AgentsDir, QuarantineDirName)
}

// pendingPath is the sidecar location for a quarantined DestRel.
func (p *Puller) pendingPath(destRel string) string {
	return filepath.Join(p.quarantineRoot(), filepath.FromSlash(destRel)+".pending.json")
}

// quarantineStaged moves a verified staged artifact into quarantine
// and writes its pending record. Called instead of installArtifact
// when the quarantine gate is on.
func (p *Puller) quarantineStaged(src string, pa PendingArtifact) error {
	qdest := filepath.Join(p.quarantineRoot(), filepath.FromSlash(pa.DestRel))
	if err := os.MkdirAll(filepath.Dir(qdest), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(qdest); err != nil {
		return err
	}
	if err := installArtifact(src, qdest, pa.DirArtifact); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pa, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p.pendingPath(pa.DestRel), append(data, '\n'), 0o644)
}

// ListPending returns every quarantined artifact's record, sorted by
// destination path for stable output.
func (p *Puller) ListPending() ([]PendingArtifact, error) {
	var out []PendingArtifact
	root := p.quarantineRoot()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".pending.json") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		var pa PendingArtifact
		if json.Unmarshal(data, &pa) == nil && pa.DestRel != "" {
			out = append(out, pa)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DestRel < out[j].DestRel })
	return out, nil
}

// matchPending selects pending artifacts by name (or all).
func (p *Puller) matchPending(name string, all bool) ([]PendingArtifact, error) {
	pendings, err := p.ListPending()
	if err != nil {
		return nil, err
	}
	if all {
		if len(pendings) == 0 {
			return nil, fmt.Errorf("quarantine is empty")
		}
		return pendings, nil
	}
	var matches []PendingArtifact
	for _, pa := range pendings {
		if pa.Name == name {
			matches = append(matches, pa)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no quarantined artifact named %q (see `sync-agents quarantine`)", name)
	}
	return matches, nil
}

// Approve promotes quarantined artifacts into the live tree. Critical
// findings block unless force; forced approvals are recorded in the
// lock entry so the decision is auditable later.
func (p *Puller) Approve(name string, all, force bool) ([]PendingArtifact, error) {
	matches, err := p.matchPending(name, all)
	if err != nil {
		return nil, err
	}

	for _, pa := range matches {
		if HasCritical(pa.Findings) && !force {
			return nil, fmt.Errorf("%s has CRITICAL findings — review them (`sync-agents quarantine`), then `approve %s --force` to accept anyway", pa.DestRel, pa.Name)
		}
	}

	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		return nil, err
	}
	for _, pa := range matches {
		qsrc := filepath.Join(p.quarantineRoot(), filepath.FromSlash(pa.DestRel))
		dest := filepath.Join(p.AgentsDir, filepath.FromSlash(pa.DestRel))
		if _, err := os.Stat(dest); err == nil && !force {
			return nil, fmt.Errorf("%s already exists in the live tree — resolve it (or approve --force to replace)", pa.DestRel)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := os.RemoveAll(dest); err != nil {
			return nil, err
		}
		if err := os.Rename(qsrc, dest); err != nil {
			return nil, err
		}
		if err := WriteOriginFor(dest, pa.DirArtifact, pa.Origin); err != nil {
			return nil, err
		}
		os.Remove(p.pendingPath(pa.DestRel))
		p.pruneQuarantineDirs()

		// Apply the entry-level lock only when the whole entry is
		// out of quarantine — a tree entry approved halfway must not
		// pretend to be current.
		remaining, err := p.ListPending()
		if err != nil {
			return nil, err
		}
		entryDone := true
		for _, other := range remaining {
			if other.Entry == pa.Entry {
				entryDone = false
				break
			}
		}
		if entryDone {
			le := pa.Lock
			if force && HasCritical(pa.Findings) {
				le.ApprovedWithFindings = true
			}
			lock.Set(le)
		}
	}
	if err := SaveLock(p.AgentsDir, lock); err != nil {
		return nil, err
	}
	return matches, nil
}

// Reject deletes quarantined artifacts and their records. The
// manifest keeps its entry — the user either edits sources.yaml or
// re-pulls (returning the artifact to quarantine) deliberately.
func (p *Puller) Reject(name string, all bool) ([]PendingArtifact, error) {
	matches, err := p.matchPending(name, all)
	if err != nil {
		return nil, err
	}
	for _, pa := range matches {
		os.RemoveAll(filepath.Join(p.quarantineRoot(), filepath.FromSlash(pa.DestRel)))
		os.Remove(p.pendingPath(pa.DestRel))
	}
	p.pruneQuarantineDirs()
	return matches, nil
}

// pruneQuarantineDirs removes empty directories (and finally the
// quarantine root itself) so an emptied quarantine leaves no residue.
func (p *Puller) pruneQuarantineDirs() {
	root := p.quarantineRoot()
	var dirs []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	// Deepest first.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		os.Remove(d) // fails silently unless empty — exactly what we want
	}
}
