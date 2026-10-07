package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file is the shell around channel.go (SPEC-013 §Shape): it binds
// the channel specs to this run's paths, observes the filesystem,
// writes .agents/index/<tool>.<ext>, and places or reports each tool's
// native mount. Commands see four entry points: deliverChannels (sync,
// fix, index, add, global sync, ...), channelRows (status, global
// status), cleanChannels (clean, global clean), and gitignoreEntries
// (sync's .gitignore lines). Every one of them starts from the same
// bindChannels result, so no path is derived twice and status, clean,
// and .gitignore agree with sync by construction.

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
	// sync, fix, and global sync.
	ChannelMount
)

// ChannelRun is one invocation's request.
type ChannelRun struct {
	Scope Scope
	Mode  ChannelMode

	// Explicit lists the tool names given to --targets on this
	// invocation. They give consent to edit a file the user owns and,
	// at global scope, open the home gate (mayMount). Targets read from
	// .agents/config do not.
	Explicit []string

	// Tools (global scope only) are the run's tools, already filtered by
	// --targets and bound by resolveSyncTools, so every tool home
	// ($CODEX_HOME, the OpenClaw workspace) is resolved once and a
	// resolver error is reported once. Project scope binds from
	// App.ActiveTargets instead.
	Tools []Tool
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

	// Native is the absolute link path, region host, or config file.
	Native string

	// Home is the tool's directory at Scope (.cursor, ~/.codex, the
	// OpenClaw workspace, ...). Budgets, shadowing, and the global home
	// gate read it.
	Home string

	// Explicit: --targets named this tool on this invocation.
	Explicit bool

	// Mountable and Why are mayMount's verdict for this run. A channel
	// that is not mountable still refreshes a mount that carries ours
	// (mayMount says yes to those).
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

// bindChannels turns the run's tools into bound channels: for each tool
// with a channel spec at run.Scope it makes the paths absolute,
// observes the native path, and records mayMount's verdict.
//
// Project scope binds the active targets. When the project's .agents/
// is the global root (sameTree), a tool that also has a global channel
// is skipped and returned in deferred: global sync owns that tree's
// delivery for it, so Codex and Copilot never see the same rules twice
// from $HOME. Cursor has no global channel and keeps its local one,
// without merging (the trees are the same).
//
// Global scope binds run.Tools against the global root and its parent
// (normally $HOME). Nothing is deferred there.
func (a *App) bindChannels(run ChannelRun) (chans []Channel, deferred []string, err error) {
	if run.Scope == ScopeGlobal {
		chans, err = a.bindGlobalChannels(run)
		return chans, nil, err
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
		ch, err := a.bindChannel(tool, ScopeLocal, spec, tree, a.ProjectRoot, a.ResolveToolDir(tool, ScopeLocal), run.Explicit)
		if err != nil {
			return nil, nil, err
		}
		chans = append(chans, ch)
	}
	return chans, deferred, nil
}

// bindGlobalChannels binds every tool in run.Tools that has a global
// channel spec. Each tool is already bound (resolveSyncTools), so its
// home is DirForScope(ScopeGlobal, parent).
func (a *App) bindGlobalChannels(run ChannelRun) ([]Channel, error) {
	tree := a.ResolveGlobalRoot()
	parent := a.ResolveGlobalRootParent()
	var chans []Channel
	for _, tool := range run.Tools {
		spec, ok := channelSpecs[tool.ID][ScopeGlobal]
		if !ok {
			continue
		}
		ch, err := a.bindChannel(tool, ScopeGlobal, spec, tree, parent, tool.DirForScope(ScopeGlobal, parent), run.Explicit)
		if err != nil {
			return nil, err
		}
		chans = append(chans, ch)
	}
	return chans, nil
}

// bindChannel binds one spec: base is the scope base (the project root,
// or the global root's parent), home the tool's directory at the scope.
func (a *App) bindChannel(tool Tool, scope Scope, spec ChannelSpec, tree, base, home string, explicit []string) (Channel, error) {
	ch := Channel{
		Tool:   tool.ID,
		Scope:  scope,
		Spec:   spec,
		Tree:   tree,
		Index:  filepath.Join(tree, "index", spec.Format.IndexName(tool.ID)),
		Native: nativeAt(mountPath(spec.Mount), base, home),
		Home:   home,
	}
	for _, name := range explicit {
		if tool.Matches(strings.ToLower(strings.TrimSpace(name))) {
			ch.Explicit = true
		}
	}
	facts, err := a.mountFacts(ch)
	if err != nil {
		return Channel{}, err
	}
	ok, why := mayMount(spec.Mount, facts)
	ch.Mountable, ch.Why = ok, strings.ReplaceAll(why, "<tool>", tool.ID)
	return ch, nil
}

// nativeAt resolves np against the scope base or the tool's home.
func nativeAt(np NativePath, base, home string) string {
	root := base
	if np.Under == AnchorHome {
		root = home
	}
	return filepath.Join(root, filepath.FromSlash(np.Rel))
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
	f := MountFacts{Scope: ch.Scope, Explicit: ch.Explicit, HomeExists: isDir(ch.Home)}
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
	case RegionMount:
		// Our markers are ours, and so is a host that is still the
		// pre-SPEC-013 whole-file concat: sync rewrites it in place as
		// the region, whose markers then carry consent.
		host, err := os.ReadFile(ch.Native)
		if err != nil {
			return f, err
		}
		_, _, found := m.Region.locate(string(host))
		f.CarriesOurs = found || isLegacyConcat(host)
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

// loadChannelInputs discovers the passive entries chans render, from
// the tree they share (the project's .agents/, or the global root). A
// broken artifact is warned about and skipped, so one bad file never
// stops delivery of the rest.
//
// The index policy is a project setting: global scope always renders
// for this machine (OS-gated) and never merges.
func (a *App) loadChannelInputs(scope Scope, chans []Channel) (channelInputs, error) {
	tree := chans[0].Tree
	displayRoot := ".agents"
	in := channelInputs{policy: IndexLocal}
	if scope == ScopeGlobal {
		displayRoot = "~/.agents"
	} else {
		policy, err := ReadConfigIndex(tree)
		if err != nil {
			a.Warn(err.Error())
		}
		in.policy = policy
	}
	skipped := "skipped in " + a.display(filepath.Join(tree, "index")) + "/"
	arts, err := discoverChannelArtifacts(tree, in.policy == IndexCommit)
	if err != nil {
		return in, err
	}
	var errs []error
	in.project, errs = passiveEntries(arts, displayRoot)
	for _, e := range errs {
		a.Warn(fmt.Sprintf("%s: %v", skipped, e))
	}

	merge := false
	for _, ch := range chans {
		merge = merge || ch.Spec.MergeGlobal
	}
	if scope != ScopeLocal || !merge || in.policy != IndexLocal || a.sameTree() {
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
		a.Warn(fmt.Sprintf("%s: %v", skipped, e))
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
	if rm, ok := ch.Spec.Mount.(RegionMount); ok {
		// The tool counts the whole host file against its limit, so
		// everything that will sit outside our region body (the host's
		// own text and the markers) is reserved.
		host, err := os.ReadFile(ch.Native)
		if err != nil && !os.IsNotExist(err) {
			return Rendered{}, err
		}
		b.Reserved += b.Cap.Measure([]byte(regionHost(host, rm.Region, nil)))
	}
	fr := Frame{Banner: localBanner}
	if ch.Scope == ScopeGlobal {
		fr.Banner = globalBanner
	}
	if ch.Spec.Format == FormatCodexOverride {
		data, err := os.ReadFile(filepath.Join(a.ProjectRoot, "AGENTS.md"))
		if err != nil && !os.IsNotExist(err) {
			return Rendered{}, err
		}
		fr.AgentsMD = data
	}
	return renderChannel(ch.Spec.Format, entries, fr, b)
}

// deliverChannels is the single entry point for per-tool content at
// either scope.
//
// In ChannelMount mode it first removes legacy placements (before any
// link goes into .cursor/rules, which may still be the old fold
// symlink into .agents/rules). Then, for each bound channel, it
// renders, writes the index file (atomic; untouched when unchanged, so
// a second run writes nothing), and mounts. Each step is idempotent and
// a mount never points at an index file that was not written first.
//
// At global scope every registered tool is bound, installed or not, so
// a channel mayMount declines is not delivered at all: no index file,
// no output. It is returned as unmounted for the caller, reported only
// when --targets named the tool, and explained by global status. That
// keeps a plain `global sync` silent about tools the user does not have
// or has not consented to (SPEC-012). At project scope the user listed
// the targets, so the index is written and the mount reported either
// way.
//
// A channel whose render fails (a broken budget config) is warned
// about and skipped; the rest still deliver.
func (a *App) deliverChannels(run ChannelRun) ([]ChannelResult, error) {
	var gone []string
	if run.Mode == ChannelMount {
		if run.Scope == ScopeGlobal {
			gone = a.removeGlobalLegacyPlacements(run.Tools)
		} else {
			gone = a.removeLegacyPlacements()
		}
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
	in, err := a.loadChannelInputs(run.Scope, chans)
	if err != nil {
		return nil, err
	}

	var results []ChannelResult
	for _, ch := range chans {
		if run.Scope == ScopeGlobal && !ch.Mountable {
			res := ChannelResult{Channel: ch, State: ChannelUnmounted, Detail: ch.Why}
			results = append(results, res)
			if ch.Explicit {
				a.reportChannel(res, run.Mode)
			}
			continue
		}
		r, err := a.render(ch, in)
		if err != nil {
			a.Warn(fmt.Sprintf("%s: not delivered: %v", ch.Tool, err))
			continue
		}
		if err := a.writeIndex(ch, r); err != nil {
			return results, fmt.Errorf("write %s: %w", a.display(ch.Index), err)
		}
		state, detail, err := a.mount(ch, r, run.Mode, gone)
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
