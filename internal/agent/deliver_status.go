package agent

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// This file reports channels: the per-channel line sync and fix print,
// and the status rows that re-render each channel and compare bytes,
// so a row says synced only when sync would write nothing.

// reportChannel prints one line per channel after a ChannelMount run
// (sync, fix, global sync): where the tool now reads from, or why it
// does not, plus budget warnings. Refresh runs stay quiet apart from
// writeIndex's "Regenerated" line and the over-cap warning.
//
// SPEC-013 §Budget: rules the cap demoted to pointers are named with
// the setting that raises the cap, and a bundle that is over the cap
// even as pointers gets a louder warning, because the tool will then
// truncate it.
func (a *App) reportChannel(res ChannelResult, mode ChannelMode) {
	ch, r := res.Channel, res.Rendered
	if r.OverCap() {
		a.Warn(fmt.Sprintf("%s: %s is %d of %d %s even with every rule as a pointer, so %s will truncate it. %s",
			ch.Tool, a.display(ch.Native), r.Size, r.Budget.Cap.Limit, capUnitName(r.Budget.Cap.Unit), ch.Tool, capAdvice(ch.Tool, r.Budget.Cap)))
	} else if len(r.Demoted) > 0 && mode == ChannelMount {
		a.Warn(fmt.Sprintf("%s: %d rule(s) did not fit in %d %s and are pointers instead: %s. %s",
			ch.Tool, len(r.Demoted), r.Budget.Cap.Limit, capUnitName(r.Budget.Cap.Unit), strings.Join(r.Demoted, ", "), capAdvice(ch.Tool, r.Budget.Cap)))
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

// capAdvice names what the user can do about a cap: raise its knob, or,
// when the vendor fixes the limit, choose which rules become pointers.
func capAdvice(tool string, c Cap) string {
	if c.Knob == "" {
		return fmt.Sprintf("%s fixes this limit; mark rules `trigger: model_decision` to choose which become pointers.", tool)
	}
	return fmt.Sprintf("Raise %s, or mark rules `trigger: model_decision`.", c.Knob)
}

// mountLabel says where the tool reads the index from:
// ".cursor/rules/sync-agents.mdc -> .agents/index/cursor.mdc",
// `opencode.json "instructions" lists .agents/index/opencode.md`, or
// "~/.codex/AGENTS.md region codex-rules".
func (a *App) mountLabel(ch Channel) string {
	switch m := ch.Spec.Mount.(type) {
	case ConfigListMount:
		return fmt.Sprintf("%s %q lists %s", a.display(ch.Native), m.Key, a.configEntry(ch))
	case RegionMount:
		return fmt.Sprintf("%s region %s", a.display(ch.Native), m.Region.Name)
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

// channelRows reports every channel run binds without writing
// anything, for `status` and `global status`. It renders exactly as
// deliverChannels does and compares bytes with the index file (and a
// region with its host), so a row says synced only when sync would
// write nothing. Channels deferred to global sync (sameTree) get a
// "skipped" row; at global scope a tool with no file to deliver to
// (channelGaps) gets a "gap" row.
func (a *App) channelRows(run ChannelRun) ([]StatusEntry, error) {
	run.Mode = ChannelRefresh
	chans, deferred, err := a.bindChannels(run)
	if err != nil {
		return nil, err
	}
	var rows []StatusEntry
	if len(chans) > 0 {
		in, err := a.loadChannelInputs(run.Scope, chans)
		if err != nil {
			return nil, err
		}
		for _, ch := range chans {
			rows = append(rows, a.channelRow(ch, in))
		}
	}
	for _, id := range deferred {
		rows = append(rows, StatusEntry{Tool: id, State: "skipped", IsChannel: true,
			Detail: "this project's .agents/ is the global root; `sync-agents global sync` delivers it"})
	}
	if run.Scope == ScopeGlobal {
		for _, tool := range run.Tools {
			if why, ok := channelGaps[tool.ID][ScopeGlobal]; ok {
				rows = append(rows, StatusEntry{Tool: tool.ID, State: "gap", IsChannel: true, Detail: why})
			}
		}
	}
	return rows, nil
}

func (a *App) channelRow(ch Channel, in channelInputs) StatusEntry {
	row := StatusEntry{Tool: ch.Tool, DestinationPath: ch.Native, IsChannel: true}
	r, err := a.render(ch, in)
	if err != nil {
		row.State, row.Detail = "error", err.Error()
		return row
	}
	state, detail, err := a.observe(ch, r)
	if err != nil {
		row.State, row.Detail = "error", err.Error()
		return row
	}
	// A stale index matters only for a channel sync would deliver; one
	// that cannot mount says why instead (at global scope its index is
	// never written).
	if ch.Mountable && state != ChannelConflict && state != ChannelManual && state != ChannelStale {
		if cur, err := os.ReadFile(ch.Index); err != nil || !bytes.Equal(cur, r.Bytes) {
			state, detail = ChannelStale, a.display(ch.Index)+" is out of date; run "+indexCommand(ch.Scope)
		}
	}
	row.State, row.Detail = string(state), detail
	if state == ChannelSynced {
		row.Detail = a.mountLabel(ch)
	}
	return row
}

// indexCommand is the command that rewrites a channel's index file.
func indexCommand(scope Scope) string {
	if scope == ScopeGlobal {
		return "`sync-agents global sync`"
	}
	return "`sync-agents index`"
}

// printChannelRows prints the "Delivery channels" section of `status`:
// one row per channel, in the states channelRows reports.
func (a *App) printChannelRows() error {
	rows, err := a.channelRows(ChannelRun{Scope: ScopeLocal, Explicit: a.explicitTargets()})
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
