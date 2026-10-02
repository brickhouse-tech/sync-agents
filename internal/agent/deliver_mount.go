package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// This file places and observes native mounts for deliver.go: the
// LinkMount symlink, the RegionMount splice, and the ConfigListMount
// entry, plus the shadowing downgrade every mount kind shares (SPEC-013
// §Mounts and consent).

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// linkSource is the symlink text from a LinkMount to its index file.
// At project scope it is relative ("../../.agents/index/cursor.mdc"),
// so the link survives a moved checkout. At global scope it is absolute,
// as every global link is (SPEC-002): a tool home such as ~/.copilot is
// often itself a symlink into a dotfiles repo, where a relative path
// would resolve from the wrong directory.
func (a *App) linkSource(ch Channel) string {
	if ch.Scope == ScopeGlobal {
		return ch.Index
	}
	rel, err := filepath.Rel(filepath.Dir(ch.Native), ch.Index)
	if err != nil {
		return ch.Index
	}
	return filepath.ToSlash(rel)
}

// configEntry is the value a ConfigListMount lists. At project scope it
// is the index file relative to the project root, slash-separated
// (".agents/index/opencode.md"), so a committed opencode.json works in
// every clone. At global scope it is the absolute index path, because
// ~/.config/opencode/opencode.json is read from every working directory.
func (a *App) configEntry(ch Channel) string {
	if ch.Scope == ScopeGlobal {
		return ch.Index
	}
	rel, err := filepath.Rel(a.ProjectRoot, ch.Index)
	if err != nil {
		return ch.Index
	}
	return filepath.ToSlash(rel)
}

// display is path relative to the project root for messages.
func (a *App) display(path string) string {
	if rel, err := filepath.Rel(a.ProjectRoot, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}

// mount places or repairs ch's native mount for r and reports the
// state. In ChannelRefresh mode it only observes: refresh creates
// nothing outside .agents/index/, and a link or config entry that
// carries ours needs no write because it points at (or lists) the index
// file that was just rewritten.
//
// gone lists the legacy paths this run removed. Under --dry-run they
// are still on disk, and a link that would be placed through one of
// them is reported as the real run would place it.
func (a *App) mount(ch Channel, r Rendered, mode ChannelMode, gone []string) (ChannelState, string, error) {
	if mode == ChannelRefresh {
		return a.observe(ch, r)
	}
	var (
		state  ChannelState
		detail string
		err    error
	)
	switch m := ch.Spec.Mount.(type) {
	case LinkMount:
		state, detail, err = a.mountLink(ch, gone)
	case ConfigListMount:
		state, detail, err = a.mountConfigList(ch, m)
	case RegionMount:
		state, detail, err = a.mountRegion(ch, m, r)
	}
	if err != nil {
		return "", "", err
	}
	return a.shadowed(ch, state, detail)
}

// observe classifies ch's native mount without writing anything. r is
// the current render, which a region is compared against.
func (a *App) observe(ch Channel, r Rendered) (ChannelState, string, error) {
	var (
		state  ChannelState
		detail string
		err    error
	)
	switch m := ch.Spec.Mount.(type) {
	case LinkMount:
		state, detail, err = a.observeLink(ch)
	case ConfigListMount:
		state, detail, err = a.observeConfigList(ch, m)
	case RegionMount:
		state, detail, err = a.observeRegion(ch, m, r)
	}
	if err != nil {
		return "", "", err
	}
	return a.shadowed(ch, state, detail)
}

// shadowed downgrades a synced channel whose tool reads another file
// first (ChannelSpec.ShadowedBy).
func (a *App) shadowed(ch Channel, state ChannelState, detail string) (ChannelState, string, error) {
	if state != ChannelSynced || ch.Spec.ShadowedBy == nil {
		return state, detail, nil
	}
	tc := ToolContext{Home: ch.Home, Parent: a.ResolveGlobalRootParent(), Env: a.ToolEnv}
	if by := ch.Spec.ShadowedBy(tc); by != "" {
		return ChannelShadowed, fmt.Sprintf("%s reads %s instead", ch.Tool, by), nil
	}
	return state, detail, nil
}

// unmountedWhy is the detail for a mount that is not in place.
func (a *App) unmountedWhy(ch Channel) string {
	if !ch.Mountable {
		return ch.Why
	}
	return "not placed yet; run " + syncCommand(ch.Scope)
}

// syncCommand is the command that delivers a channel at scope.
func syncCommand(scope Scope) string {
	if scope == ScopeGlobal {
		return "`sync-agents global sync`"
	}
	return "`sync-agents sync`"
}

// linkDetour returns a symlinked directory between the project root and
// a project-scope link path, or "". Global links are absolute
// (linkSource), so a symlinked tool home is fine there.
func (a *App) linkDetour(ch Channel) string {
	if ch.Scope == ScopeGlobal {
		return ""
	}
	return symlinkedParent(a.ProjectRoot, ch.Native)
}

func (a *App) observeLink(ch Channel) (ChannelState, string, error) {
	if at := a.linkDetour(ch); at != "" {
		return ChannelConflict, fmt.Sprintf("%s is a symlink, so the link would land outside this project; move it aside", a.display(at)), nil
	}
	fi, err := os.Lstat(ch.Native)
	switch {
	case os.IsNotExist(err):
		return ChannelUnmounted, a.unmountedWhy(ch), nil
	case err != nil:
		return "", "", err
	case fi.Mode()&os.ModeSymlink != 0 && linkSatisfied(ch.Native, a.linkSource(ch)):
		return ChannelSynced, "", nil
	case fi.Mode()&os.ModeSymlink != 0:
		existing, _ := os.Readlink(ch.Native)
		return ChannelUnmounted, fmt.Sprintf("points at %s; sync relinks it", existing), nil
	default:
		return ChannelConflict, fmt.Sprintf("%s is a real %s; move it aside or resync with --overwrite", a.display(ch.Native), realKind(ch.Native)), nil
	}
}

func (a *App) mountLink(ch Channel, gone []string) (ChannelState, string, error) {
	if at := a.linkDetour(ch); at != "" && !slices.Contains(gone, at) {
		return a.observeLink(ch)
	}
	if !ch.Mountable {
		return ChannelUnmounted, ch.Why, nil
	}
	_, err := a.placeLink(a.linkSource(ch), ch.Native, a.DryRun)
	if errors.Is(err, ErrConflict) {
		return a.observeLink(ch)
	}
	if err != nil {
		return "", "", err
	}
	return ChannelSynced, "", nil
}

// symlinkedParent returns the first directory between root and path's
// parent that is a symlink, or "". A LinkMount placed through such a
// directory would land wherever it points: through the pre-SPEC-013
// `.cursor/rules -> ../.agents/rules` fold, for example, inside
// .agents/rules itself.
func symlinkedParent(root, path string) string {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == "." {
		return ""
	}
	dir := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		fi, err := os.Lstat(dir)
		if err != nil {
			return ""
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return dir
		}
	}
	return ""
}

// manualEntry is the ChannelManual detail: the exact entry to add.
func (a *App) manualEntry(ch Channel, m ConfigListMount, err error) string {
	return fmt.Sprintf("%v; add %q to %q in %s by hand", err, a.configEntry(ch), m.Key, a.display(ch.Native))
}

func (a *App) observeConfigList(ch Channel, m ConfigListMount) (ChannelState, string, error) {
	src, err := os.ReadFile(ch.Native)
	if os.IsNotExist(err) {
		return ChannelUnmounted, a.unmountedWhy(ch), nil
	}
	if err != nil {
		return "", "", err
	}
	_, changed, err := ensureJSONArrayEntry(src, m.Key, a.configEntry(ch))
	switch {
	case errors.Is(err, errJSONNotEditable):
		return ChannelManual, a.manualEntry(ch, m, err), nil
	case err != nil:
		return "", "", err
	case changed:
		return ChannelUnmounted, a.unmountedWhy(ch), nil
	}
	return ChannelSynced, "", nil
}

// mountConfigList adds our entry to the config list. An absent config
// is created holding only the entry; an existing one is edited by a
// byte-range insert only with consent (Mountable), through
// writeIfUnchanged so a concurrent edit is never overwritten.
func (a *App) mountConfigList(ch Channel, m ConfigListMount) (ChannelState, string, error) {
	src, err := os.ReadFile(ch.Native)
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	exists := err == nil
	out, changed, err := ensureJSONArrayEntry(src, m.Key, a.configEntry(ch))
	switch {
	case errors.Is(err, errJSONNotEditable):
		return ChannelManual, a.manualEntry(ch, m, err), nil
	case err != nil:
		return "", "", err
	case !changed:
		return ChannelSynced, "", nil
	case !ch.Mountable:
		return ChannelUnmounted, ch.Why, nil
	}
	verb := "Added"
	if !exists {
		verb = "Created"
	}
	if a.DryRun {
		fmt.Fprintf(a.Stdout, "  would edit: %s (add %q to %q)\n", a.display(ch.Native), a.configEntry(ch), m.Key)
		return ChannelSynced, "", nil
	}
	if err := writeIfUnchanged(ch.Native, src, out); err != nil {
		if errors.Is(err, errConcurrentEdit) {
			return ChannelUnmounted, a.display(ch.Native) + " changed during sync; left as is (rerun to add the entry)", nil
		}
		return "", "", err
	}
	a.Info(fmt.Sprintf("%s %s with %q in %q", verb, a.display(ch.Native), a.configEntry(ch), m.Key))
	return ChannelSynced, "", nil
}

// regionHost returns the host file's bytes with r's region holding body.
// A host that is missing, or still the pre-SPEC-013 whole-file concat
// (wholly ours), becomes the region alone; any other host keeps every
// byte outside the markers (spliceRegion appends the region when the
// markers are absent). render calls it with a nil body to measure what
// the tool loads besides our content.
func regionHost(host []byte, r ManagedRegion, body []byte) string {
	block := regionBlock(r, body)
	if isLegacyConcat(host) {
		return block
	}
	return spliceRegion(string(host), r, block)
}

// mountRegion splices r's region into the host file. A missing host is
// created (directories included) only when the spec allows it and the
// channel is mountable; an existing host is edited only when mountable
// (our markers or consent). The write is a compare-and-swap, so an edit
// by the user or the tool's UI during sync is never overwritten; the
// next run retries.
func (a *App) mountRegion(ch Channel, m RegionMount, r Rendered) (ChannelState, string, error) {
	cur, err := os.ReadFile(ch.Native)
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	exists := err == nil
	if !ch.Mountable {
		return ChannelUnmounted, ch.Why, nil
	}
	next := regionHost(cur, m.Region, r.Bytes)
	if exists && string(cur) == next {
		return ChannelSynced, "", nil
	}
	verb := "Updated region " + m.Region.Name + " in"
	switch {
	case !exists:
		verb = "Created"
	case isLegacyConcat(cur):
		verb = "Rewrote the old generated file as region " + m.Region.Name + " in"
	case !strings.Contains(string(cur), m.Region.Start()):
		verb = "Added region " + m.Region.Name + " to"
	}
	if a.DryRun {
		fmt.Fprintf(a.Stdout, "  would edit: %s (region %s)\n", a.display(ch.Native), m.Region.Name)
		return ChannelSynced, "", nil
	}
	if !exists {
		if err := os.MkdirAll(filepath.Dir(ch.Native), 0o755); err != nil {
			return "", "", err
		}
	}
	if err := writeIfUnchanged(ch.Native, cur, []byte(next)); err != nil {
		if errors.Is(err, errConcurrentEdit) {
			return ChannelUnmounted, a.display(ch.Native) + " changed during sync; left as is (rerun to update the region)", nil
		}
		return "", "", err
	}
	a.Info(fmt.Sprintf("%s %s", verb, a.display(ch.Native)))
	return ChannelSynced, "", nil
}

// observeRegion compares the region in the host with what mountRegion
// would write.
func (a *App) observeRegion(ch Channel, m RegionMount, r Rendered) (ChannelState, string, error) {
	cur, err := os.ReadFile(ch.Native)
	switch {
	case os.IsNotExist(err):
		return ChannelUnmounted, a.unmountedWhy(ch), nil
	case err != nil:
		return "", "", err
	case isLegacyConcat(cur):
		return ChannelUnmounted, fmt.Sprintf("still the old generated file; %s rewrites it as region %s", syncCommand(ch.Scope), m.Region.Name), nil
	}
	if _, _, found := m.Region.locate(string(cur)); !found {
		return ChannelUnmounted, a.unmountedWhy(ch), nil
	}
	if regionHost(cur, m.Region, r.Bytes) != string(cur) {
		return ChannelStale, fmt.Sprintf("region %s is out of date; run %s", m.Region.Name, syncCommand(ch.Scope)), nil
	}
	return ChannelSynced, "", nil
}
