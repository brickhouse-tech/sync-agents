package agent

import "path/filepath"

// resolveClineDir is Cline's user-scope home: $CLINE_DIR when set, else
// ~/.cline (SPEC-016 E1).
func resolveClineDir(parent string, env ToolEnv) (string, error) {
	if d := env.getenv("CLINE_DIR"); d != "" {
		return expandTilde(d, parent), nil
	}
	return filepath.Join(parent, ".cline"), nil
}
