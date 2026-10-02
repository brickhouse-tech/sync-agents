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
