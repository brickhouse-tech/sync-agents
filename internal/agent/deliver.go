package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// This file is the shell around channel.go (SPEC-013 §Shape): it binds
// the channel specs to this run's paths, observes the filesystem,
// writes .agents/index/<tool>.<ext>, and places or reports each tool's
// native mount. Commands see four entry points: deliverChannels (sync,
// fix, index, add, ...), channelRows (status), cleanChannels (clean),
// and gitignoreEntries (sync's .gitignore lines). Every one of them
// starts from the same bindChannels result, so no path is derived
// twice and status, clean, and .gitignore agree with sync by
// construction.
//
// Only project scope is wired here. Global scope (U6 of SPEC-013)
// keeps its own pipeline in globalsync.go until it moves onto
// channels.

// ChannelMode is how much a channel run may change.
type ChannelMode int

const (
	// ChannelRefresh rewrites index files and refreshes mounts that
	// already carry ours. It creates nothing outside .agents/index/.
	// Used by index, watch, add, adr, import, and the source commands,
	// so authoring a rule never edits a tool directory.
	ChannelRefresh ChannelMode = iota

	// ChannelMount refreshes, then removes legacy placements and
	// creates or repairs native mounts where mayMount allows. Used by
	// sync and fix.
	ChannelMount
)

// ChannelRun is one invocation's request.
type ChannelRun struct {
	Scope Scope
	Mode  ChannelMode

	// Explicit lists the tool names given to --targets on this
	// invocation. They give consent to edit a file the user owns
	// (mayMount). Targets read from .agents/config do not.
	Explicit []string
}

// ChannelState classifies one channel for sync output and status.
type ChannelState string

const (
	// ChannelSynced: the index file matches the render and the mount
	// reaches it.
	ChannelSynced ChannelState = "synced"

	// ChannelStale: the index file differs from the render, because
	// sources or AGENTS.md changed since the last run.
	ChannelStale ChannelState = "stale"

	// ChannelUnmounted: the native mount is absent (never synced, or
	// consent not given). Detail says why.
	ChannelUnmounted ChannelState = "unmounted"

	// ChannelConflict: a real file holds a LinkMount path, or a
	// symlinked directory sits on the way to it. It is left alone and
	// counted in sync's exit status.
	ChannelConflict ChannelState = "conflict"

	// ChannelShadowed: the mount is in place, but the tool reads
	// another file first.
	ChannelShadowed ChannelState = "shadowed"

	// ChannelManual: the config cannot be edited safely (JSONC or not
	// parseable). Detail holds the exact entry to add by hand.
	ChannelManual ChannelState = "manual"
)

// Channel is a ChannelSpec bound to one run: every path is absolute and
// the consent gate is already evaluated. bindChannels builds it, and
// nothing downstream derives a path again.
type Channel struct {
	Tool  string
	Scope Scope
	Spec  ChannelSpec

	// Tree is the .agents/ directory whose passive artifacts this
	// channel renders.
	Tree string

	// Index is <Tree>/index/<Spec.Format.IndexName(Tool)>.
	Index string

	// Native is the absolute link path or config file.
	Native string

	// Home is the tool's directory at Scope (.cursor, .github/copilot,
	// ...). Budgets and shadowing read it.
	Home string

	// Mountable and Why are mayMount's verdict for this run. A channel
	// that is not mountable still refreshes a mount that carries ours.
	Mountable bool
	Why       string
}

// ChannelResult is one channel after a run.
type ChannelResult struct {
	Channel  Channel
	Rendered Rendered
	State    ChannelState
	Detail   string
}

// IndexPolicy is the `index` key in .agents/config. It decides whether
// generated delivery is committed (SPEC-013 §Index policy). One value
// covers .agents/index/ and every LinkMount path, so a committed link
// never points at an ignored file.
type IndexPolicy int

const (
	// IndexLocal (default): .agents/index/ and the link paths are
	// gitignored together, so a fresh clone has neither and nothing
	// dangles. Output is OS-gated for this machine and, for MergeGlobal
	// channels (Cursor), carries this user's global rules.
	IndexLocal IndexPolicy = iota

	// IndexCommit: both are committed, so fresh clones and cloud agents
	// get delivery with no setup. Every OS scope is compiled (with OS
	// headers) and global rules are never merged, so the committed
	// bytes are the same on every contributor's machine.
	IndexCommit
)

// ReadConfigIndex reads `index = local|commit` from <agentsDir>/config.
// An absent file or key means local. Any other value is an error; the
// caller falls back to local, the choice that never commits anything.
func ReadConfigIndex(agentsDir string) (IndexPolicy, error) {
	val, err := readConfigKey(agentsDir, "index")
	if err != nil {
		return IndexLocal, err
	}
	switch strings.ToLower(val) {
	case "", "local":
		return IndexLocal, nil
	case "commit":
		return IndexCommit, nil
	default:
		return IndexLocal, fmt.Errorf("index = %q in .agents/config is not local or commit; using local", val)
	}
}

// readConfigKey returns key's value from <agentsDir>/config, or "" when
// the file or the key is absent. Lines are `key = value`; `#` starts a
// comment line. The first occurrence wins, as in the other config
// readers.
func readConfigKey(agentsDir, key string) (string, error) {
	data, err := os.ReadFile(filepath.Join(agentsDir, "config"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), nil
		}
	}
	return "", nil
}

// linksBucket reports whether local `sync` links bucket b into target's
// tool directory (SPEC-013 §Per-tool delivery).
//
// A tool with a local channel receives passive content through it, so
// the passive bucket (rules) is not folded into the tool's directory.
// That frees .cursor/rules for the .mdc link and retires the
// .github/copilot/rules, .codex/rules, and .opencode/rules links,
// which no tool read. Claude and Windsurf have no local channel (they
// read the folded .agents/rules natively) and keep every bucket, as do
// unregistered targets (wave). Every other bucket follows
// Bucket.SyncsToTool as before.
func linksBucket(target string, b Bucket) bool {
	if !b.SyncsToTool(target) {
		return false
	}
	tool, ok := ResolveTool(target)
	if !ok {
		return true
	}
	_, hasChannel := channelSpecs[tool.ID][ScopeLocal]
	return !(hasChannel && BucketDefaultSemantic(b.Artifact) == Passive)
}

// sameTree reports whether the project's .agents/ is the global root:
// the project is $HOME. Paths are compared after resolving symlinks; a
// missing global root is never the same tree.
func (a *App) sameTree() bool {
	local, err := filepath.EvalSymlinks(filepath.Join(a.ProjectRoot, ".agents"))
	if err != nil {
		return false
	}
	global, err := filepath.EvalSymlinks(a.ResolveGlobalRoot())
	if err != nil {
		return false
	}
	return local == global
}

// explicitTargets is ChannelRun.Explicit for project commands: the
// active targets when they came from --targets, else nothing.
func (a *App) explicitTargets() []string {
	if a.TargetsFromFlag {
		return a.ActiveTargets
	}
	return nil
}

// bindChannels turns the active targets into bound channels at project
// scope. For each target with a local channel spec it makes the paths
// absolute, observes the native path, and records mayMount's verdict.
//
// When the project's .agents/ is the global root (sameTree), a tool
// that also has a global channel is skipped and returned in deferred:
// global sync owns that tree's delivery for it, so Codex and Copilot
// never see the same rules twice from $HOME. Cursor has no global
// channel and keeps its local one, without merging (the trees are the
// same).
func (a *App) bindChannels(run ChannelRun) (chans []Channel, deferred []string, err error) {
	if run.Scope != ScopeLocal {
		return nil, nil, fmt.Errorf("%s channels are delivered by `sync-agents global sync`", run.Scope)
	}
	tree := filepath.Join(a.ProjectRoot, ".agents")
	same := a.sameTree()
	seen := map[string]bool{}
	for _, target := range a.ActiveTargets {
		tool, ok := ResolveTool(target)
		if !ok || seen[tool.ID] {
			continue
		}
		spec, ok := channelSpecs[tool.ID][ScopeLocal]
		if !ok {
			continue
		}
		seen[tool.ID] = true
		if _, global := channelSpecs[tool.ID][ScopeGlobal]; same && global {
			deferred = append(deferred, tool.ID)
			continue
		}
		ch := Channel{
			Tool:   tool.ID,
			Scope:  ScopeLocal,
			Spec:   spec,
			Tree:   tree,
			Index:  filepath.Join(tree, "index", spec.Format.IndexName(tool.ID)),
			Native: filepath.Join(a.ProjectRoot, filepath.FromSlash(mountPath(spec.Mount).Rel)),
			Home:   a.ResolveToolDir(tool, ScopeLocal),
		}
		facts, err := a.mountFacts(ch)
		if err != nil {
			return nil, nil, err
		}
		for _, name := range run.Explicit {
			if tool.Matches(name) {
				facts.Explicit = true
			}
		}
		ok, why := mayMount(spec.Mount, facts)
		ch.Mountable, ch.Why = ok, strings.ReplaceAll(why, "<tool>", tool.ID)
		chans = append(chans, ch)
	}
	return chans, deferred, nil
}

// mountPath is the native path a mount places or edits.
func mountPath(m Mount) NativePath {
	switch m := m.(type) {
	case LinkMount:
		return m.At
	case RegionMount:
		return m.Host
	case ConfigListMount:
		return m.File
	}
	panic(fmt.Sprintf("unknown mount %T", m))
}

// mountFacts observes ch's native path for mayMount.
func (a *App) mountFacts(ch Channel) (MountFacts, error) {
	f := MountFacts{Scope: ch.Scope, HomeExists: isDir(ch.Home)}
	fi, err := os.Lstat(ch.Native)
	if err != nil && !os.IsNotExist(err) {
		return f, err
	}
	f.NativeExists = err == nil
	if !f.NativeExists {
		return f, nil
	}
	switch m := ch.Spec.Mount.(type) {
	case LinkMount:
		f.CarriesOurs = fi.Mode()&os.ModeSymlink != 0 && linkSatisfied(ch.Native, a.linkSource(ch))
	case ConfigListMount:
		src, err := os.ReadFile(ch.Native)
		if err != nil {
			return f, err
		}
		_, changed, err := ensureJSONArrayEntry(src, m.Key, a.configEntry(ch))
		f.CarriesOurs = err == nil && !changed
	}
	return f, nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// linkSource is the relative symlink text from a LinkMount to its
// index file, e.g. "../../.agents/index/cursor.mdc".
func (a *App) linkSource(ch Channel) string {
	rel, err := filepath.Rel(filepath.Dir(ch.Native), ch.Index)
	if err != nil {
		return ch.Index
	}
	return filepath.ToSlash(rel)
}

// configEntry is the value a ConfigListMount lists: the index file
// relative to the project root, slash-separated
// (".agents/index/opencode.md"), so a committed opencode.json works in
// every clone.
func (a *App) configEntry(ch Channel) string {
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

// channelInputs is what every channel of a run renders from: computed
// once, so channels differ only in format, mount, and budget.
type channelInputs struct {
	policy  IndexPolicy
	project []Entry

	// global is the global root's passive entries, loaded only when a
	// MergeGlobal channel is bound under IndexLocal and the trees
	// differ. Nil otherwise.
	global []Entry
}

// loadChannelInputs discovers the passive entries chans render. A
// broken artifact is warned about and skipped, so one bad file never
// stops delivery of the rest.
func (a *App) loadChannelInputs(chans []Channel) (channelInputs, error) {
	tree := filepath.Join(a.ProjectRoot, ".agents")
	policy, err := ReadConfigIndex(tree)
	if err != nil {
		a.Warn(err.Error())
	}
	in := channelInputs{policy: policy}
	arts, err := discoverChannelArtifacts(tree, policy == IndexCommit)
	if err != nil {
		return in, err
	}
	var errs []error
	in.project, errs = passiveEntries(arts, ".agents")
	for _, e := range errs {
		a.Warn(fmt.Sprintf("skipped in .agents/index/: %v", e))
	}

	merge := false
	for _, ch := range chans {
		merge = merge || ch.Spec.MergeGlobal
	}
	if !merge || policy != IndexLocal || a.sameTree() {
		return in, nil
	}
	root := a.ResolveGlobalRoot()
	if !isDir(root) {
		return in, nil
	}
	garts, err := DiscoverArtifacts(root)
	if err != nil {
		return in, err
	}
	in.global, errs = passiveEntries(garts, "~/.agents")
	for _, e := range errs {
		a.Warn(fmt.Sprintf("skipped in .agents/index/: %v", e))
	}
	return in, nil
}

// render renders ch exactly as sync writes it. status calls it too and
// compares bytes, so a row says synced only when sync would write
// nothing.
func (a *App) render(ch Channel, in channelInputs) (Rendered, error) {
	entries := in.project
	if ch.Spec.MergeGlobal && in.global != nil {
		entries = mergeEntries(in.project, in.global)
	}
	var b Budget
	if ch.Spec.Budget != nil {
		var err error
		b, err = ch.Spec.Budget(ToolContext{Home: ch.Home, Parent: a.ResolveGlobalRootParent(), Env: a.ToolEnv})
		if err != nil {
			return Rendered{}, err
		}
	}
	fr := Frame{Banner: localBanner}
	if ch.Spec.Format == FormatCodexOverride {
		data, err := os.ReadFile(filepath.Join(a.ProjectRoot, "AGENTS.md"))
		if err != nil && !os.IsNotExist(err) {
			return Rendered{}, err
		}
		fr.AgentsMD = data
	}
	return renderChannel(ch.Spec.Format, entries, fr, b)
}

// deliverChannels is the single entry point for per-tool content.
//
// In ChannelMount mode it first removes legacy placements (before any
// link goes into .cursor/rules, which may still be the old fold
// symlink into .agents/rules). Then, for each bound channel, it
// renders, writes the index file (atomic; untouched when unchanged, so
// a second run writes nothing), and mounts. Each step is idempotent and
// a mount never points at an index file that was not written first.
//
// A channel whose render fails (a broken budget config) is warned
// about and skipped; the rest still deliver.
func (a *App) deliverChannels(run ChannelRun) ([]ChannelResult, error) {
	var gone []string
	if run.Mode == ChannelMount {
		gone = a.removeLegacyPlacements()
	}
	chans, deferred, err := a.bindChannels(run)
	if err != nil {
		return nil, err
	}
	if run.Mode == ChannelMount {
		for _, id := range deferred {
			a.Info(fmt.Sprintf("%s: skipped at project scope: this project's .agents/ is the global root, so `sync-agents global sync` delivers it", id))
		}
	}
	if len(chans) == 0 {
		return nil, nil
	}
	in, err := a.loadChannelInputs(chans)
	if err != nil {
		return nil, err
	}

	var results []ChannelResult
	for _, ch := range chans {
		r, err := a.render(ch, in)
		if err != nil {
			a.Warn(fmt.Sprintf("%s: not delivered: %v", ch.Tool, err))
			continue
		}
		if err := a.writeIndex(ch, r); err != nil {
			return results, fmt.Errorf("write %s: %w", a.display(ch.Index), err)
		}
		state, detail, err := a.mount(ch, run.Mode, gone)
		if err != nil {
			return results, fmt.Errorf("%s: %w", ch.Tool, err)
		}
		res := ChannelResult{Channel: ch, Rendered: r, State: state, Detail: detail}
		results = append(results, res)
		a.reportChannel(res, run.Mode)
	}
	return results, nil
}

// writeIndex writes r to ch.Index unless it already holds those bytes.
func (a *App) writeIndex(ch Channel, r Rendered) error {
	if a.DryRun {
		if cur, err := os.ReadFile(ch.Index); err != nil || !bytes.Equal(cur, r.Bytes) {
			fmt.Fprintf(a.Stdout, "  would write: %s\n", a.display(ch.Index))
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ch.Index), 0o755); err != nil {
		return err
	}
	changed, err := writeIfChanged(ch.Index, r.Bytes)
	if err == nil && changed {
		a.Info("Regenerated " + a.display(ch.Index))
	}
	return err
}

// mount places or repairs ch's native mount and reports the state. In
// ChannelRefresh mode it only observes: refresh creates nothing outside
// .agents/index/, and a mount that carries ours needs no write because
// it points at (or lists) the index file that was just rewritten.
//
// gone lists the legacy paths this run removed. Under --dry-run they
// are still on disk, and a link that would be placed through one of
// them is reported as the real run would place it.
func (a *App) mount(ch Channel, mode ChannelMode, gone []string) (ChannelState, string, error) {
	if mode == ChannelRefresh {
		return a.observe(ch)
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
		return "", "", fmt.Errorf("region mounts are delivered by `sync-agents global sync`")
	}
	if err != nil {
		return "", "", err
	}
	return a.shadowed(ch, state, detail)
}

// observe classifies ch's native mount without writing anything.
func (a *App) observe(ch Channel) (ChannelState, string, error) {
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
		return "", "", fmt.Errorf("region mounts are reported by `sync-agents global status`")
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
	return "not placed yet; run `sync-agents sync`"
}

func (a *App) observeLink(ch Channel) (ChannelState, string, error) {
	if at := symlinkedParent(a.ProjectRoot, ch.Native); at != "" {
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
	if at := symlinkedParent(a.ProjectRoot, ch.Native); at != "" && !slices.Contains(gone, at) {
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

// reportChannel prints one line per channel after a ChannelMount run
// (sync, fix): where the tool now reads from, or why it does not, plus
// budget notes. Refresh runs stay quiet apart from writeIndex's
// "Regenerated" line.
func (a *App) reportChannel(res ChannelResult, mode ChannelMode) {
	ch, r := res.Channel, res.Rendered
	if r.OverCap() {
		a.Warn(fmt.Sprintf("%s: %s is %d of %d %s even with every rule as a pointer, so %s will truncate it. Raise %s, or mark rules `trigger: model_decision`.",
			ch.Tool, a.display(ch.Native), r.Size, r.Budget.Cap.Limit, capUnitName(r.Budget.Cap.Unit), ch.Tool, r.Budget.Cap.Knob))
	} else if len(r.Pointers) > 0 && r.Budget.Cap.Limit > 0 && mode == ChannelMount {
		a.Info(fmt.Sprintf("%s: %d inlined, %d as pointers (%s)", ch.Tool, len(r.Inlined), len(r.Pointers), strings.Join(r.Pointers, ", ")))
	}
	if mode != ChannelMount {
		return
	}
	where := a.display(ch.Native)
	switch res.State {
	case ChannelSynced:
		a.Info(fmt.Sprintf("%-8s %s (%s)", ch.Tool, a.mountLabel(ch), sizeNote(r)))
	case ChannelConflict:
		a.Warn(fmt.Sprintf("conflict: %s %s: %s", ch.Tool, where, res.Detail))
	case ChannelManual, ChannelShadowed:
		a.Warn(fmt.Sprintf("%s %s: %s", ch.Tool, where, res.Detail))
	default:
		a.Info(fmt.Sprintf("%-8s %s: %s", ch.Tool, where, res.Detail))
	}
}

// mountLabel says where the tool reads the index from:
// ".cursor/rules/sync-agents.mdc -> .agents/index/cursor.mdc" or
// `opencode.json "instructions" lists .agents/index/opencode.md`.
func (a *App) mountLabel(ch Channel) string {
	if m, ok := ch.Spec.Mount.(ConfigListMount); ok {
		return fmt.Sprintf("%s %q lists %s", a.display(ch.Native), m.Key, a.configEntry(ch))
	}
	return a.display(ch.Native) + " -> " + a.display(ch.Index)
}

// sizeNote summarizes a render: entry counts, and the size against the
// cap when there is one.
func sizeNote(r Rendered) string {
	n := fmt.Sprintf("%d rules", len(r.Inlined)+len(r.Pointers))
	if len(r.Pointers) > 0 {
		n += fmt.Sprintf(", %d as pointers", len(r.Pointers))
	}
	if r.Budget.Cap.Limit > 0 {
		n += fmt.Sprintf(", %d of %d %s", r.Size, r.Budget.Cap.Limit, capUnitName(r.Budget.Cap.Unit))
	}
	return n
}

func capUnitName(u CapUnit) string {
	if u == CapUTF16 {
		return "chars"
	}
	return "bytes"
}

// channelRows reports every bound channel without writing anything,
// for `status`. It renders exactly as deliverChannels does and compares
// bytes with the index file, so a row says synced only when sync would
// write nothing. Channels deferred to global sync (sameTree) get a
// "skipped" row.
func (a *App) channelRows(scope Scope, explicit []string) ([]StatusEntry, error) {
	chans, deferred, err := a.bindChannels(ChannelRun{Scope: scope, Mode: ChannelRefresh, Explicit: explicit})
	if err != nil {
		return nil, err
	}
	var rows []StatusEntry
	if len(chans) > 0 {
		in, err := a.loadChannelInputs(chans)
		if err != nil {
			return nil, err
		}
		for _, ch := range chans {
			rows = append(rows, a.channelRow(ch, in))
		}
	}
	for _, id := range deferred {
		rows = append(rows, StatusEntry{Tool: id, State: "skipped",
			Detail: "this project's .agents/ is the global root; `sync-agents global sync` delivers it"})
	}
	return rows, nil
}

func (a *App) channelRow(ch Channel, in channelInputs) StatusEntry {
	row := StatusEntry{Tool: ch.Tool, DestinationPath: ch.Native}
	r, err := a.render(ch, in)
	if err != nil {
		row.State, row.Detail = "error", err.Error()
		return row
	}
	state, detail, err := a.observe(ch)
	if err != nil {
		row.State, row.Detail = "error", err.Error()
		return row
	}
	if state != ChannelConflict && state != ChannelManual {
		if cur, err := os.ReadFile(ch.Index); err != nil || !bytes.Equal(cur, r.Bytes) {
			state, detail = ChannelStale, a.display(ch.Index)+" is out of date; run `sync-agents index`"
		}
	}
	row.State, row.Detail = string(state), detail
	if state == ChannelSynced {
		row.Detail = a.mountLabel(ch)
	}
	return row
}

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

// legacyFold is one tool-directory link the pre-SPEC-013 sync made:
// Path folded (or drilled) Source, the bucket directory in .agents/.
type legacyFold struct {
	Path, Source string
}

// legacyFoldPaths lists the folds linksBucket now withholds: every
// <tool dir>/<bucket> fold of a passive bucket into a tool that has a
// local channel (.cursor/rules, .github/copilot/rules, .codex/rules,
// .opencode/rules). It is derived from the same registry linksBucket
// reads, so the two cannot disagree.
func legacyFoldPaths(root string) []legacyFold {
	var folds []legacyFold
	for _, tool := range Tools {
		for _, b := range Buckets {
			if b.SyncsToTool(tool.ID) && !linksBucket(tool.ID, b) {
				folds = append(folds, legacyFold{
					Path:   filepath.Join(ResolveTargetDir(tool.ID, root), b.Dir),
					Source: filepath.Join(root, ".agents", b.Dir),
				})
			}
		}
	}
	sort.Slice(folds, func(i, j int) bool { return folds[i].Path < folds[j].Path })
	return folds
}

// removeLegacyPlacements removes the local legacy links (SPEC-013
// §Migration, step 4) and returns the paths it removed. The proof is
// the link itself: a symlink that resolves into the folded bucket
// (.agents/rules) is sync-agents' (the old fold). A real directory
// there is the tool's own: only the links inside it that resolve into
// the bucket (the old per-rule drill) are removed, and the directory
// goes only when that leaves it empty. Our own channel link in
// .cursor/rules points into .agents/index/, so it is never taken for a
// legacy one. Anything else is reported and left. It is idempotent and
// dry-run aware.
func (a *App) removeLegacyPlacements() []string {
	var removed []string
	for _, fold := range legacyFoldPaths(a.ProjectRoot) {
		path, tree := fold.Path, fold.Source
		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			if !pointsInto(path, tree) {
				a.Info(fmt.Sprintf("left %s: a symlink outside %s is not sync-agents'", a.display(path), a.display(tree)))
				continue
			}
			if a.removePath(path) {
				removed = append(removed, path)
				a.pruneEmptyParents(filepath.Dir(path))
			}
		case fi.IsDir():
			entries, err := os.ReadDir(path)
			if err != nil {
				a.Warn(fmt.Sprintf("read %s: %v", a.display(path), err))
				continue
			}
			for _, e := range entries {
				p := filepath.Join(path, e.Name())
				if e.Type()&os.ModeSymlink != 0 && pointsInto(p, tree) && a.removePath(p) {
					removed = append(removed, p)
				}
			}
			a.pruneEmptyParents(path)
		default:
			a.Info(fmt.Sprintf("left %s: a real file is not sync-agents'", a.display(path)))
		}
	}
	if len(removed) > 0 && !a.DryRun {
		a.Info("Removed legacy rule links: rules now reach Cursor, Copilot, Codex, and opencode through .agents/index/")
	}
	return removed
}

// pointsInto reports whether the symlink at link resolves inside dir.
// A dangling link is judged by its text, so a fold whose target was
// deleted is still recognized.
func pointsInto(link, dir string) bool {
	within := func(p, root string) bool {
		return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
	}
	if real, err := filepath.EvalSymlinks(link); err == nil {
		if rdir, err := filepath.EvalSymlinks(dir); err == nil && within(real, rdir) {
			return true
		}
	}
	text, err := os.Readlink(link)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(text) {
		text = filepath.Join(filepath.Dir(link), text)
	}
	return within(filepath.Clean(text), filepath.Clean(dir))
}

// gitignoreEntries derives the exact lines sync ensures in .gitignore
// from the channels this run delivered (SPEC-013 §Index policy).
// IndexLocal adds ".agents/index/" and each LinkMount path sync placed;
// IndexCommit adds neither, so both are committed together. A link
// path held by a real file (conflict) is the user's and is never
// ignored. "CLAUDE.md" is added when it is, or is about to be, our
// symlink (U5 policy), under either policy. Paths are relative to
// root, slash-separated.
func gitignoreEntries(p IndexPolicy, root string, results []ChannelResult, claude ClaudeMDDecision) []string {
	var out []string
	if p == IndexLocal && len(results) > 0 {
		out = append(out, ".agents/index/")
		for _, res := range results {
			if _, ok := res.Channel.Spec.Mount.(LinkMount); !ok || res.State == ChannelConflict {
				continue
			}
			if rel, err := filepath.Rel(root, res.Channel.Native); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
	}
	if claude.linked() {
		out = append(out, "CLAUDE.md")
	}
	return out
}

// refreshIndex is what a command that changed .agents/ runs afterwards
// (add, adr, the source commands): the one-time AGENTS.md migration,
// then a ChannelRefresh of .agents/index/. Failures are warnings,
// because the command's own change already succeeded.
func (a *App) refreshIndex() {
	a.migrateAgentsMDOrWarn()
	if _, err := a.deliverChannels(ChannelRun{Scope: ScopeLocal, Mode: ChannelRefresh}); err != nil {
		a.Warn(fmt.Sprintf("regenerate .agents/index/: %v", err))
	}
}

// countState counts results in state s.
func countState(results []ChannelResult, s ChannelState) int {
	n := 0
	for _, r := range results {
		if r.State == s {
			n++
		}
	}
	return n
}

// printChannelRows prints the "Delivery channels" section of `status`:
// one row per channel, in the states channelRows reports.
func (a *App) printChannelRows() error {
	rows, err := a.channelRows(ScopeLocal, a.explicitTargets())
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	fmt.Fprintln(a.Stdout)
	fmt.Fprintln(a.Stdout, "Delivery channels (.agents/index/):")
	for _, r := range rows {
		fmt.Fprintf(a.Stdout, "  [%s] %-8s %s\n", r.State, r.Tool, r.Detail)
	}
	return nil
}
