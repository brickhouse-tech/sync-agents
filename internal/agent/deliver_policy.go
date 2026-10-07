package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file holds the .agents/config keys the delivery shell reads
// (index, through readConfigKey) and the .gitignore lines it derives
// from a run (SPEC-013 §Index policy).

// IndexPolicy is the `index` key in .agents/config. It decides whether
// generated delivery is committed (SPEC-013 §Index policy). One value
// covers .agents/index/ and every LinkMount path, so a committed link
// never points at an ignored file.
type IndexPolicy int

const (
	// IndexLocal (default): .agents/index/ and the link paths are
	// gitignored together, so a fresh clone has neither and nothing
	// dangles. Output is OS-gated for this machine and, for MergeGlobal
	// channels (Cursor), carries this user's global rules.
	IndexLocal IndexPolicy = iota

	// IndexCommit: both are committed, so fresh clones and cloud agents
	// get delivery with no setup. Every OS scope is compiled (with OS
	// headers) and global rules are never merged, so the committed
	// bytes are the same on every contributor's machine.
	IndexCommit
)

// ReadConfigIndex reads `index = local|commit` from <agentsDir>/config.
// An absent file or key means local. Any other value is an error; the
// caller falls back to local, the choice that never commits anything.
func ReadConfigIndex(agentsDir string) (IndexPolicy, error) {
	val, err := readConfigKey(agentsDir, "index")
	if err != nil {
		return IndexLocal, err
	}
	switch strings.ToLower(val) {
	case "", "local":
		return IndexLocal, nil
	case "commit":
		return IndexCommit, nil
	default:
		return IndexLocal, fmt.Errorf("index = %q in .agents/config is not local or commit; using local", val)
	}
}

// readConfigKey returns key's value from <agentsDir>/config, or "" when
// the file or the key is absent. Lines are `key = value`; `#` starts a
// comment line. The first occurrence wins, as in the other config
// readers.
func readConfigKey(agentsDir, key string) (string, error) {
	data, err := os.ReadFile(filepath.Join(agentsDir, "config"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), nil
		}
	}
	return "", nil
}

// gitignoreEntries derives the exact lines sync ensures in .gitignore
// from the channels this run delivered (SPEC-013 §Index policy).
// IndexLocal adds ".agents/index/" and each LinkMount path sync placed;
// IndexCommit adds neither, so both are committed together. A link
// path held by a real file (conflict) is the user's and is never
// ignored. "CLAUDE.md" is added when it is, or is about to be, our
// symlink (U5 policy), under either policy. Paths are relative to
// root, slash-separated.
func gitignoreEntries(p IndexPolicy, root string, results []ChannelResult, claude ClaudeMDDecision) []string {
	var out []string
	if p == IndexLocal && len(results) > 0 {
		out = append(out, ".agents/index/")
		for _, res := range results {
			if _, ok := res.Channel.Spec.Mount.(LinkMount); !ok || res.State == ChannelConflict {
				continue
			}
			if rel, err := filepath.Rel(root, res.Channel.Native); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
	}
	if claude.linked() {
		out = append(out, "CLAUDE.md")
	}
	return out
}
