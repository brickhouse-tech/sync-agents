package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This file owns the project CLAUDE.md -> AGENTS.md symlink (SPEC-013
// §CLAUDE.md policy). Claude Code 2.1.281 and later read AGENTS.md on
// their own when no CLAUDE.md, .claude/CLAUDE.md, or CLAUDE.local.md is
// present, so the link is created only where the installed Claude Code
// still needs it. A real CLAUDE.md is the user's and is never moved or
// deleted, --overwrite included.

// Version is a Claude Code release number.
type Version struct{ Major, Minor, Patch int }

// String renders v as "X.Y.Z".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Less reports whether v is an earlier release than o.
func (v Version) Less(o Version) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// ParseVersion reads the first "X.Y.Z" in s, for example
// "2.1.286 (Claude Code)". It reports false when s holds no such
// triple or a component does not fit in an int.
func ParseVersion(s string) (Version, bool) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	var parts [3]int
	for i := range parts {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Version{}, false
		}
		parts[i] = n
	}
	return Version{parts[0], parts[1], parts[2]}, true
}

// ClaudeNativeAgentsMD is the first Claude Code release that reads
// AGENTS.md natively in every session when a directory has no
// CLAUDE.md. 2.1.277 through 2.1.280 already read it, except in Amazon
// Bedrock and telemetry-off sessions, so they still get the link (E4,
// code.claude.com/docs/en/memory, fetched 2026-10-01).
var ClaudeNativeAgentsMD = Version{2, 1, 281}

// CommandRunner runs a program and returns its stdout. ToolEnv.Run is
// one; OSToolEnv installs runCommand. The zero ToolEnv has no runner,
// so tests never start a real claude.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// runCommand is the real CommandRunner. WaitDelay bounds the wait for
// output pipes after ctx kills the process, so a child that leaves a
// grandchild holding stdout cannot outlive the timeout by much.
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

// claudeProbeTimeout bounds `claude --version`. The probe only runs when
// the CLAUDE.md decision depends on it (SPEC-013 T6).
const claudeProbeTimeout = 5 * time.Second

// errNoRunner is the probe error when ToolEnv has no CommandRunner.
var errNoRunner = errors.New("no command runner configured")

// ClaudeProbe is what `claude --version` reported.
type ClaudeProbe struct {
	Version Version
	Known   bool

	// Err says why Known is false: no runner, claude not on PATH, a
	// non-zero exit, a timeout, or output with no version in it.
	Err error
}

// probeClaude runs `claude --version` through run, with a 5 s timeout.
func probeClaude(ctx context.Context, run CommandRunner) ClaudeProbe {
	if run == nil {
		return ClaudeProbe{Err: errNoRunner}
	}
	ctx, cancel := context.WithTimeout(ctx, claudeProbeTimeout)
	defer cancel()
	out, err := run(ctx, "claude", "--version")
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ClaudeProbe{Err: fmt.Errorf("claude --version timed out after %s", claudeProbeTimeout)}
		}
		return ClaudeProbe{Err: fmt.Errorf("claude --version: %w", err)}
	}
	v, ok := ParseVersion(string(out))
	if !ok {
		return ClaudeProbe{Err: fmt.Errorf("claude --version printed no version: %q", strings.TrimSpace(string(out)))}
	}
	return ClaudeProbe{Version: v, Known: true}
}

// ClaudeMDMode is the `claude-md` key in .agents/config.
type ClaudeMDMode int

const (
	// ClaudeMDAuto decides from the installed Claude Code version (default).
	ClaudeMDAuto ClaudeMDMode = iota

	// ClaudeMDLink always ensures CLAUDE.md -> AGENTS.md.
	ClaudeMDLink

	// ClaudeMDOff never touches CLAUDE.md.
	ClaudeMDOff
)

// ReadConfigClaudeMD reads the `claude-md = auto|link|off` key from
// <agentsDir>/config. An absent file or key means auto. Any other
// value is an error, so a typo never silently picks a policy.
func ReadConfigClaudeMD(agentsDir string) (ClaudeMDMode, error) {
	data, err := os.ReadFile(filepath.Join(agentsDir, "config"))
	if errors.Is(err, os.ErrNotExist) {
		return ClaudeMDAuto, nil
	}
	if err != nil {
		return ClaudeMDAuto, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if strings.TrimSpace(parts[0]) != "claude-md" {
			continue
		}
		switch val := strings.ToLower(strings.TrimSpace(parts[1])); val {
		case "auto", "":
			return ClaudeMDAuto, nil
		case "link":
			return ClaudeMDLink, nil
		case "off":
			return ClaudeMDOff, nil
		default:
			return ClaudeMDAuto, fmt.Errorf("claude-md = %q in .agents/config is not auto, link, or off", val)
		}
	}
	return ClaudeMDAuto, nil
}

// ClaudeMDState is what <project>/CLAUDE.md is now.
type ClaudeMDState int

const (
	ClaudeMDAbsent      ClaudeMDState = iota
	ClaudeMDOurLink                   // symlink that resolves to the project AGENTS.md
	ClaudeMDForeignLink               // any other symlink
	ClaudeMDRealFile                  // a regular file or directory: the user's
)

// ClaudeMDAction is what sync does to <project>/CLAUDE.md. Sync never
// touches a real CLAUDE.md or a foreign symlink; it creates our symlink
// when Claude Code needs it and removes it when it is in the way.
type ClaudeMDAction int

const (
	// ClaudeMDKeep leaves CLAUDE.md exactly as it is.
	ClaudeMDKeep ClaudeMDAction = iota

	// ClaudeMDLinkIt creates or repairs CLAUDE.md -> AGENTS.md.
	ClaudeMDLinkIt

	// ClaudeMDUnlink removes our CLAUDE.md -> AGENTS.md symlink. On a
	// Claude Code that reads AGENTS.md natively the link is not just
	// redundant: any CLAUDE.md stops Claude from reading AGENTS.md in
	// every directory below it, so nested AGENTS.md files go dark.
	ClaudeMDUnlink
)

// ClaudeMDDecision is an action, the sentence status prints for it, and
// whether sync warns.
type ClaudeMDDecision struct {
	Action ClaudeMDAction

	// Current is the state the decision was made from.
	Current ClaudeMDState

	// Reason completes the sentence "CLAUDE.md <Reason>".
	Reason string

	// Warn makes sync and fix print Reason as a warning.
	Warn bool

	// Managed is false when claude is not an active target or
	// claude-md = off; sync and fix then say nothing about CLAUDE.md.
	Managed bool
}

// linked reports whether CLAUDE.md is, or after this run will be, our
// symlink. Only then does sync gitignore it.
func (d ClaudeMDDecision) linked() bool {
	return d.Action == ClaudeMDLinkIt || (d.Current == ClaudeMDOurLink && d.Action != ClaudeMDUnlink)
}

// claudeMDFacts is everything decideClaudeMD reads besides the probe.
type claudeMDFacts struct {
	Mode         ClaudeMDMode
	ClaudeActive bool
	AgentsMD     bool // <project>/AGENTS.md exists
	LocalMD      bool // <project>/CLAUDE.local.md exists
	Current      ClaudeMDState

	// Shadowing is the nearest instruction file in a directory above the
	// project that stops Claude Code from reading AGENTS.md here (see
	// shadowingClaudeMD), or "" when there is none.
	Shadowing string
}

// decideClaudeMD is the CLAUDE.md policy (SPEC-013 §CLAUDE.md policy).
// It is pure and table-tested. probe is called only when the answer
// depends on the version, so sync runs `claude --version` only when it
// has to. Rows are checked in order:
//
//	claude not an active target        -> Keep, unmanaged
//	claude-md = off                    -> Keep, unmanaged
//	CLAUDE.md is a real file           -> Keep, Warn: it hides AGENTS.md; suggest @AGENTS.md
//	CLAUDE.md is a foreign symlink     -> Keep, Warn
//	no AGENTS.md                       -> Keep (a link would dangle)
//	claude-md = link                   -> LinkIt
//	auto, CLAUDE.local.md present      -> LinkIt: it suppresses native AGENTS.md reading
//	auto, CLAUDE.md in a parent dir    -> LinkIt: it suppresses native AGENTS.md reading
//	auto, version < 2.1.281            -> LinkIt: that version needs CLAUDE.md
//	auto, version >= 2.1.281, our link -> Unlink: it hides nested AGENTS.md files
//	auto, version >= 2.1.281, absent   -> Keep: none created
//	auto, version unknown              -> Keep, Warn, naming probe.Err and the claude-md key
//
// "Unknown changes nothing" is deliberate: claude is often on a
// terminal's PATH but not on a git hook's, and acting on unknown would
// make the two runs undo each other.
func decideClaudeMD(f claudeMDFacts, probe func() ClaudeProbe) ClaudeMDDecision {
	d := ClaudeMDDecision{Action: ClaudeMDKeep, Current: f.Current, Managed: true}
	link := func(reason string) ClaudeMDDecision {
		d.Action, d.Reason = ClaudeMDLinkIt, reason
		return d
	}
	keep := func(reason string, warn bool) ClaudeMDDecision {
		d.Reason, d.Warn = reason, warn
		return d
	}

	switch {
	case !f.ClaudeActive:
		d.Managed = false
		return keep("not managed: claude is not an active target", false)
	case f.Mode == ClaudeMDOff:
		d.Managed = false
		return keep("not managed: claude-md = off", false)
	case f.Current == ClaudeMDRealFile:
		return keep("is a real file, so Claude Code reads it instead of AGENTS.md; "+
			"add the line @AGENTS.md to it to load both (sync-agents never moves it)", true)
	case f.Current == ClaudeMDForeignLink:
		return keep("is a symlink that does not point at AGENTS.md; leaving it in place", true)
	case !f.AgentsMD:
		return keep("not linked: AGENTS.md does not exist", false)
	case f.Mode == ClaudeMDLink:
		return link("-> AGENTS.md (claude-md = link)")
	case f.LocalMD:
		return link("-> AGENTS.md: CLAUDE.local.md stops Claude Code from reading AGENTS.md on its own")
	case f.Shadowing != "":
		return link(fmt.Sprintf("-> AGENTS.md: %s stops Claude Code from reading AGENTS.md on its own here", f.Shadowing))
	}

	p := probe()
	switch {
	case !p.Known:
		return keep(fmt.Sprintf("left unchanged: cannot tell the Claude Code version (%v); "+
			"set claude-md = link or claude-md = off in .agents/config to decide without it", p.Err), true)
	case p.Version.Less(ClaudeNativeAgentsMD):
		return link(fmt.Sprintf("-> AGENTS.md: Claude Code %s reads CLAUDE.md, not AGENTS.md (native from %s)",
			p.Version, ClaudeNativeAgentsMD))
	case f.Current == ClaudeMDOurLink:
		d.Action = ClaudeMDUnlink
		d.Reason = fmt.Sprintf("-> AGENTS.md removed: Claude Code %s reads AGENTS.md natively (>= %s), "+
			"and the link would stop it reading AGENTS.md in subdirectories", p.Version, ClaudeNativeAgentsMD)
		return d
	default:
		return keep(fmt.Sprintf("not created: Claude Code %s reads AGENTS.md natively (>= %s)",
			p.Version, ClaudeNativeAgentsMD), false)
	}
}

// classifyClaudeMD reports what <root>/CLAUDE.md is. A symlink counts
// as ours when it resolves to AGENTS.md, so an absolute link placed by
// hand or by an older release is not treated as foreign.
func classifyClaudeMD(root string) (ClaudeMDState, error) {
	path := filepath.Join(root, "CLAUDE.md")
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ClaudeMDAbsent, nil
	}
	if err != nil {
		return ClaudeMDAbsent, err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return ClaudeMDRealFile, nil
	}
	if linkSatisfied(path, "AGENTS.md") {
		return ClaudeMDOurLink, nil
	}
	return ClaudeMDForeignLink, nil
}

// pathExists reports whether path exists (without following a final
// symlink). Errors other than "not found" are returned.
func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// shadowingClaudeMD returns the nearest CLAUDE.md, .claude/CLAUDE.md,
// or CLAUDE.local.md in a directory above root, walking up to the
// filesystem root. Claude Code reads AGENTS.md on its own only when none
// of these exists in the working directory or above it. The user-level
// home/.claude/CLAUDE.md does not count (Claude Code docs, "AGENTS.md").
// A home directory that is itself a project with CLAUDE.md -> AGENTS.md
// therefore hides AGENTS.md from every project below it.
func shadowingClaudeMD(root, home string) (string, error) {
	userLevel := ""
	if home != "" {
		userLevel = filepath.Join(home, ".claude", "CLAUDE.md")
	}
	dir := filepath.Clean(root)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
		for _, rel := range []string{"CLAUDE.md", filepath.Join(".claude", "CLAUDE.md"), "CLAUDE.local.md"} {
			path := filepath.Join(dir, rel)
			if path == userLevel {
				continue
			}
			ok, err := pathExists(path)
			if err != nil {
				return "", err
			}
			if ok {
				return path, nil
			}
		}
	}
}

// claudeMDDecision reads the mode and the project state and decides for
// this run. Each command calls it once, so `claude --version` runs at
// most once per process and an unknown-version warning prints once.
// status reports the decision; sync and fix act on it; updateGitignore
// reads it. A config or filesystem error becomes a warning that changes
// nothing.
func (a *App) claudeMDDecision() ClaudeMDDecision {
	unchanged := func(err error) ClaudeMDDecision {
		return ClaudeMDDecision{Reason: fmt.Sprintf("left unchanged: %v", err), Warn: true, Managed: true}
	}
	mode, err := ReadConfigClaudeMD(filepath.Join(a.ProjectRoot, ".agents"))
	if err != nil {
		return unchanged(err)
	}
	cur, err := classifyClaudeMD(a.ProjectRoot)
	if err != nil {
		return unchanged(err)
	}
	agentsMD, err := pathExists(filepath.Join(a.ProjectRoot, "AGENTS.md"))
	if err != nil {
		return unchanged(err)
	}
	localMD, err := pathExists(filepath.Join(a.ProjectRoot, "CLAUDE.local.md"))
	if err != nil {
		return unchanged(err)
	}
	home, _ := os.UserHomeDir()
	shadowing, err := shadowingClaudeMD(a.ProjectRoot, home)
	if err != nil {
		return unchanged(err)
	}
	facts := claudeMDFacts{
		Mode:         mode,
		ClaudeActive: a.isBucketActive("claude"),
		AgentsMD:     agentsMD,
		LocalMD:      localMD,
		Current:      cur,
		Shadowing:    shadowing,
	}
	return decideClaudeMD(facts, func() ClaudeProbe {
		return probeClaude(context.Background(), a.ToolEnv.Run)
	})
}

// applyClaudeMD carries out d and reports whether it changed (or, under
// dry-run, would change) CLAUDE.md. Only ClaudeMDLinkIt writes, and
// decideClaudeMD never picks it over a real file, so a user's CLAUDE.md
// is never moved or deleted, --overwrite included.
func (a *App) applyClaudeMD(d ClaudeMDDecision) (bool, error) {
	if !d.Managed {
		return false, nil
	}
	if d.Warn {
		a.Warn("CLAUDE.md " + d.Reason)
		return false, nil
	}
	if d.Action == ClaudeMDUnlink {
		return a.unlinkClaudeMD(d)
	}
	if d.Action != ClaudeMDLinkIt {
		a.Info("CLAUDE.md " + d.Reason)
		return false, nil
	}
	outcome, err := a.placeLink("AGENTS.md", filepath.Join(a.ProjectRoot, "CLAUDE.md"), a.DryRun)
	if err != nil {
		return false, fmt.Errorf("link CLAUDE.md -> AGENTS.md: %w", err)
	}
	if outcome != linkNoop {
		a.Info("CLAUDE.md " + d.Reason)
	}
	return outcome != linkNoop, nil
}

// unlinkClaudeMD removes our CLAUDE.md symlink. It re-checks that the
// path is still a symlink resolving to AGENTS.md, so a file the user
// put there since the decision is never removed.
func (a *App) unlinkClaudeMD(d ClaudeMDDecision) (bool, error) {
	cur, err := classifyClaudeMD(a.ProjectRoot)
	if err != nil {
		return false, err
	}
	if cur != ClaudeMDOurLink {
		return false, nil
	}
	if a.DryRun {
		a.Info("would remove CLAUDE.md " + d.Reason)
		return true, nil
	}
	if err := os.Remove(filepath.Join(a.ProjectRoot, "CLAUDE.md")); err != nil {
		return false, fmt.Errorf("remove CLAUDE.md symlink: %w", err)
	}
	a.Info("CLAUDE.md " + d.Reason)
	return true, nil
}

// statusLine renders d for `sync-agents status`.
func (d ClaudeMDDecision) statusLine() string {
	tag := "info"
	switch {
	case d.Warn:
		tag = "warn"
	case d.Action == ClaudeMDUnlink:
		tag = "stale"
	case d.Current == ClaudeMDOurLink:
		tag = "ok"
	case d.Action == ClaudeMDLinkIt:
		tag = "missing"
	}
	return fmt.Sprintf("[%s] CLAUDE.md %s", tag, d.Reason)
}
