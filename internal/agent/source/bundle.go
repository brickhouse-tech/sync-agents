package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// bundle.go implements the manifest-maintenance half of SPEC-003:
// source add/remove/bundle/detach. Pull installs what the manifest
// declares; these commands edit the declaration itself — bundle being
// the inverse of pull (recover a manifest from installed artifacts'
// origin metadata).

// Add validates the entry, appends it to sources.yaml (creating the
// file on first use), and immediately pulls it — `source add` is
// "declare and install" in one step (AC-1).
func (p *Puller) Add(ctx context.Context, raw string, opts PullOpts) (PullReport, error) {
	e, err := ParseEntry(raw)
	if err != nil {
		return PullReport{}, err
	}
	m, _, err := LoadManifest(p.AgentsDir)
	if err != nil {
		return PullReport{}, err
	}
	for _, existing := range m.Sources {
		if existing == e.Raw {
			return PullReport{}, fmt.Errorf("entry already declared in %s: %s", ManifestPath(p.AgentsDir), e.Raw)
		}
	}
	if !opts.DryRun {
		m.Sources = append(m.Sources, e.Raw)
		if err := SaveManifest(p.AgentsDir, m); err != nil {
			return PullReport{}, err
		}
	}
	return p.Pull(ctx, PullOpts{
		Force:   opts.Force,
		DryRun:  opts.DryRun,
		Offline: opts.Offline,
		Only:    []string{e.Raw},
	})
}

// Remove drops the entry from sources.yaml and sources.lock and — by
// default — deletes the installed artifact(s) plus origin metadata.
// keep leaves the artifact in place, flipped to source:"manual", so
// it survives as an untracked local file (AC-4).
func (p *Puller) Remove(name string, keep bool) error {
	m, exists, err := LoadManifest(p.AgentsDir)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("no %s found", ManifestPath(p.AgentsDir))
	}

	raw, e, err := findEntry(m, name)
	if err != nil {
		return err
	}

	if e.Type == EntryTree {
		// Tree entries own every installed artifact whose origin
		// points at the same repo — the same trace bundle/list use.
		for _, a := range p.scanInstalled() {
			if a.origin.Owner != e.Owner || a.origin.Repo != e.Repo {
				continue
			}
			if err := p.removeOrDetachArtifact(a.dest, a.dirArtifact, a.origin, keep); err != nil {
				return err
			}
		}
	} else {
		entryName, _ := applyOverrides(e, m)
		dest, dirArtifact := p.artifactDest(e, entryName)
		if o, oerr := ReadOriginFor(dest, dirArtifact); oerr == nil {
			if err := p.removeOrDetachArtifact(dest, dirArtifact, o, keep); err != nil {
				return err
			}
		} else if _, serr := os.Lstat(dest); serr == nil && !keep {
			// Installed but origin-less (user stripped it?): still
			// honor the remove request rather than orphaning it.
			if err := os.RemoveAll(dest); err != nil {
				return err
			}
		}
	}

	m.Sources = removeString(m.Sources, raw)
	if err := SaveManifest(p.AgentsDir, m); err != nil {
		return err
	}
	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		return err
	}
	lock.Remove(raw)
	return SaveLock(p.AgentsDir, lock)
}

func (p *Puller) removeOrDetachArtifact(dest string, dirArtifact bool, o Origin, keep bool) error {
	if keep {
		o.Source = SourceManual
		return WriteOriginFor(dest, dirArtifact, o)
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if !dirArtifact {
		// Flat artifacts keep their origin as a sibling; a dir
		// artifact's origin was inside the dir and is already gone.
		if err := os.Remove(OriginPathFor(dest, false)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Detach severs a pulled artifact from live management while keeping it
// on disk. For a normal source it flips origin to source:"manual" and
// drops the manifest + lock entries. For a linked source (SPEC-007) it
// instead FREEZES the dev copy into a vendored snapshot — see
// detachLink. The returned frozen bool is true in the latter case, so
// callers can report the right outcome.
func (p *Puller) Detach(name string) (frozen bool, err error) {
	m, exists, err := LoadManifest(p.AgentsDir)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, fmt.Errorf("no %s found", ManifestPath(p.AgentsDir))
	}
	raw, e, err := findEntry(m, name)
	if err != nil {
		return false, err
	}

	// SPEC-007: detaching a linked source is "freeze my dev copy" —
	// materialize the symlink target into a real vendored artifact,
	// drop the link override, and record a normal snapshot lock entry.
	// The upstream identity in `sources` stays, so pull governs it as a
	// snapshot from here on.
	if linkRel := linkFor(e, m); linkRel != "" {
		return true, p.detachLink(raw, e, linkRel, m)
	}

	if e.Type == EntryTree {
		for _, a := range p.scanInstalled() {
			if a.origin.Owner == e.Owner && a.origin.Repo == e.Repo {
				a.origin.Source = SourceManual
				if err := WriteOriginFor(a.dest, a.dirArtifact, a.origin); err != nil {
					return false, err
				}
			}
		}
	} else {
		entryName, _ := applyOverrides(e, m)
		dest, dirArtifact := p.artifactDest(e, entryName)
		if o, oerr := ReadOriginFor(dest, dirArtifact); oerr == nil {
			o.Source = SourceManual
			if err := WriteOriginFor(dest, dirArtifact, o); err != nil {
				return false, err
			}
		}
	}

	m.Sources = removeString(m.Sources, raw)
	if err := SaveManifest(p.AgentsDir, m); err != nil {
		return false, err
	}
	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		return false, err
	}
	lock.Remove(raw)
	return false, SaveLock(p.AgentsDir, lock)
}

// detachLink freezes a linked source (SPEC-007): it replaces the
// symlink with a real copy of the current target contents, drops the
// link override (keeping the upstream `sources` entry), writes normal
// origin metadata, and records a snapshot lock entry hashed over the
// materialized files. After this the entry is an ordinary vendored
// artifact that pull governs by SHA.
func (p *Puller) detachLink(raw string, e Entry, linkRel string, m Manifest) error {
	name, _ := applyOverrides(e, m)
	dest, dirArtifact := p.artifactDest(e, name)

	checkoutRoot := filepath.Join(p.AgentsDir, filepath.FromSlash(linkRel))
	target := linkArtifactTarget(checkoutRoot, e)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("cannot detach %s: link target %s is missing (%w) — restore the checkout or `source remove` it", name, target, err)
	}

	// Provenance to freeze into the snapshot: the informational HEAD the
	// lock recorded at link/update time (empty if never resolved).
	sha := ""
	if lock, err := LoadLock(p.AgentsDir); err == nil {
		if le := lock.Find(raw); le != nil {
			sha = le.ResolvedSHA
		}
	}

	// Materialize: real copy replaces the symlink. installArtifact copies
	// the target into a sibling temp first, then swaps — the checkout is
	// never mutated.
	if err := installArtifact(target, dest, dirArtifact); err != nil {
		return err
	}
	contentHash, err := HashTree(dest)
	if err != nil {
		return err
	}
	fetchedAt := p.now().Format(time.RFC3339)
	if err := WriteOriginFor(dest, dirArtifact, Origin{
		Owner: e.Owner, Repo: e.Repo, Path: e.Path, Ref: e.Ref,
		SHA: sha, ContentHash: contentHash, FetchedAt: fetchedAt, Source: SourceManifest,
	}); err != nil {
		return err
	}

	// Drop the link override; keep the sources identity entry.
	m.Overrides = dropLinkOverride(m.Overrides, e)
	if err := SaveManifest(p.AgentsDir, m); err != nil {
		return err
	}
	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		return err
	}
	lock.Set(LockEntry{
		Entry: raw, ResolvedSHA: sha, ContentHash: contentHash, FetchedAt: fetchedAt,
	})
	return SaveLock(p.AgentsDir, lock)
}

// dropLinkOverride clears the link field from an entry's override,
// removing the override entirely when nothing else remains on it.
func dropLinkOverride(overrides []Override, e Entry) []Override {
	out := overrides[:0]
	for _, o := range overrides {
		if o.Matches(e.Raw) && o.Link != "" {
			o.Link = ""
			// An override that was link-only is now empty — drop it so
			// the manifest stays clean.
			if o.Rename == "" && o.PinSHA == "" && len(o.ExcludePaths) == 0 {
				continue
			}
		}
		out = append(out, o)
	}
	return out
}

// BundleReport summarizes what Bundle changed.
type BundleReport struct {
	Added    []string // entries appended to sources.yaml
	Flipped  []string // artifacts whose origin flipped manual → manifest
	Warnings []string // artifacts that could not be expressed as entries
}

// Bundle is the inverse of pull (AC-6): scan the buckets for
// artifacts carrying origin metadata, emit/refresh sources.yaml from
// them, and flip manual origins to manifest so future pulls govern
// them. Artifacts whose bucket has no entry-grammar type (agents,
// plans, specs — installable only via tree:) are warned about and
// skipped rather than guessed into a wrong entry shape.
func (p *Puller) Bundle() (BundleReport, error) {
	var rep BundleReport
	m, _, err := LoadManifest(p.AgentsDir)
	if err != nil {
		return rep, err
	}

	// Index declared entries by owner/repo/path so re-bundling is
	// idempotent — an artifact already governed by the manifest only
	// needs its source flag checked.
	declared := map[string]bool{}
	declaredTrees := map[string]bool{}
	for _, raw := range m.Sources {
		e, err := ParseEntry(raw)
		if err != nil {
			continue
		}
		if e.Type == EntryTree {
			declaredTrees[e.Owner+"/"+e.Repo] = true
			continue
		}
		declared[e.Owner+"/"+e.Repo+"/"+e.Path] = true
	}

	for _, a := range p.scanInstalled() {
		typ, ok := entryTypeForBucket(a.bucket.Dir)
		coveredByTree := declaredTrees[a.origin.Owner+"/"+a.origin.Repo]
		if !ok && !coveredByTree {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"%s: bucket %q has no source entry type — declare a tree: entry for %s/%s to govern it",
				a.dest, a.bucket.Dir, a.origin.Owner, a.origin.Repo))
			continue
		}

		if ok && !coveredByTree && !declared[a.origin.Owner+"/"+a.origin.Repo+"/"+a.origin.Path] {
			entry := formatEntry(typ, a.origin)
			m.Sources = append(m.Sources, entry)
			declared[a.origin.Owner+"/"+a.origin.Repo+"/"+a.origin.Path] = true
			rep.Added = append(rep.Added, entry)
		}
		if a.origin.Source != SourceManifest {
			a.origin.Source = SourceManifest
			if err := WriteOriginFor(a.dest, a.dirArtifact, a.origin); err != nil {
				return rep, err
			}
			rep.Flipped = append(rep.Flipped, a.dest)
		}
	}

	if len(rep.Added) > 0 {
		if err := SaveManifest(p.AgentsDir, m); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

// entryTypeForBucket maps a bucket directory to the entry type that
// can express a single artifact from it. Only the classic three have
// one; everything else arrives via tree: entries.
func entryTypeForBucket(dir string) (EntryType, bool) {
	switch dir {
	case "skills":
		return EntrySkill, true
	case "rules":
		return EntryRule, true
	case "workflows":
		return EntryWorkflow, true
	}
	return "", false
}

// formatEntry reconstructs the entry string for an origin record.
func formatEntry(typ EntryType, o Origin) string {
	var b strings.Builder
	b.WriteString(string(typ))
	b.WriteString(":")
	b.WriteString(o.Owner)
	b.WriteString("/")
	b.WriteString(o.Repo)
	if o.Ref != "" {
		b.WriteString("@")
		b.WriteString(o.Ref)
	}
	if o.Path != "" {
		b.WriteString("/")
		b.WriteString(o.Path)
	}
	return b.String()
}

// findEntry resolves a user-supplied name (artifact name or literal
// entry string) to its manifest entry.
func findEntry(m Manifest, name string) (raw string, e Entry, err error) {
	var matches []string
	for _, candidate := range m.Sources {
		pe, perr := ParseEntry(candidate)
		if perr != nil {
			continue
		}
		if candidate == name || pe.Name() == name {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return "", Entry{}, fmt.Errorf("no source entry named %q in sources.yaml (try the full entry string)", name)
	case 1:
		e, err = ParseEntry(matches[0])
		return matches[0], e, err
	default:
		return "", Entry{}, fmt.Errorf("name %q matches %d entries (%s) — use the full entry string", name, len(matches), strings.Join(matches, ", "))
	}
}

func removeString(list []string, s string) []string {
	out := list[:0]
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
