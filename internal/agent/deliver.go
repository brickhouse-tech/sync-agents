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
