package agent

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/brickhouse-tech/sync-agents/internal/agent/source"
	"github.com/brickhouse-tech/sync-agents/internal/agent/templates"
	"github.com/brickhouse-tech/sync-agents/internal/version"
)

var AllTargets = []string{"claude", "windsurf", "cursor", "copilot"}

// App is the per-invocation state container for sync-agents commands.
// One App is constructed by main(), populated from CLI flags in
// PersistentPreRunE, then handed to each command's library function.
//
// Fields are intentionally a flat set of values — no embedded
// configuration objects — so that test setup is a one-shot literal
// (e.g., &App{ProjectRoot: t.TempDir(), GlobalRoot: t.TempDir() + "/.agents"})
// without builders or option-functions.
type App struct {
	// ProjectRoot is the absolute path of the project's working tree
	// — the directory containing `.agents/`. Resolved by
	// FindProjectRoot at startup, or overridden by the --dir flag.
	ProjectRoot string

	// GlobalRoot is the absolute path of the user's `.agents/` tree at
	// user scope, when overridden programmatically. Empty means
	// "consult $SYNC_AGENTS_GLOBAL_ROOT or fall back to $HOME/.agents"
	// — see ResolveGlobalRoot in globalroot.go for the full precedence
	// chain, and SPEC-002 §Configurable global root for the
	// requirement.
	//
	// Tests set this directly to a t.TempDir-backed path so they never
	// touch the real $HOME. The CLI populates it from --global-root.
	GlobalRoot string

	// DryRun, when true, prints the operations that would be performed
	// but does not modify the filesystem.
	DryRun bool

	// Force, when true, lets commands proceed past a safety check
	// that would otherwise stop them: add overwrites an existing
	// artifact, promote/approve accept critical scan findings, global
	// sync moves a conflicting file aside. For the project-scope link
	// commands (sync, fix) it is a deprecated alias of Overwrite — see
	// deprecateForce in bucketlink.go — and never deletes anything.
	Force bool

	// Overwrite, when true, lets sync and fix move a real file or
	// directory that shadows a claimed artifact aside to
	// <path>.replaced-by-sync-agents before placing the symlink.
	// Nothing is ever deleted (SPEC-010 §Phase 3). Without it, such a
	// path is reported as a conflict and left in place.
	Overwrite bool

	// ActiveTargets is the list of tool IDs the current command should
	// touch. Populated from .agents/config and overridden by the
	// --targets flag.
	ActiveTargets []string

	// Stdout and Stderr are the writers used by Info/Warn/Error. Tests
	// inject bytes.Buffer here to assert on output without capturing
	// the real stdio.
	Stdout io.Writer
	Stderr io.Writer

	// ToolEnv is what tool resolvers (OpenClaw's workspace lookup)
	// read. NewApp installs the real environment; the zero value sees
	// no environment variables, so test literals stay inside their temp
	// global root.
	ToolEnv ToolEnv
}

func NewApp() *App {
	return &App{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		ToolEnv: OSToolEnv(),
	}
}

func (a *App) Info(msg string)  { fmt.Fprintf(a.Stdout, "[info] %s\n", msg) }
func (a *App) Warn(msg string)  { fmt.Fprintf(a.Stdout, "[warn] %s\n", msg) }
func (a *App) Error(msg string) { fmt.Fprintf(a.Stderr, "[error] %s\n", msg) }

func FindProjectRoot(startDir string) string {
	if startDir == "" {
		startDir = "."
	}
	dir, err := filepath.Abs(startDir)
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".agents")); err == nil && fi.IsDir() {
			return dir
		}
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	wd, _ := os.Getwd()
	return wd
}

func (a *App) EnsureAgentsDir() error {
	path := filepath.Join(a.ProjectRoot, ".agents")
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		a.Error(".agents/ directory not found. Run 'sync-agents init' first.")
		return fmt.Errorf("no agents dir")
	}
	return nil
}

func ResolveTargetDir(target, root string) string {
	if target == "copilot" {
		return filepath.Join(root, ".github", "copilot")
	}
	return filepath.Join(root, "."+target)
}

// dropGlobalOnlyTargets removes every registered tool with no project
// scope (openclaw) from ActiveTargets, warning once per tool. Local
// commands build the target dir as <project>/.<target>, so without
// this a `--targets openclaw` would create .openclaw/ inside the repo.
// Unregistered names (wave, ...) pass through untouched.
func (a *App) dropGlobalOnlyTargets() {
	var kept []string
	for _, t := range a.ActiveTargets {
		if tool, ok := ResolveTool(t); ok && !tool.HasScope(ScopeLocal) {
			a.Warn(fmt.Sprintf("target %q is global-only; skipping for project scope (use `sync-agents global sync --targets %s`)", t, tool.ID))
			continue
		}
		kept = append(kept, t)
	}
	a.ActiveTargets = kept
}

func ResolveAgentsRel(target string) string {
	if target == "copilot" {
		return "../../.agents"
	}
	return "../.agents"
}

func ReadConfigTargets(projectRoot string) []string {
	configFile := filepath.Join(projectRoot, ".agents", "config")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return copyTargets(AllTargets)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "targets" {
			val := strings.TrimSpace(parts[1])
			var result []string
			for _, t := range strings.Split(val, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					result = append(result, t)
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}
	return copyTargets(AllTargets)
}

func copyTargets(t []string) []string {
	r := make([]string, len(t))
	copy(r, t)
	return r
}

// CreateSymlink places the symlink target -> source, creating parent
// directories as needed. A symlink already at target is a no-op when
// it resolves to source and is replaced otherwise. A real file or
// directory at target is never deleted: without App.Overwrite the
// call returns ErrConflict and leaves it alone; with App.Overwrite it
// is renamed to a BackupSuffix sibling first. Under dryRun the call
// prints "would link"/"would move aside" and writes nothing (but
// still returns ErrConflict so dry-run reports the same conflicts a
// real run would).
func (a *App) CreateSymlink(source, target string, dryRun bool) error {
	_, err := a.placeLink(source, target, dryRun)
	return err
}

func (a *App) PrintTree(dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	count := len(names)
	for i, name := range names {
		connector := "├── "
		childPrefix := "│   "
		if i == count-1 {
			connector = "└── "
			childPrefix = "    "
		}
		fullPath := filepath.Join(dir, name)
		fi, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			linkTarget, _ := os.Readlink(fullPath)
			fmt.Fprintf(a.Stdout, "%s%s%s -> %s\n", prefix, connector, name, linkTarget)
		} else if fi.IsDir() {
			fmt.Fprintf(a.Stdout, "%s%s%s/\n", prefix, connector, name)
			a.PrintTree(fullPath, prefix+childPrefix)
		} else {
			fmt.Fprintf(a.Stdout, "%s%s%s\n", prefix, connector, name)
		}
	}
}

// -------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------

func (a *App) CmdInit() error {
	a.Info("Initializing .agents/ directory structure...")

	agentsDir := filepath.Join(a.ProjectRoot, ".agents")
	for _, sub := range InitBucketDirs() {
		os.MkdirAll(filepath.Join(agentsDir, sub), 0755)
	}

	stateRule := filepath.Join(agentsDir, "rules", "state.md")
	if _, err := os.Stat(stateRule); os.IsNotExist(err) {
		os.WriteFile(stateRule, []byte(templates.State()), 0644)
		a.Info("Created .agents/rules/state.md from template")
	} else {
		a.Warn(".agents/rules/state.md already exists, skipping")
	}

	// Migrate legacy STATE.md
	legacyState := filepath.Join(agentsDir, "STATE.md")
	if _, err := os.Stat(legacyState); err == nil {
		a.migrateLegacyState(agentsDir)
	}

	// Config
	configFile := filepath.Join(agentsDir, "config")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		content := "# sync-agents configuration\n# Comma-separated list of sync targets (available: claude, windsurf, cursor, copilot)\n# Override per-command with: sync-agents sync --targets claude,cursor\ntargets = claude,windsurf,cursor,copilot\n"
		os.WriteFile(configFile, []byte(content), 0644)
		a.Info("Created .agents/config")
	} else {
		a.Warn(".agents/config already exists, skipping")
	}

	// AGENTS.md is the user's file (SPEC-013): init writes a short stub
	// only when there is none, and never touches an existing one.
	agentsMD := filepath.Join(a.ProjectRoot, "AGENTS.md")
	if _, err := os.Lstat(agentsMD); os.IsNotExist(err) {
		if err := os.WriteFile(agentsMD, []byte(AgentsMDStub), 0o644); err != nil {
			a.Error(fmt.Sprintf("write AGENTS.md: %v", err))
			return err
		}
		a.Info("Created AGENTS.md")
	} else {
		a.Warn("AGENTS.md already exists, skipping")
	}

	a.addDefaultGitignoreEntries()

	a.Info("Initialization complete. Directory structure:")
	a.PrintTree(agentsDir, "")
	return nil
}

// CmdAdd creates a new artifact in the canonical tree.
//
// Two modes (SPEC-011 Part C):
//
//   - Default: scaffold from the bucket's template, with ${NAME}
//     substituted.
//   - opts.From set: seed from an artifact that already exists
//     elsewhere, either as a normalized copy or, with opts.Link, as a
//     symlink that leaves the source owning its content.
//
// Both modes end at the same canonical path, so nothing downstream
// needs to know which one ran.
func (a *App) CmdAdd(typ, name string, opts AddOpts) error {
	if typ == "" || name == "" {
		a.Error(fmt.Sprintf("Usage: sync-agents add <%s> <name>", strings.Join(ArtifactNames(), "|")))
		return fmt.Errorf("missing args")
	}
	if opts.Link && opts.From == "" {
		a.Error("--link requires --from: there is nothing to link to when scaffolding from a template")
		return fmt.Errorf("link without from")
	}

	bucket, ok := BucketForTypeString(typ)
	if !ok || bucket.NewTemplate == nil {
		a.Error(fmt.Sprintf("Unknown type: %s. Must be one of: %s", typ, strings.Join(ArtifactNames(), ", ")))
		return fmt.Errorf("unknown type")
	}
	typ = bucket.Dir

	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}

	var fpath string
	if bucket.DirPerArtifact {
		fpath = filepath.Join(a.ProjectRoot, ".agents", typ, name, "SKILL.md")
	} else if bucket.NewSubdir != "" {
		fpath = filepath.Join(a.ProjectRoot, ".agents", typ, bucket.NewSubdir, name+bucket.FileExt())
	} else {
		fpath = filepath.Join(a.ProjectRoot, ".agents", typ, name+bucket.FileExt())
	}

	// Lstat, not Stat: a dangling symlink at the canonical path is
	// still something the user put there, and reporting "does not
	// exist" before overwriting it would be a lie.
	if _, err := os.Lstat(fpath); err == nil && !a.Force {
		a.Error(fmt.Sprintf("File already exists: %s (use --force to overwrite)", fpath))
		return fmt.Errorf("exists")
	}

	if opts.From != "" {
		srcPath, err := a.resolveAddSource(opts.From)
		if err != nil {
			a.Error(err.Error())
			return err
		}
		if opts.Link {
			if err := a.importByLink(srcPath, fpath, name, bucket); err != nil {
				a.Error(err.Error())
				return err
			}
			a.Info(fmt.Sprintf("Linked %s: %s -> %s", typ, fpath, srcPath))
		} else {
			if err := a.importByCopy(srcPath, fpath, name, bucket); err != nil {
				a.Error(err.Error())
				return err
			}
			a.Info(fmt.Sprintf("Imported %s: %s (from %s)", typ, fpath, srcPath))
		}
		a.migrateAgentsMDOrWarn()
		return nil
	}

	content := strings.ReplaceAll(bucket.NewTemplate(), "${NAME}", name)

	os.MkdirAll(filepath.Dir(fpath), 0755)
	os.WriteFile(fpath, []byte(content), 0644)
	a.Info(fmt.Sprintf("Created %s: %s", typ, fpath))

	a.migrateAgentsMDOrWarn()
	return nil
}

func (a *App) CmdSync() error {
	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}
	a.dropGlobalOnlyTargets()

	a.deprecateForce()

	a.Info("Syncing .agents/ to agent directories...")
	a.migrateAgentsMDOrWarn()

	conflicts := 0
	for _, target := range a.ActiveTargets {
		targetDir := ResolveTargetDir(target, a.ProjectRoot)
		agentsRel := ResolveAgentsRel(target)

		relDisplay := targetDir
		if strings.HasPrefix(targetDir, a.ProjectRoot+"/") {
			relDisplay = targetDir[len(a.ProjectRoot)+1:]
		}
		a.Info(fmt.Sprintf("Syncing to %s/", relDisplay))

		for _, b := range Buckets {
			if !b.SyncsToTool(target) {
				continue
			}
			subdirPath := filepath.Join(a.ProjectRoot, ".agents", b.Dir)
			if fi, err := os.Stat(subdirPath); err == nil && fi.IsDir() {
				conflicts += a.linkBucket(targetDir, agentsRel, b).Conflicts
			}
		}
	}

	// CLAUDE.md follows the SPEC-013 policy (claudemd.go). A
	// hand-written CLAUDE.md is warned about but kept out of the exit
	// status: it is common and harmless, and failing every such project
	// would teach users to ignore the conflict exit.
	claudeMD := a.claudeMDDecision()
	if _, err := a.applyClaudeMD(claudeMD); err != nil {
		a.Warn(err.Error())
	}

	// Hooks (SPEC-004 Part C): merge .agents/hooks/*.json fragments
	// into .claude/settings.json. This runs after symlink creation
	// because hooks are a JSON merge, not a directory symlink.
	if a.isBucketActive("claude") {
		hooksDir := filepath.Join(a.ProjectRoot, ".agents", "hooks")
		settingsPath := filepath.Join(a.ProjectRoot, ".claude", "settings.json")
		statePath := filepath.Join(a.ProjectRoot, ".agents", ".sync", "claude-hooks-state.json")
		if a.DryRun {
			if _, err := os.Stat(hooksDir); err == nil {
				a.Info(fmt.Sprintf("[dry-run] would merge hooks into %s", settingsPath))
			}
		} else {
			n, err := a.MergeHooks(hooksDir, settingsPath, statePath)
			if err != nil {
				a.Warn(fmt.Sprintf("hooks merge: %v", err))
			} else if n > 0 {
				a.Info(fmt.Sprintf("merged %d hook(s) into %s", n, settingsPath))
			}
		}
	}

	a.updateGitignore(claudeMD)

	if conflicts > 0 {
		a.Warn(fmt.Sprintf("Sync finished with %d conflict(s); nothing was deleted", conflicts))
		return fmt.Errorf("%d conflict(s)", conflicts)
	}
	a.Info("Sync complete.")
	return nil
}

func (a *App) CmdStatus() error {
	a.dropGlobalOnlyTargets()
	fmt.Fprintf(a.Stdout, "sync-agents v%s\n", version.Version)
	fmt.Fprintln(a.Stdout)

	agentsDir := filepath.Join(a.ProjectRoot, ".agents")
	if fi, err := os.Stat(agentsDir); err == nil && fi.IsDir() {
		fmt.Fprintf(a.Stdout, "[ok] .agents/ exists\n")
		a.PrintTree(agentsDir, "")
	} else {
		fmt.Fprintf(a.Stdout, "[missing] .agents/ not found\n")
	}

	fmt.Fprintln(a.Stdout)

	agentsMD := filepath.Join(a.ProjectRoot, "AGENTS.md")
	if _, err := os.Stat(agentsMD); err == nil {
		fmt.Fprintf(a.Stdout, "[ok] AGENTS.md exists\n")
	} else {
		fmt.Fprintf(a.Stdout, "[missing] AGENTS.md not found\n")
	}

	fmt.Fprintln(a.Stdout, a.claudeMDDecision().statusLine())

	fmt.Fprintln(a.Stdout)

	for _, target := range a.statusTargets() {
		targetDir := ResolveTargetDir(target, a.ProjectRoot)
		displayDir := targetDir
		if strings.HasPrefix(targetDir, a.ProjectRoot+"/") {
			displayDir = targetDir[len(a.ProjectRoot)+1:]
		}

		rulesLink := filepath.Join(targetDir, "rules")
		hasDirOrLinks := false
		if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
			hasDirOrLinks = true
		}
		if fi, err := os.Lstat(rulesLink); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			hasDirOrLinks = true
		}

		if hasDirOrLinks {
			fmt.Fprintf(a.Stdout, "%s/\n", displayDir)
			for _, b := range Buckets {
				if !b.SyncsToTool(target) {
					continue
				}
				sub := filepath.Join(targetDir, b.Dir)
				sfi, serr := os.Lstat(sub)
				if serr == nil && sfi.Mode()&os.ModeSymlink != 0 {
					lt, _ := os.Readlink(sub)
					fmt.Fprintf(a.Stdout, "  [synced] %s -> %s\n", b.Dir, lt)
				} else if serr == nil && sfi.IsDir() {
					stats := bucketMergeStats(sub, filepath.Join(a.ProjectRoot, ".agents", b.Dir))
					conflictNote := ""
					if stats.Conflicts > 0 {
						conflictNote = fmt.Sprintf(", %d conflict(s)", stats.Conflicts)
					}
					if stats.Linked >= 1 {
						fmt.Fprintf(a.Stdout, "  [merged] %s (%d/%d linked%s)\n", b.Dir, stats.Linked, stats.Total, conflictNote)
					} else {
						fmt.Fprintf(a.Stdout, "  [local] %s (not symlinked%s)\n", b.Dir, conflictNote)
					}
				} else if b.InInit {
					// Optional buckets (agents/…) are only reported
					// when something exists for them; the classic
					// three always show, matching pre-registry output.
					fmt.Fprintf(a.Stdout, "  [missing] %s\n", b.Dir)
				} else if fi, err := os.Stat(filepath.Join(a.ProjectRoot, ".agents", b.Dir)); err == nil && fi.IsDir() {
					fmt.Fprintf(a.Stdout, "  [missing] %s\n", b.Dir)
				}
			}
		} else {
			fmt.Fprintf(a.Stdout, "[not synced] %s/\n", displayDir)
		}
	}
	return nil
}

// statusTargets is AllTargets plus any configured extra target (such
// as wave), so a directory sync merges into is never missing from
// status.
func (a *App) statusTargets() []string {
	targets := copyTargets(AllTargets)
	for _, t := range a.ActiveTargets {
		known := false
		for _, k := range targets {
			if k == t {
				known = true
				break
			}
		}
		if !known {
			targets = append(targets, t)
		}
	}
	return targets
}

// CmdIndex runs the one-time AGENTS.md migration (SPEC-013). It no
// longer writes a link index: no tool followed those links, and
// AGENTS.md now belongs to the user. main.go runs the skill
// frontmatter backfill before it unless --no-fix is given.
func (a *App) CmdIndex() error {
	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}
	if _, err := a.migrateProjectAgentsMD(); err != nil {
		a.Error(fmt.Sprintf("AGENTS.md migration: %v", err))
		return err
	}
	return nil
}

func (a *App) CmdClean() error {
	a.dropGlobalOnlyTargets()
	a.Info("Removing synced symlinks...")

	for _, target := range a.ActiveTargets {
		targetDir := ResolveTargetDir(target, a.ProjectRoot)
		displayDir := targetDir
		if strings.HasPrefix(targetDir, a.ProjectRoot+"/") {
			displayDir = targetDir[len(a.ProjectRoot)+1:]
		}

		for _, subdir := range BucketDirs() {
			sub := filepath.Join(targetDir, subdir)
			fi, err := os.Lstat(sub)
			if err == nil && fi.Mode()&os.ModeSymlink != 0 {
				os.Remove(sub)
				a.Info(fmt.Sprintf("Removed: %s/%s", displayDir, subdir))
			}
		}

		if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
			entries, _ := os.ReadDir(targetDir)
			if len(entries) == 0 {
				os.Remove(targetDir)
				a.Info(fmt.Sprintf("Removed empty directory: %s/", displayDir))
			}
		}
	}

	// Only our CLAUDE.md -> AGENTS.md link is removed; a real file or
	// a symlink pointing elsewhere is the user's.
	if state, err := classifyClaudeMD(a.ProjectRoot); err != nil {
		a.Warn(fmt.Sprintf("CLAUDE.md: %v", err))
	} else if state == ClaudeMDOurLink {
		if err := os.Remove(filepath.Join(a.ProjectRoot, "CLAUDE.md")); err != nil {
			a.Warn(fmt.Sprintf("remove CLAUDE.md symlink: %v", err))
		} else {
			a.Info("Removed: CLAUDE.md symlink")
		}
	}

	a.Info("Clean complete.")
	return nil
}

func (a *App) CmdWatch() error {
	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}

	watchDir := filepath.Join(a.ProjectRoot, ".agents")

	if _, err := exec.LookPath("fswatch"); err == nil {
		a.Info("Watching .agents/ for changes... (Ctrl+C to stop)")
		a.CmdIndex()
		cmd := exec.Command("fswatch", "-o", watchDir)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			a.Info("Change detected, regenerating index...")
			a.CmdIndex()
		}
		return cmd.Wait()
	}

	if _, err := exec.LookPath("inotifywait"); err == nil {
		a.Info("Watching .agents/ for changes... (Ctrl+C to stop)")
		a.CmdIndex()
		cmd := exec.Command("inotifywait", "-m", "-r", "-e", "modify,create,delete,move", "--format", "%w%f", watchDir)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			a.Info("Change detected, regenerating index...")
			a.CmdIndex()
		}
		return cmd.Wait()
	}

	a.Error("Neither fswatch (macOS) nor inotifywait (Linux) found.")
	a.Error("Install with: brew install fswatch  OR  apt install inotify-tools")
	return fmt.Errorf("no watcher")
}

func (a *App) CmdImport(url string, trust bool) error {
	if url == "" {
		a.Error("Usage: sync-agents import <url>")
		return fmt.Errorf("missing url")
	}

	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}

	filename := filepath.Base(url)
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}

	var typ string
	for _, b := range Buckets {
		if strings.Contains(url, "/"+b.Dir+"/") {
			typ = b.Dir
			break
		}
	}

	if typ == "" {
		fmt.Fprintln(a.Stdout, "Could not detect type from URL. Choose:")
		for i, b := range Buckets {
			fmt.Fprintf(a.Stdout, "  %d) %s\n", i+1, b.Artifact)
		}
		fmt.Fprintf(a.Stdout, "Selection (1-%d): ", len(Buckets))
		var choice string
		fmt.Scanln(&choice)
		idx, err := strconv.Atoi(choice)
		if err != nil || idx < 1 || idx > len(Buckets) {
			a.Error("Invalid selection")
			return fmt.Errorf("invalid selection")
		}
		typ = Buckets[idx-1].Dir
	}

	destRel := typ + "/" + filename
	dest := filepath.Join(a.ProjectRoot, ".agents", typ, filename)

	a.Info(fmt.Sprintf("Importing %s → .agents/%s/%s", url, typ, filename))

	// Native fetch (SPEC-003 rollout step 4): no curl subprocess, so
	// import works in minimal containers. file:// stays supported —
	// the bats suite and local workflows depend on it.
	data, err := fetchImportURL(url)
	if err != nil {
		a.Error(fmt.Sprintf("Failed to download: %s (%v)", url, err))
		return err
	}

	// SPEC-005 Part B: scan the fetched artifact and, by default, park
	// it in quarantine for review instead of dropping it into the live
	// tree. This closes the hole SPEC-005 names explicitly — `import`
	// used to write remote content straight into .agents/ with no scan
	// and no gate, while `pull` was gated. Now both route through the
	// same quarantine. `--trust` (or `quarantine = off` in config)
	// bypasses the gate, but the scan still runs and prints loudly.
	tmpDir, err := os.MkdirTemp("", "sync-import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	staged := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(staged, data, 0644); err != nil {
		return err
	}
	findings := source.ScanTree(staged)

	gate := ReadConfigQuarantine(filepath.Join(a.ProjectRoot, ".agents"))
	if trust {
		gate = false
	}

	if gate {
		origin, _ := a.importOrigin(url, staged) // zero Origin if not a GitHub URL → stays untracked
		p := a.sourcePuller(SourceCmdOpts{})
		if err := p.QuarantineImport(staged, destRel, origin, findings); err != nil {
			a.Error(fmt.Sprintf("Failed to quarantine: %v", err))
			return err
		}
		a.reportImportFindings(findings)
		name := strings.TrimSuffix(filename, filepath.Ext(filename))
		a.Info(fmt.Sprintf("Quarantined .agents/%s — review with `sync-agents quarantine`, then `sync-agents approve %s`.", destRel, name))
		return nil
	}

	// --trust (or quarantine disabled): install directly. The scan
	// still runs and any findings are printed — the bypass is loud,
	// never silent.
	if len(findings) > 0 {
		a.Warn("--trust: installing WITHOUT the quarantine gate — scan findings below")
		a.reportImportFindings(findings)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		a.Error(fmt.Sprintf("Failed to write: %s (%v)", dest, err))
		return err
	}

	// Best-effort provenance: when the URL is a raw.githubusercontent
	// file we can recover owner/repo/ref/path and record a manual
	// origin, which `source bundle` later converts into a manifest
	// entry. Plain URLs simply skip this — an artifact without origin
	// is valid and untracked (SPEC-003).
	a.writeImportOrigin(url, dest)

	a.Info("Imported successfully.")
	a.CmdIndex()
	return nil
}

// reportImportFindings prints scanner findings for an import in a
// stable, human-readable form. Silent when there are none.
func (a *App) reportImportFindings(findings []source.Finding) {
	if len(findings) == 0 {
		return
	}
	crit := 0
	for _, f := range findings {
		if f.Severity == source.SeverityCritical {
			crit++
		}
	}
	a.Info(fmt.Sprintf("scan: %d finding(s), %d CRITICAL", len(findings), crit))
	for _, f := range findings {
		loc := f.Path
		if loc == "" {
			loc = "artifact"
		}
		a.Info(fmt.Sprintf("  [%s] %s: %s (%s)", f.Severity, f.Class, f.Detail, loc))
	}
}

// fetchImportURL retrieves an import URL's content. https and file
// schemes only: plain http would silently ship artifacts over an
// unauthenticated channel, which is exactly the tampering surface the
// SPEC-003 integrity work exists to close.
func fetchImportURL(rawURL string) ([]byte, error) {
	switch {
	case strings.HasPrefix(rawURL, "file://"):
		return os.ReadFile(strings.TrimPrefix(rawURL, "file://"))
	case strings.HasPrefix(rawURL, "https://"):
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(rawURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		// Imports are single markdown/JSON artifacts; 10 MiB is far
		// beyond any legitimate one and bounds a hostile response.
		return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	case strings.HasPrefix(rawURL, "http://"):
		return nil, fmt.Errorf("plain http:// is not supported — use https:// (or file:// for local files)")
	default:
		return nil, fmt.Errorf("unsupported URL scheme — expected https:// or file://")
	}
}

// writeImportOrigin records manual-source provenance for imports from
// raw.githubusercontent.com. Failures only warn: origin metadata is
// an enhancement to import, never a reason for it to fail.
// importOrigin builds manual-source provenance for an import. ok is
// false (and the Origin zero) when the URL isn't a recoverable
// raw.githubusercontent file — such imports are valid but untracked.
func (a *App) importOrigin(rawURL, file string) (source.Origin, bool) {
	o, ok := originFromRawGitHubURL(rawURL)
	if !ok {
		return source.Origin{}, false
	}
	if h, err := source.HashTree(file); err == nil {
		o.ContentHash = h
	}
	o.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	o.Source = source.SourceManual
	return o, true
}

func (a *App) writeImportOrigin(rawURL, dest string) {
	o, ok := a.importOrigin(rawURL, dest)
	if !ok {
		return
	}
	if err := source.WriteOriginFor(dest, false, o); err != nil {
		a.Warn(fmt.Sprintf("could not write origin metadata for %s: %v", dest, err))
	}
}

// originFromRawGitHubURL parses
// https://raw.githubusercontent.com/<owner>/<repo>/<ref>/<path>
// (including the refs/heads/<branch> and refs/tags/<tag> long forms)
// into origin metadata. Returns ok=false for anything else.
func originFromRawGitHubURL(rawURL string) (source.Origin, bool) {
	const prefix = "https://raw.githubusercontent.com/"
	if !strings.HasPrefix(rawURL, prefix) {
		return source.Origin{}, false
	}
	parts := strings.Split(strings.TrimPrefix(rawURL, prefix), "/")
	if len(parts) < 4 {
		return source.Origin{}, false
	}
	owner, repo := parts[0], parts[1]
	var ref string
	var pathParts []string
	if parts[2] == "refs" && len(parts) >= 6 && (parts[3] == "heads" || parts[3] == "tags") {
		ref = parts[4]
		pathParts = parts[5:]
	} else {
		ref = parts[2]
		pathParts = parts[3:]
	}
	if owner == "" || repo == "" || ref == "" || len(pathParts) == 0 || pathParts[len(pathParts)-1] == "" {
		return source.Origin{}, false
	}
	o := source.Origin{Owner: owner, Repo: repo, Ref: ref, Path: strings.Join(pathParts, "/")}
	if source.IsCommitSHA(ref) {
		o.SHA = strings.ToLower(ref)
	}
	return o, true
}

func (a *App) CmdHook() error {
	gitDir := filepath.Join(a.ProjectRoot, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		a.Error("Not a git repository (no .git/ found).")
		return fmt.Errorf("not a git repo")
	}

	hookDir := filepath.Join(gitDir, "hooks")
	os.MkdirAll(hookDir, 0755)
	hookFile := filepath.Join(hookDir, "pre-commit")

	marker := "sync-agents start"

	if data, err := os.ReadFile(hookFile); err == nil {
		if strings.Contains(string(data), marker) {
			a.Info(fmt.Sprintf("Git hook already installed in %s", hookFile))
			return nil
		}
	}

	hookBlock := `
# --- sync-agents start ---
if command -v sync-agents >/dev/null 2>&1; then
  sync-agents sync 2>/dev/null
  sync-agents index 2>/dev/null
  git add AGENTS.md CLAUDE.md .claude/ .windsurf/ .cursor/ .github/copilot/ 2>/dev/null || true
fi
# --- sync-agents end ---
`

	if _, err := os.Stat(hookFile); err == nil {
		f, err := os.OpenFile(hookFile, os.O_APPEND|os.O_WRONLY, 0755)
		if err != nil {
			return err
		}
		f.WriteString(hookBlock)
		f.Close()
		a.Info(fmt.Sprintf("Appended sync-agents hook to existing %s", hookFile))
	} else {
		content := "#!/bin/sh\n" + hookBlock + "\n"
		os.WriteFile(hookFile, []byte(content), 0755)
		a.Info(fmt.Sprintf("Created git hook: %s", hookFile))
	}
	return nil
}

func (a *App) CmdFix(fixType string, noClobber bool) error {
	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}
	a.dropGlobalOnlyTargets()
	a.deprecateForce()
	a.migrateAgentsMDOrWarn()

	var subdirs []string
	if fixType == "all" || fixType == "" {
		subdirs = BucketDirs()
	} else if b, ok := BucketForDir(fixType); ok {
		subdirs = []string{b.Dir}
	} else {
		a.Error(fmt.Sprintf("Unknown type: %s (expected: %s, or all)", fixType, strings.Join(BucketDirs(), ", ")))
		return fmt.Errorf("unknown type")
	}

	agentsAbs, _ := filepath.Abs(filepath.Join(a.ProjectRoot, ".agents"))
	fixed := 0
	skipped := 0
	merged := 0

	// Phase 1: Migrate legacy dirs
	for _, subdir := range subdirs {
		legacyDir := filepath.Join(a.ProjectRoot, subdir)
		agentsSubdir := filepath.Join(agentsAbs, subdir)

		fi, err := os.Lstat(legacyDir)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			a.Info(fmt.Sprintf("%s/ is already a symlink — nothing to do.", subdir))
			continue
		}
		if !fi.IsDir() {
			continue
		}

		// Check same inode
		if sameInode(legacyDir, agentsSubdir) {
			a.Warn(fmt.Sprintf("%s/ and .agents/%s/ are the same directory (same inode).", subdir, subdir))
			a.Warn(fmt.Sprintf("Replacing %s/ with a symlink to .agents/%s/.", subdir, subdir))
			if a.DryRun {
				fmt.Fprintf(a.Stdout, "  would remove %s/ (same inode as .agents/%s/)\n", subdir, subdir)
				fmt.Fprintf(a.Stdout, "  would create symlink %s/ -> .agents/%s\n", subdir, subdir)
			} else {
				os.RemoveAll(legacyDir)
				os.Symlink(".agents/"+subdir, legacyDir)
				a.Info(fmt.Sprintf("Replaced %s/ with symlink -> .agents/%s", subdir, subdir))
			}
			fixed++
			continue
		}

		a.Info(fmt.Sprintf("Found legacy directory: %s/", subdir))
		os.MkdirAll(agentsSubdir, 0755)

		// Move directories
		dirEntries, _ := os.ReadDir(legacyDir)
		for _, entry := range dirEntries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			dest := filepath.Join(agentsSubdir, name)

			if _, err := os.Stat(dest); err == nil {
				if noClobber {
					a.Warn(fmt.Sprintf("Skipping %s/%s — already exists in .agents/%s/ (--no-clobber)", subdir, name, subdir))
					skipped++
					continue
				}
				if a.DryRun {
					fmt.Fprintf(a.Stdout, "  would merge: %s/%s -> .agents/%s/%s (overwrite)\n", subdir, name, subdir, name)
				} else {
					os.RemoveAll(dest)
					os.Rename(filepath.Join(legacyDir, name), dest)
					a.Info(fmt.Sprintf("Merged: %s/%s -> .agents/%s/%s (overwrote existing)", subdir, name, subdir, name))
				}
				merged++
				fixed++
				continue
			}

			if a.DryRun {
				fmt.Fprintf(a.Stdout, "  would move: %s/%s -> .agents/%s/%s\n", subdir, name, subdir, name)
			} else {
				os.Rename(filepath.Join(legacyDir, name), dest)
				a.Info(fmt.Sprintf("Moved: %s/%s -> .agents/%s/%s", subdir, name, subdir, name))
			}
			fixed++
		}

		// Move files
		dirEntries, _ = os.ReadDir(legacyDir)
		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			dest := filepath.Join(agentsSubdir, name)

			if _, err := os.Stat(dest); err == nil {
				if noClobber {
					a.Warn(fmt.Sprintf("Skipping %s/%s — already exists in .agents/%s/ (--no-clobber)", subdir, name, subdir))
					skipped++
					continue
				}
				if a.DryRun {
					fmt.Fprintf(a.Stdout, "  would merge: %s/%s -> .agents/%s/%s (overwrite)\n", subdir, name, subdir, name)
				} else {
					os.Rename(filepath.Join(legacyDir, name), dest)
					a.Info(fmt.Sprintf("Merged: %s/%s -> .agents/%s/%s (overwrote existing)", subdir, name, subdir, name))
				}
				merged++
				fixed++
				continue
			}

			if a.DryRun {
				fmt.Fprintf(a.Stdout, "  would move: %s/%s -> .agents/%s/%s\n", subdir, name, subdir, name)
			} else {
				os.Rename(filepath.Join(legacyDir, name), dest)
				a.Info(fmt.Sprintf("Moved: %s/%s -> .agents/%s/%s", subdir, name, subdir, name))
			}
			fixed++
		}

		// Replace legacy dir with symlink
		if a.DryRun {
			fmt.Fprintf(a.Stdout, "  would replace %s/ with symlink -> .agents/%s\n", subdir, subdir)
		} else {
			remaining, _ := os.ReadDir(legacyDir)
			if len(remaining) == 0 {
				os.Remove(legacyDir)
				os.Symlink(".agents/"+subdir, legacyDir)
				a.Info(fmt.Sprintf("Replaced %s/ with symlink -> .agents/%s", subdir, subdir))
			} else {
				a.Warn(fmt.Sprintf("%s/ is not empty after migration — skipping symlink replacement", subdir))
			}
		}
	}

	// Phase 1b: Convert flat skill files to directory layout
	for _, subdir := range subdirs {
		if subdir != "skills" {
			continue
		}
		skillsDir := filepath.Join(agentsAbs, "skills")
		if _, err := os.Stat(skillsDir); err != nil {
			continue
		}

		entries, _ := os.ReadDir(skillsDir)
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".md") {
				continue
			}
			baseName := strings.TrimSuffix(name, ".md")
			targetDir := filepath.Join(skillsDir, baseName)
			targetFile := filepath.Join(targetDir, "SKILL.md")

			if _, err := os.Stat(targetDir); err == nil {
				if _, err := os.Stat(targetFile); err == nil {
					if noClobber {
						a.Warn(fmt.Sprintf("Skipping flat skill %s — %s/SKILL.md already exists (--no-clobber)", name, baseName))
						skipped++
						continue
					}
					if a.DryRun {
						fmt.Fprintf(a.Stdout, "  would convert: skills/%s -> skills/%s/SKILL.md (overwrite)\n", name, baseName)
					} else {
						os.Rename(filepath.Join(skillsDir, name), targetFile)
						a.Info(fmt.Sprintf("Converted: skills/%s -> skills/%s/SKILL.md (overwrote existing)", name, baseName))
					}
					merged++
					fixed++
					continue
				}
			}

			if a.DryRun {
				fmt.Fprintf(a.Stdout, "  would convert: skills/%s -> skills/%s/SKILL.md\n", name, baseName)
			} else {
				os.MkdirAll(targetDir, 0755)
				os.Rename(filepath.Join(skillsDir, name), targetFile)
				a.Info(fmt.Sprintf("Converted: skills/%s -> skills/%s/SKILL.md", name, baseName))
			}
			fixed++
		}
	}

	// Phase 2: Repair broken/missing symlinks
	repaired := 0
	conflicts := 0
	for _, target := range a.ActiveTargets {
		targetDir := ResolveTargetDir(target, a.ProjectRoot)
		agentsRel := ResolveAgentsRel(target)

		for _, subdir := range subdirs {
			b, ok := BucketForDir(subdir)
			if !ok || !b.SyncsToTool(target) {
				continue
			}
			if fi, err := os.Stat(filepath.Join(agentsAbs, subdir)); err != nil || !fi.IsDir() {
				continue
			}
			res := a.linkBucket(targetDir, agentsRel, b)
			conflicts += res.Conflicts
			if res.changed() {
				repaired++
			}
		}
	}
	// CLAUDE.md follows the same SPEC-013 policy as sync.
	if changed, err := a.applyClaudeMD(a.claudeMDDecision()); err != nil {
		a.Warn(err.Error())
	} else if changed {
		repaired++
	}

	// Phase 3: Migrate legacy STATE.md
	stateMigrated := 0
	legacyStatePath := filepath.Join(agentsAbs, "STATE.md")
	if _, err := os.Stat(legacyStatePath); err == nil {
		stateRulePath := filepath.Join(agentsAbs, "rules", "state.md")
		if _, err := os.Stat(stateRulePath); os.IsNotExist(err) {
			if a.DryRun {
				fmt.Fprintf(a.Stdout, "  would create: .agents/rules/state.md from template\n")
			} else {
				os.MkdirAll(filepath.Join(agentsAbs, "rules"), 0755)
				os.WriteFile(stateRulePath, []byte(templates.State()), 0644)
				a.Info("Created .agents/rules/state.md (state convention rule)")
			}
		}
		if a.DryRun {
			fmt.Fprintf(a.Stdout, "  would migrate: .agents/STATE.md → per-file state pattern\n")
		} else {
			a.migrateLegacyState(agentsAbs)
		}
		stateMigrated = 1
	}

	// Summary
	if fixed == 0 && skipped == 0 && repaired == 0 && stateMigrated == 0 && conflicts == 0 {
		a.Info("Nothing to fix — all directories and symlinks are correct.")
	} else {
		if fixed > 0 {
			a.Info(fmt.Sprintf("Fixed %d item(s).", fixed))
		}
		if merged > 0 {
			a.Info(fmt.Sprintf("Merged %d item(s) (legacy overwrote existing).", merged))
		}
		if skipped > 0 {
			a.Warn(fmt.Sprintf("Skipped %d item(s) (use without --no-clobber to merge).", skipped))
		}
		if repaired > 0 {
			a.Info(fmt.Sprintf("Repaired %d symlink(s).", repaired))
		}
		if stateMigrated > 0 {
			a.Info("Migrated legacy STATE.md to per-file state pattern.")
		}
		if fixed > 0 {
			a.Info("Run 'sync-agents sync' to update agent target symlinks.")
		}
	}
	if conflicts > 0 {
		a.Warn(fmt.Sprintf("Fix finished with %d conflict(s); nothing was deleted", conflicts))
		return fmt.Errorf("%d conflict(s)", conflicts)
	}
	return nil
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

func (a *App) migrateLegacyState(agentsDir string) {
	legacy := filepath.Join(agentsDir, "STATE.md")
	if _, err := os.Stat(legacy); err != nil {
		return
	}

	data, err := os.ReadFile(legacy)
	if err != nil {
		return
	}

	// Check for meaningful content
	contentLines := 0
	boilerplate := regexp.MustCompile(`^(---|trigger:|#|$|Track project|Update this|Be sure|Description of|Save both|A new file|STATE HISTORY)`)
	for _, line := range strings.Split(string(data), "\n") {
		if !boilerplate.MatchString(line) {
			contentLines++
		}
	}

	if contentLines > 0 {
		timestamp := time.Now().Format("20060102150405")
		migrated := filepath.Join(agentsDir, fmt.Sprintf("STATE_legacy-history_%s.md", timestamp))
		os.WriteFile(migrated, data, 0644)
		a.Info(fmt.Sprintf("Migrated legacy STATE.md history → %s", filepath.Base(migrated)))
	}

	os.Remove(legacy)
	a.Info("Removed legacy .agents/STATE.md (replaced by rules/state.md pattern)")
}

// listMDFilesRecursive returns the .md files under dir at any depth,
// as slash-separated paths relative to dir with the extension
// stripped ("effort-x/plan-a"). ADR status directories allow
// grouping records in subdirectories (SPEC-004 Part F), so lookups by
// name walk them recursively.
//
// The second return value is a slice of non-fatal warning messages
// (permission errors, unreadable files) for the caller to surface.
func listMDFilesRecursive(dir string) ([]string, []string) {
	var names []string
	var warns []string
	// An absent directory is empty, not a fault: every ADR status
	// subdirectory (accepted/proposed/denied) is optional, so a missing
	// one must list as empty and stay silent rather than warn.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			return nil
		}
		re, err := filepath.Rel(dir, path)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		names = append(names, strings.TrimSuffix(filepath.ToSlash(re), ".md"))
		return nil
	})
	sort.Strings(names)
	return names, warns
}

// artifactDescription extracts the frontmatter `description` of the
// markdown file at path for a one-line listing. SPEC-013 keeps it for
// the pointer lines of capped delivery bundles. Returns
// "" (no suffix rendered) when the file has no frontmatter, the
// description is empty, or it is an unfinished scaffold stub (starts
// with "TODO"). Folded (`>`) and literal (`|`) multi-line
// descriptions are resolved to their text and collapsed onto one line
// (#95). Long descriptions are truncated so one artifact can't
// dominate a listing.
func artifactDescription(path string) string {
	const maxIndexDescription = 140
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	block, err := parseFMBlock(string(raw))
	if err != nil || !block.present {
		return ""
	}
	desc := block.value("description")
	if desc == "" || strings.HasPrefix(desc, "TODO") {
		return ""
	}
	desc = strings.Join(strings.Fields(desc), " ")
	if len(desc) > maxIndexDescription {
		desc = truncateAtWord(desc, maxIndexDescription) + "…"
	}
	return desc
}

// stateSnapshotIsShared reports whether a STATE_*.md snapshot opts in
// as a shared task via `shared: true` frontmatter. Snapshots are
// per-engineer by default; only shared ones enter the integrity lock.
func stateSnapshotIsShared(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, err := parseFMBlock(string(raw))
	if err != nil || !block.present {
		return false
	}
	v, _ := block.get("shared")
	return strings.EqualFold(strings.TrimSpace(v), "true")
}

// isBucketActive reports whether the target with the given ID is in
// ActiveTargets. Used to gate hooks merge/local-sync operations.
func (a *App) isBucketActive(targetID string) bool {
	for _, t := range a.ActiveTargets {
		if t == targetID {
			return true
		}
	}
	return false
}

func (a *App) addDefaultGitignoreEntries() {
	gitignore := filepath.Join(a.ProjectRoot, ".gitignore")

	// Create if not exists
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		os.WriteFile(gitignore, []byte{}, 0644)
		a.Info("Created .gitignore")
	}

	data, _ := os.ReadFile(gitignore)
	content := string(data)

	// Add .DS_Store
	if !regexp.MustCompile(`(?i)^\.DS_Store$`).MatchString(content) {
		hasDS := false
		for _, line := range strings.Split(content, "\n") {
			if strings.EqualFold(strings.TrimSpace(line), ".DS_Store") {
				hasDS = true
				break
			}
		}
		if !hasDS {
			if len(content) > 0 && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += ".DS_Store\n"
			a.Info("Added .DS_Store to .gitignore")
		}
	}

	marker := "# sync-agents — ignore tool artifacts, keep symlinks"
	sectionEntries := []string{
		".cursor/*",
		"!.cursor/rules",
		".codex/*",
		"!.codex/instructions.md",
		".github/copilot/*",
		"!.github/copilot/instructions.md",
	}

	if strings.Contains(content, marker) {
		needsUpdate := false
		for _, entry := range sectionEntries {
			if !strings.Contains(content, entry) {
				needsUpdate = true
				break
			}
		}
		if needsUpdate {
			var result []string
			inSection := false
			for _, line := range strings.Split(content, "\n") {
				if line == marker {
					inSection = true
					result = append(result, line)
					result = append(result, sectionEntries...)
					continue
				}
				if inSection {
					if line == "" || strings.HasPrefix(line, "#") {
						inSection = false
						result = append(result, line)
					}
					continue
				}
				result = append(result, line)
			}
			content = strings.Join(result, "\n")
			a.Info("Updated sync-agents section in .gitignore")
		}
	} else {
		if len(content) > 0 && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += marker + "\n"
		for _, entry := range sectionEntries {
			content += entry + "\n"
		}
		a.Info(fmt.Sprintf("Added sync-agents section to .gitignore with %d entries", len(sectionEntries)+1))
	}

	os.WriteFile(gitignore, []byte(content), 0644)
}

// updateGitignore appends the exact paths sync owns to .gitignore.
// CLAUDE.md is listed only when the CLAUDE.md decision says it is, or
// will be, our symlink: a real CLAUDE.md is the user's to commit.
func (a *App) updateGitignore(claude ClaudeMDDecision) {
	gitignore := filepath.Join(a.ProjectRoot, ".gitignore")

	var entries []string
	for _, target := range a.ActiveTargets {
		targetDir := ResolveTargetDir(target, a.ProjectRoot)
		rel := targetDir
		if strings.HasPrefix(targetDir, a.ProjectRoot+"/") {
			rel = targetDir[len(a.ProjectRoot)+1:]
		}
		entries = append(entries, rel+"/")
	}
	if claude.linked() {
		entries = append(entries, "CLAUDE.md")
	}

	if a.DryRun {
		data, _ := os.ReadFile(gitignore)
		content := string(data)
		for _, entry := range entries {
			if !containsExactLine(content, entry) {
				fmt.Fprintf(a.Stdout, "  would add to .gitignore: %s\n", entry)
			}
		}
		return
	}

	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		os.WriteFile(gitignore, []byte{}, 0644)
	}

	data, _ := os.ReadFile(gitignore)
	content := string(data)

	added := 0
	for _, entry := range entries {
		if !containsExactLine(content, entry) {
			if added == 0 {
				if !strings.Contains(content, "# sync-agents") {
					if len(content) > 0 && content != "" {
						if !strings.HasSuffix(content, "\n") {
							content += "\n"
						}
						content += "\n"
					}
					content += "# sync-agents (generated symlinks)\n"
				}
			}
			content += entry + "\n"
			added++
		}
	}

	if added > 0 {
		os.WriteFile(gitignore, []byte(content), 0644)
		a.Info(fmt.Sprintf("Added %d entries to .gitignore", added))
	}
}

func containsExactLine(content, line string) bool {
	for _, l := range strings.Split(content, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

func sameInode(path1, path2 string) bool {
	fi1, err := os.Stat(path1)
	if err != nil {
		return false
	}
	fi2, err := os.Stat(path2)
	if err != nil {
		return false
	}
	return os.SameFile(fi1, fi2)
}
