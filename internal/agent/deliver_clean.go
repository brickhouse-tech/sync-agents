package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file undoes what deliverChannels created, for `clean`: our
// links, our config entry, and the index files. Everything else is the
// user's and stays.

// cleanChannels removes what deliverChannels created and returns how
// many paths it removed or edited: LinkMount symlinks that point at
// their index file, our config entry (the config itself only when
// nothing else is left of the file sync created), the index files, and
// directories those removals leave empty. A real file, a symlink that
// points elsewhere, and every other config byte are the user's and
// stay. AGENTS.md is never touched.
func (a *App) cleanChannels(scope Scope, explicit []string) (int, error) {
	chans, _, err := a.bindChannels(ChannelRun{Scope: scope, Mode: ChannelRefresh, Explicit: explicit})
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
					a.pruneEmptyParents(filepath.Dir(ch.Native))
				}
			}
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
	a.pruneEmptyParents(filepath.Join(a.ProjectRoot, ".agents", "index"))
	return removed, nil
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
// stopping at the project root. Only empty directories go, so a
// directory holding anything of the user's always stays.
func (a *App) pruneEmptyParents(dir string) {
	if a.DryRun {
		return
	}
	for dir != a.ProjectRoot && strings.HasPrefix(dir, a.ProjectRoot+string(filepath.Separator)) {
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
