package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// GlobalConfigContent is the canonical body of ~/.agents/config that
// CmdGlobalInit writes when no config file is present.
//
// The target list is fixed at five tools (claude, codeium, cursor,
// copilot, codex). It is intentionally not derived from the local
// .agents/config: the global config should not change based on which
// project happens to be the cwd when `global init` runs. SPEC-002
// AC-11 codifies this independence.
//
// The header lines mirror the comment style of the project-level
// .agents/config so users who have seen one recognize the other.
const GlobalConfigContent = `# sync-agents global configuration
# Comma-separated list of sync targets (available: claude, codeium, cursor, copilot, codex)
# Note: 'codeium' is the user-scope name for Windsurf; the project-scope dir is .windsurf/
# Override per-command with: sync-agents global sync --targets claude,cursor
targets = claude,codeium,cursor,copilot,codex
`

// CmdGlobalInit initializes the user's global .agents/ tree.
//
// Behavior (per SPEC-002 §Requirement: Global agents directory):
//
//   - Creates ~/.agents/rules/, ~/.agents/skills/, ~/.agents/workflows/
//     if missing. MkdirAll is used so any of the dirs already existing
//     is fine.
//   - Writes ~/.agents/config with the canonical global target list
//     if no config file is present. An existing config is left
//     untouched, even one with a different target list — `global init`
//     does not migrate config; that is the user's call.
//   - Idempotent: running twice in a row leaves the filesystem in the
//     same state and exits 0.
//
// The "global root" is resolved via App.ResolveGlobalRoot (see
// globalroot.go), which respects $SYNC_AGENTS_GLOBAL_ROOT, the
// --global-root flag, and the programmatic App.GlobalRoot field for
// tests. A.DryRun causes a plan to print but no filesystem changes.
//
// Returns the first error encountered. Partial state may remain on
// disk if a mkdir or write fails — recovery is to re-run `global
// init`, which will fill in whatever is missing.
//
// See docs/commands/global-init.md for the user-facing documentation.
func (a *App) CmdGlobalInit() error {
	root := a.ResolveGlobalRoot()
	a.Info(fmt.Sprintf("Initializing global .agents/ at %s", root))

	subdirs := []string{"rules", "skills", "workflows"}
	for _, sub := range subdirs {
		path := filepath.Join(root, sub)
		if a.DryRun {
			a.Info(fmt.Sprintf("[dry-run] would ensure dir %s", path))
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			a.Error(fmt.Sprintf("failed to create %s: %v", path, err))
			return err
		}
	}

	configPath := filepath.Join(root, "config")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if a.DryRun {
			a.Info(fmt.Sprintf("[dry-run] would write config to %s", configPath))
		} else {
			if err := os.WriteFile(configPath, []byte(GlobalConfigContent), 0o644); err != nil {
				a.Error(fmt.Sprintf("failed to write %s: %v", configPath, err))
				return err
			}
			a.Info(fmt.Sprintf("Created %s", configPath))
		}
	} else {
		a.Warn(fmt.Sprintf("%s already exists, leaving untouched", configPath))
	}

	if !a.DryRun {
		a.Info("Global initialization complete.")
	}
	return nil
}
