package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/brickhouse-tech/sync-agents/internal/agent/source"
)

// sourcecmd.go is the CLI glue between App and the source package
// (SPEC-003). The orchestration lives in internal/agent/source —
// scope-agnostic, testable without an App; this file only resolves
// scope (--global → ResolveGlobalRoot), adapts the bucket registry,
// renders reports through Info/Warn/Error, and regenerates AGENTS.md
// after successful writes (SPEC-003 Q4).

// SourceCmdOpts carries the source-command flags that aren't already
// persistent App fields (Force/DryRun live on App like every other
// command).
type SourceCmdOpts struct {
	// Global routes the operation at the user-scope .agents/ tree
	// (SPEC-002 global root resolution) instead of the project tree.
	Global bool

	// Offline forbids network access: refs resolve from the lock and
	// tarballs must already be in the cache.
	Offline bool

	// Only restricts pull to one entry (--only NAME).
	Only string

	// JSON switches list output to machine-readable form.
	JSON bool

	// Keep makes remove leave the artifact on disk as a manual one.
	Keep bool

	// Fetcher overrides the GitHub fetcher — tests inject an
	// httptest-backed one here. Nil means production GitHub.
	Fetcher source.Fetcher
}

// sourceAgentsDir resolves which .agents/ tree the command targets.
func (a *App) sourceAgentsDir(global bool) string {
	if global {
		return a.ResolveGlobalRoot()
	}
	return filepath.Join(a.ProjectRoot, ".agents")
}

// sourcePuller assembles a Puller for the requested scope. The bucket
// registry is adapted to plain data here — the source package must
// not import this package, and this single translation point is what
// keeps tree fanout automatically in step when SPEC-004 adds buckets.
func (a *App) sourcePuller(opts SourceCmdOpts) *source.Puller {
	f := opts.Fetcher
	if f == nil {
		f = &source.GitHubFetcher{}
	}
	buckets := make([]source.BucketInfo, len(Buckets))
	for i, b := range Buckets {
		buckets[i] = source.BucketInfo{Dir: b.Dir, DirPerArtifact: b.DirPerArtifact}
	}
	return &source.Puller{
		AgentsDir: a.sourceAgentsDir(opts.Global),
		Fetcher:   f,
		Buckets:   buckets,
		Out:       a.Stdout,
		Err:       a.Stderr,
	}
}

// renderPullReport prints per-entry outcomes: successes to stdout,
// failures to stderr (the local-modification scenario requires the
// reason on stderr).
func (a *App) renderPullReport(rep source.PullReport) {
	for _, r := range rep.Results {
		label := r.Name
		if r.SHA != "" {
			label += " @ " + shortRef(r.SHA)
		}
		switch r.Kind {
		case source.ResultFailed:
			a.Error(fmt.Sprintf("[failed] %s: %s", label, r.Detail))
		case source.ResultSkipped:
			a.Info(fmt.Sprintf("%s %s", r.Detail, label))
		default:
			msg := fmt.Sprintf("[%s] %s", r.Kind, label)
			if r.Detail != "" {
				msg += " (" + r.Detail + ")"
			}
			a.Info(msg)
		}
	}
}

func shortRef(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// finishSourceWrite regenerates AGENTS.md after a source command that
// changed the tree. Only at project scope — the global tree's fan-out
// is `global sync`'s job, and there is no AGENTS.md contract there.
func (a *App) finishSourceWrite(changed bool, opts SourceCmdOpts) {
	if !changed || opts.Global || a.DryRun {
		return
	}
	a.generateAgentsMD()
	a.Info("Updated AGENTS.md index")
}

// CmdPull implements `sync-agents pull`.
func (a *App) CmdPull(opts SourceCmdOpts) error {
	p := a.sourcePuller(opts)
	rep, err := p.Pull(context.Background(), source.PullOpts{
		Force:   a.Force,
		DryRun:  a.DryRun,
		Offline: opts.Offline,
		Only:    onlyList(opts.Only),
	})
	a.renderPullReport(rep)
	a.finishSourceWrite(rep.Changed(), opts)
	a.Info("pull: " + rep.Summary())
	if err != nil {
		a.Error(err.Error())
		return err
	}
	return nil
}

// CmdUpdate implements `sync-agents update [NAME]`.
func (a *App) CmdUpdate(name string, opts SourceCmdOpts) error {
	p := a.sourcePuller(opts)
	rep, err := p.Pull(context.Background(), source.PullOpts{
		Force:      a.Force,
		DryRun:     a.DryRun,
		Only:       onlyList(name),
		UpdateMode: true,
	})
	a.renderPullReport(rep)
	a.finishSourceWrite(rep.Changed(), opts)
	a.Info("update: " + rep.Summary())
	if err != nil {
		a.Error(err.Error())
		return err
	}
	return nil
}

// CmdSourceAdd implements `sync-agents source add <entry>`.
func (a *App) CmdSourceAdd(entry string, opts SourceCmdOpts) error {
	if entry == "" {
		a.Error("Usage: sync-agents source add <type>:<owner>/<repo>[@ref][/path]")
		return fmt.Errorf("missing entry")
	}
	p := a.sourcePuller(opts)
	rep, err := p.Add(context.Background(), entry, source.PullOpts{
		Force:   a.Force,
		DryRun:  a.DryRun,
		Offline: opts.Offline,
	})
	a.renderPullReport(rep)
	a.finishSourceWrite(rep.Changed(), opts)
	if err != nil {
		a.Error(err.Error())
		return err
	}
	a.Info(fmt.Sprintf("Added source: %s", entry))
	return nil
}

// CmdSourceRemove implements `sync-agents source remove <name>`.
func (a *App) CmdSourceRemove(name string, opts SourceCmdOpts) error {
	if name == "" {
		a.Error("Usage: sync-agents source remove <name> [--keep]")
		return fmt.Errorf("missing name")
	}
	p := a.sourcePuller(opts)
	if err := p.Remove(name, opts.Keep); err != nil {
		a.Error(err.Error())
		return err
	}
	a.finishSourceWrite(true, opts)
	if opts.Keep {
		a.Info(fmt.Sprintf("Removed source %s (artifact kept as manual)", name))
	} else {
		a.Info(fmt.Sprintf("Removed source %s and its artifact(s)", name))
	}
	return nil
}

// CmdSourceList implements `sync-agents source list`.
func (a *App) CmdSourceList(opts SourceCmdOpts) error {
	p := a.sourcePuller(opts)
	items, err := p.List()
	if err != nil {
		a.Error(err.Error())
		return err
	}
	if opts.JSON {
		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, string(data))
		return nil
	}
	if len(items) == 0 {
		a.Info("No sources declared. Add one with `sync-agents source add <entry>`.")
		return nil
	}
	for _, it := range items {
		sha := it.ResolvedSHA
		if sha == "" {
			sha = "(never pulled)"
		} else {
			sha = shortRef(sha)
		}
		fmt.Fprintf(a.Stdout, "[%s] %s  %s\n", it.Status, it.Entry, sha)
	}
	return nil
}

// CmdSourceBundle implements `sync-agents source bundle`.
func (a *App) CmdSourceBundle(opts SourceCmdOpts) error {
	p := a.sourcePuller(opts)
	rep, err := p.Bundle()
	for _, w := range rep.Warnings {
		a.Warn(w)
	}
	for _, e := range rep.Added {
		a.Info("bundled: " + e)
	}
	for _, f := range rep.Flipped {
		a.Info("now manifest-governed: " + f)
	}
	if err != nil {
		a.Error(err.Error())
		return err
	}
	if len(rep.Added)+len(rep.Flipped) == 0 {
		a.Info("Nothing to bundle — no artifacts with origin metadata outside the manifest.")
	}
	return nil
}

// CmdSourceDetach implements `sync-agents source detach <name>`.
func (a *App) CmdSourceDetach(name string, opts SourceCmdOpts) error {
	if name == "" {
		a.Error("Usage: sync-agents source detach <name>")
		return fmt.Errorf("missing name")
	}
	p := a.sourcePuller(opts)
	if err := p.Detach(name); err != nil {
		a.Error(err.Error())
		return err
	}
	a.Info(fmt.Sprintf("Detached %s — artifact kept, now source: \"manual\"", name))
	return nil
}

// onlyList adapts the --only flag (single name, possibly
// comma-separated) to the orchestrator's slice form.
func onlyList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
