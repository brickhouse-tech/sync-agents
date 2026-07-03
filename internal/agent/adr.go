package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ADR status directories under .agents/adrs/ (SPEC-004 Part F). The
// directory an ADR lives in is the source of truth for its status;
// the frontmatter `status:` key is kept in sync as a convenience for
// tools that read the file in isolation.
const (
	ADRStatusProposed = "proposed"
	ADRStatusAccepted = "accepted"
	ADRStatusDenied   = "denied"
)

// ADRStatuses lists the recognised status directories in display
// order (accepted first in the index; proposed is where new records
// start; denied is retained but never indexed).
var ADRStatuses = []string{ADRStatusAccepted, ADRStatusProposed, ADRStatusDenied}

// CmdADR transitions an ADR between statuses: `sync-agents adr
// <accept|deny|propose> <name>`. The record file moves between
// status subdirectories, its frontmatter `status:` is updated, and
// the AGENTS.md index is regenerated (which is what actually
// adds/removes it from agent-visible context).
func (a *App) CmdADR(action, name string) error {
	var target string
	switch action {
	case "accept", "accepted":
		target = ADRStatusAccepted
	case "deny", "denied", "reject":
		target = ADRStatusDenied
	case "propose", "proposed":
		target = ADRStatusProposed
	default:
		a.Error(fmt.Sprintf("Usage: sync-agents adr <accept|deny|propose> <name> (got action %q)", action))
		return fmt.Errorf("unknown adr action")
	}
	if name == "" {
		a.Error("Usage: sync-agents adr <accept|deny|propose> <name>")
		return fmt.Errorf("missing name")
	}

	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}

	name = strings.TrimSuffix(name, ".md")
	src, currentStatus, err := a.findADR(name)
	if err != nil {
		return err
	}
	if currentStatus == target {
		a.Info(fmt.Sprintf("ADR %q is already %s", name, target))
		return nil
	}

	rel := strings.TrimPrefix(src, filepath.Join(a.ProjectRoot, ".agents", "adrs", currentStatus)+string(filepath.Separator))
	dst := filepath.Join(a.ProjectRoot, ".agents", "adrs", target, rel)

	if a.DryRun {
		a.Info(fmt.Sprintf("[dry-run] would move %s -> %s and set status: %s", src, dst, target))
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		a.Error(fmt.Sprintf("destination already exists: %s", dst))
		return fmt.Errorf("destination exists")
	}
	if err := os.Rename(src, dst); err != nil {
		return err
	}

	// Keep the frontmatter status label in sync with the directory.
	// Failure here is a warning, not an error — the move (the source
	// of truth) already happened.
	if raw, err := os.ReadFile(dst); err == nil {
		if block, perr := parseFMBlock(string(raw)); perr == nil && block.present {
			if cur, _ := block.get("status"); cur != target {
				block.set("status", target)
				if werr := writeFileAtomic(dst, []byte(block.render()), 0o644); werr != nil {
					a.Warn(fmt.Sprintf("could not update status frontmatter: %v", werr))
				}
			}
		}
	}

	a.Info(fmt.Sprintf("ADR %q: %s -> %s", name, currentStatus, target))
	a.generateAgentsMD()
	a.Info("Updated AGENTS.md index")
	return nil
}

// findADR locates <name>.md under any status directory (including
// nested grouping subdirs) and returns its path + current status.
// Ambiguity (same name under two statuses) is an error the user must
// resolve by hand — guessing would silently pick a record.
func (a *App) findADR(name string) (string, string, error) {
	type match struct {
		path   string
		status string
	}
	var matches []match
	for _, status := range ADRStatuses {
		dir := filepath.Join(a.ProjectRoot, ".agents", "adrs", status)
		files, _ := listMDFilesRecursive(dir)
		for _, rel := range files {
			if rel == name || strings.HasSuffix(rel, "/"+name) {
				matches = append(matches, match{
					path:   filepath.Join(dir, filepath.FromSlash(rel)+".md"),
					status: status,
				})
			}
		}
	}
	switch len(matches) {
	case 0:
		a.Error(fmt.Sprintf("ADR %q not found under .agents/adrs/{%s}/", name, strings.Join(ADRStatuses, ",")))
		return "", "", fmt.Errorf("adr not found")
	case 1:
		return matches[0].path, matches[0].status, nil
	default:
		var where []string
		for _, m := range matches {
			where = append(where, m.path)
		}
		a.Error(fmt.Sprintf("ADR %q is ambiguous:\n  %s", name, strings.Join(where, "\n  ")))
		return "", "", fmt.Errorf("ambiguous adr name")
	}
}
