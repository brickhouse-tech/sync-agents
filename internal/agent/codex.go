package agent

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// This file is the only place that knows Codex's on-disk layout
// (SPEC-013 §Per-tool delivery, E7). Codex reads
// $CODEX_HOME/AGENTS.override.md, else $CODEX_HOME/AGENTS.md, then one
// file per directory from the git root down to the working directory
// (AGENTS.override.md before AGENTS.md), and stops loading once the
// combined size reaches project_doc_max_bytes.

// codexDefaultProjectDocMaxBytes is Codex's combined limit on the
// instruction files it loads when project_doc_max_bytes is unset.
const codexDefaultProjectDocMaxBytes = 32 * 1024

// codexLayout is what sync-agents needs from a Codex install.
type codexLayout struct {
	// Home is $CODEX_HOME, else <parent>/.codex.
	Home string

	// MaxBytes is project_doc_max_bytes from <Home>/config.toml, else
	// 32 KiB.
	MaxBytes int

	// Global is the global file Codex loads: AGENTS.override.md when it
	// is non-empty, else AGENTS.md (which may not exist).
	Global string
}

// resolveCodex mirrors Codex's own lookup. config.toml is scanned for a
// top-level `project_doc_max_bytes = N` line (before the first table
// header), so there is no TOML dependency; everything else in the file
// is ignored. A missing config.toml is normal. A value that is not a
// positive integer is an error, because guessing past it would size
// bundles against a limit Codex does not use.
func resolveCodex(parent string, env ToolEnv) (codexLayout, error) {
	home := filepath.Join(parent, ".codex")
	if h := env.getenv("CODEX_HOME"); h != "" {
		home = expandTilde(h, parent)
	}
	layout := codexLayout{
		Home:     home,
		MaxBytes: codexDefaultProjectDocMaxBytes,
		Global:   filepath.Join(home, "AGENTS.md"),
	}

	configPath := filepath.Join(home, "config.toml")
	data, err := env.readFile(configPath)
	switch {
	case err == nil:
		n, found, err := scanTOMLTopLevelInt(data, "project_doc_max_bytes")
		if err != nil {
			return codexLayout{}, fmt.Errorf("parse %s: %w", configPath, err)
		}
		if found {
			layout.MaxBytes = n
		}
	case !os.IsNotExist(err):
		return codexLayout{}, fmt.Errorf("read %s: %w", configPath, err)
	}

	if override := filepath.Join(home, "AGENTS.override.md"); nonEmptyFile(env, override) {
		layout.Global = override
	}
	return layout, nil
}

// scanTOMLTopLevelInt finds `key = N` among the top-level keys of a TOML
// document: the lines before the first `[table]` header. A trailing
// `# comment` is allowed. found is false when the key is absent.
func scanTOMLTopLevelInt(data []byte, key string) (n int, found bool, err error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			break
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		v, _, _ = strings.Cut(v, "#")
		v = strings.ReplaceAll(strings.TrimSpace(v), "_", "")
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return 0, false, fmt.Errorf("%s = %q is not a positive integer", key, strings.TrimSpace(v))
		}
		return n, true, nil
	}
	return 0, false, sc.Err()
}

// nonEmptyFile reports whether path holds anything besides whitespace.
// Codex skips an empty override, so an empty one shadows nothing.
func nonEmptyFile(env ToolEnv, path string) bool {
	data, err := env.readFile(path)
	return err == nil && len(bytes.TrimSpace(data)) > 0
}

// codexCap is Codex's limit with its knob spelled out for warnings.
func codexCap(l codexLayout) Cap {
	return Cap{
		Limit: l.MaxBytes,
		Unit:  CapBytes,
		Knob:  "project_doc_max_bytes in " + filepath.Join(l.Home, "config.toml"),
	}
}

// codexGlobalBudget fits the global file against the whole limit,
// because Codex loads it first. The shell reserves the host's bytes
// outside the codex-rules region.
func codexGlobalBudget(tc ToolContext) (Budget, error) {
	l, err := resolveCodex(tc.Parent, tc.Env)
	if err != nil {
		return Budget{}, err
	}
	return Budget{Cap: codexCap(l)}, nil
}

// codexProjectBudget reserves the global file's size, because Codex
// loads the project-root override after it under the same combined
// limit.
func codexProjectBudget(tc ToolContext) (Budget, error) {
	l, err := resolveCodex(tc.Parent, tc.Env)
	if err != nil {
		return Budget{}, err
	}
	data, err := tc.Env.readFile(l.Global)
	if err != nil && !os.IsNotExist(err) {
		return Budget{}, fmt.Errorf("read %s: %w", l.Global, err)
	}
	return Budget{Cap: codexCap(l), Reserved: len(data)}, nil
}

// codexGlobalOverride is the codex global ShadowedBy: a non-empty
// AGENTS.override.md in the Codex home hides the AGENTS.md that carries
// our region. It returns that path, or "".
func codexGlobalOverride(tc ToolContext) string {
	override := filepath.Join(tc.Home, "AGENTS.override.md")
	if nonEmptyFile(tc.Env, override) {
		return override
	}
	return ""
}
