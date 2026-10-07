package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file is the only place that knows OpenClaw's on-disk layout.
// OpenClaw reads a fixed set of files from its workspace directory and
// follows neither @-imports nor Markdown links, so passive rules reach
// its agents only as text inlined into <workspace>/AGENTS.md (the
// openclaw row of channelSpecs, region OpenClawRulesRegion).

// ToolEnv is the environment a tool resolver reads: environment
// variables, files, and programs it may run. It is the injected
// boundary input that keeps resolvers pure, so tests drive every
// precedence branch without touching the real environment, $HOME, or
// an installed tool.
type ToolEnv struct {
	// Getenv looks up an environment variable. Nil reads as unset.
	Getenv func(string) string

	// ReadFile reads a config file. Nil means os.ReadFile.
	ReadFile func(string) ([]byte, error)

	// Run executes a program, such as `claude --version` for the
	// CLAUDE.md policy (claudemd.go). Nil means no program can run, so
	// the probe reports an unknown version and nothing changes.
	Run CommandRunner
}

// OSToolEnv reads the real process environment and filesystem and runs
// real programs. NewApp installs it; a zero-value App (every test
// literal) sees no environment variables at all and runs nothing, so a
// developer's OPENCLAW_* exports or installed claude can never steer a
// test.
func OSToolEnv() ToolEnv {
	return ToolEnv{Getenv: os.Getenv, ReadFile: os.ReadFile, Run: runCommand}
}

func (e ToolEnv) getenv(key string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(key)
}

func (e ToolEnv) readFile(path string) ([]byte, error) {
	if e.ReadFile == nil {
		return os.ReadFile(path)
	}
	return e.ReadFile(path)
}

// openClawDefaultBootstrapMaxChars is OpenClaw's per-file bootstrap
// cap when agents.defaults.bootstrapMaxChars is unset. Above it OpenClaw
// keeps the head and tail of the file and drops the middle.
const openClawDefaultBootstrapMaxChars = 20000

// openClawLayout is what sync-agents needs from an OpenClaw install.
type openClawLayout struct {
	// Workspace is the absolute workspace directory holding AGENTS.md.
	Workspace string

	// BootstrapMaxChars is the per-file cap OpenClaw truncates at.
	BootstrapMaxChars int
}

// openClawConfig is the subset of openclaw.json sync-agents reads.
type openClawConfig struct {
	Agents struct {
		Defaults struct {
			Workspace         string `json:"workspace"`
			BootstrapMaxChars *int   `json:"bootstrapMaxChars"`
		} `json:"defaults"`
	} `json:"agents"`
}

// resolveOpenClaw mirrors OpenClaw 2026.9.6's resolution order
// (6.35 put a profile's workspace at home/.openclaw/workspace-<profile>
// instead; the configured workspace wins on both):
//
//   - home: $OPENCLAW_HOME, else parent (the global root's parent, which
//     is $HOME in production and a temp dir in tests).
//   - state dir: $OPENCLAW_STATE_DIR, else home/.openclaw, or
//     home/.openclaw-<profile> for a non-default $OPENCLAW_PROFILE.
//   - config: $OPENCLAW_CONFIG_PATH, else <state dir>/openclaw.json.
//   - workspace: config agents.defaults.workspace, else
//     $OPENCLAW_WORKSPACE_DIR, else <state dir>/workspace.
//
// A missing config file is normal (defaults apply). A config that
// exists but does not parse is an error: guessing past it could splice
// rules into a workspace OpenClaw is not using.
func resolveOpenClaw(parent string, env ToolEnv) (openClawLayout, error) {
	home := parent
	if h := env.getenv("OPENCLAW_HOME"); h != "" {
		home = expandTilde(h, parent)
	}
	stateDir := filepath.Join(home, ".openclaw")
	if p := strings.TrimSpace(env.getenv("OPENCLAW_PROFILE")); p != "" && !strings.EqualFold(p, "default") {
		stateDir = filepath.Join(home, ".openclaw-"+p)
	}
	if s := env.getenv("OPENCLAW_STATE_DIR"); s != "" {
		stateDir = expandTilde(s, home)
	}
	configPath := filepath.Join(stateDir, "openclaw.json")
	if c := env.getenv("OPENCLAW_CONFIG_PATH"); c != "" {
		configPath = expandTilde(c, home)
	}

	var cfg openClawConfig
	data, err := env.readFile(configPath)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return openClawLayout{}, fmt.Errorf("parse %s: %w", configPath, err)
		}
	case !os.IsNotExist(err):
		return openClawLayout{}, fmt.Errorf("read %s: %w", configPath, err)
	}

	layout := openClawLayout{
		Workspace:         filepath.Join(stateDir, "workspace"),
		BootstrapMaxChars: openClawDefaultBootstrapMaxChars,
	}
	if w := env.getenv("OPENCLAW_WORKSPACE_DIR"); w != "" {
		layout.Workspace = expandTilde(w, home)
	}
	if w := cfg.Agents.Defaults.Workspace; w != "" {
		layout.Workspace = expandTilde(w, home)
	}
	if n := cfg.Agents.Defaults.BootstrapMaxChars; n != nil && *n > 0 {
		layout.BootstrapMaxChars = *n
	}
	return layout, nil
}

// resolveOpenClawWorkspace is the openclaw Tool's ResolveGlobalDir.
func resolveOpenClawWorkspace(parent string, env ToolEnv) (string, error) {
	layout, err := resolveOpenClaw(parent, env)
	return layout.Workspace, err
}

// expandTilde expands a leading "~" against home and makes a relative
// result absolute (against the working directory, as OpenClaw's own
// path.resolve does).
func expandTilde(p, home string) string {
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, p[2:])
	}
	return absOrSelf(p)
}
