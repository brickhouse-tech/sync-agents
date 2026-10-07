package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file undoes what deliverChannels created, for `clean` and
// `global clean`: our links, our regions, our config entry, and the
// index files. Everything else is the user's and stays.

// cleanChannels removes what deliverChannels created for run's
// channels and returns how many paths it removed or edited: LinkMount
// symlinks that point at their index file, our region (the host itself
// only when sync may create it and nothing else is left), our config
// entry (the config itself only when nothing else is left of the file
// sync created), the index files, and directories those removals leave
// empty. A real file, a symlink that points elsewhere, every byte
// outside our region, and every other config byte are the user's and
// stay. A project AGENTS.md is never touched.
func (a *App) cleanChannels(run ChannelRun) (int, error) {
	run.Mode = ChannelRefresh
	chans, _, err := a.bindChannels(run)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, ch := range chans {
		switch m := ch.Spec.Mount.(type) {
		case LinkMount:
			fi, err := os.Lstat(ch.Native)
			if err == nil && fi.Mode()&os.ModeSymlink != 0 && linkSatisfied(ch.Native, a.linkSource(ch)) {
				if a.removePath(ch.Native) {
					removed++
					a.pruneEmptyParents(filepath.Dir(ch.Native), a.pruneStop(ch))
				}
			}
		case RegionMount:
			n, err := a.cleanRegion(ch, m)
			if err != nil {
				return removed, err
			}
			removed += n
		case ConfigListMount:
			n, err := a.cleanConfigList(ch, m)
			if err != nil {
				return removed, err
			}
			removed += n
		}
		if _, err := os.Lstat(ch.Index); err == nil && a.removePath(ch.Index) {
			removed++
		}
	}
	if len(chans) > 0 {
		tree := chans[0].Tree
		a.pruneEmptyParents(filepath.Join(tree, "index"), tree)
	}
	return removed, nil
}

// pruneStop is the directory pruning stops at after removing ch's link:
// the project root, or at global scope the tool's home, which belongs
// to the tool and stays even when empty.
func (a *App) pruneStop(ch Channel) string {
	if ch.Scope == ScopeGlobal {
		return ch.Home
	}
	return a.ProjectRoot
}

// cleanRegion strips our region from the host and keeps every other
// byte. A host left holding nothing is removed only when the spec lets
// sync create it (CreateHost): an empty ~/.codex/AGENTS.md carries
// nobody's text. A host another program seeds (OpenClaw's AGENTS.md)
// always stays. The rewrite is a compare-and-swap.
func (a *App) cleanRegion(ch Channel, m RegionMount) (int, error) {
	src, err := os.ReadFile(ch.Native)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	out, found := stripRegion(string(src), m.Region)
	if !found {
		return 0, nil
	}
	if m.CreateHost && strings.TrimSpace(out) == "" {
		if a.removePath(ch.Native) {
			return 1, nil
		}
		return 0, nil
	}
	if a.DryRun {
		fmt.Fprintf(a.Stdout, "  would edit: %s (remove region %s)\n", a.display(ch.Native), m.Region.Name)
		return 1, nil
	}
	if err := writeIfUnchanged(ch.Native, src, []byte(out)); err != nil {
		if errors.Is(err, errConcurrentEdit) {
			a.Warn(a.display(ch.Native) + " changed during clean; left as is (rerun)")
			return 0, nil
		}
		return 0, err
	}
	a.Info(fmt.Sprintf("Removed region %s from %s", m.Region.Name, a.display(ch.Native)))
	return 1, nil
}

// cleanConfigList removes our entry from the config. When the file
// then holds exactly what remains of a config sync created, it was
// ours alone and is removed.
func (a *App) cleanConfigList(ch Channel, m ConfigListMount) (int, error) {
	src, err := os.ReadFile(ch.Native)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	out, changed, err := removeJSONArrayEntry(src, m.Key, a.configEntry(ch))
	if errors.Is(err, errJSONNotEditable) {
		a.Warn(fmt.Sprintf("%s: %v; remove %q from %q by hand", a.display(ch.Native), err, a.configEntry(ch), m.Key))
		return 0, nil
	}
	if err != nil || !changed {
		return 0, err
	}
	created, _, _ := ensureJSONArrayEntry(nil, m.Key, a.configEntry(ch))
	leftover, _, _ := removeJSONArrayEntry(created, m.Key, a.configEntry(ch))
	if bytes.Equal(out, leftover) {
		if a.removePath(ch.Native) {
			return 1, nil
		}
		return 0, nil
	}
	if a.DryRun {
		fmt.Fprintf(a.Stdout, "  would edit: %s (remove %q from %q)\n", a.display(ch.Native), a.configEntry(ch), m.Key)
		return 1, nil
	}
	if err := writeIfUnchanged(ch.Native, src, out); err != nil {
		if errors.Is(err, errConcurrentEdit) {
			a.Warn(a.display(ch.Native) + " changed during clean; left as is (rerun)")
			return 0, nil
		}
		return 0, err
	}
	a.Info(fmt.Sprintf("Removed %q from %q in %s", a.configEntry(ch), m.Key, a.display(ch.Native)))
	return 1, nil
}

// removePath removes one file or symlink (dry-run aware) and reports
// whether it did, or would.
func (a *App) removePath(path string) bool {
	if a.DryRun {
		fmt.Fprintf(a.Stdout, "  would remove: %s\n", a.display(path))
		return true
	}
	if err := os.Remove(path); err != nil {
		a.Warn(fmt.Sprintf("remove %s: %v", a.display(path), err))
		return false
	}
	a.Info("Removed: " + a.display(path))
	return true
}

// pruneEmptyParents removes dir and then each parent that is left empty,
// stopping at stop (never removed). Only empty directories go, so a
// directory holding anything of the user's always stays.
func (a *App) pruneEmptyParents(dir, stop string) {
	if a.DryRun {
		return
	}
	for dir != stop && strings.HasPrefix(dir, stop+string(filepath.Separator)) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
