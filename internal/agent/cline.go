package agent

import (
	"os"
	"path/filepath"
	"slices"
)

// resolveClineDir is Cline's user-scope home: $CLINE_DIR when set, else
// ~/.cline (SPEC-016 E1).
func resolveClineDir(parent string, env ToolEnv) (string, error) {
	if d := env.getenv("CLINE_DIR"); d != "" {
		return expandTilde(d, parent), nil
	}
	return filepath.Join(parent, ".cline"), nil
}

// toolNotes lists, per tool, what sync and status report about files
// the tool reads that sync-agents does not place.
var toolNotes = map[string]func(a *App) []string{"cline": (*App).clineNotes}

// clineNotes reports the project files that make Cline load a rule or
// skill twice. A real .clinerules/ is the user's and is never touched
// (SPEC-016 E8). Cline also reads .claude/skills, so every skill sync
// links there for claude appears beside its .agents/skills copy; that
// link stays, because Claude needs it.
func (a *App) clineNotes() []string {
	var notes []string
	if fi, err := os.Lstat(filepath.Join(a.ProjectRoot, ".clinerules")); err == nil && fi.IsDir() {
		notes = append(notes, ".clinerules/ is yours and left alone; Cline reads it as well as .cline/rules")
	}
	skills, _ := BucketForDir("skills")
	claudeSkills := filepath.Join(ResolveTargetDir("claude", a.ProjectRoot), skills.Dir)
	if slices.Contains(a.ActiveTargets, "claude") && len(a.agentsLinks(claudeSkills, skills)) > 0 {
		notes = append(notes, "Cline reads .claude/skills as well as .agents/skills, so each skill linked for claude loads twice")
	}
	return notes
}
