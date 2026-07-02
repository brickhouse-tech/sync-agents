package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SPEC-004 Part E: validate SKILL.md frontmatter against Claude's
// published skill authoring rules
// (https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)
// and mechanically upgrade what's fixable.
//
// Check codes (E = error, exit non-zero when unfixed; W = warning):
//
//	E001 frontmatter block missing            → fix: inject one
//	E002 name missing/empty                   → fix: slug of dir name
//	E003 name has invalid chars ([a-z0-9-])   → fix: slugify
//	E004 name > 64 chars                      → fix: truncate at hyphen
//	E005 name contains reserved word          → report only
//	E006 name ≠ skill directory name          → fix: dir slug
//	E007 description missing/empty            → fix: derive from body
//	E008 description > 1024 chars             → fix: truncate
//	E009 XML tags in name/description         → fix: strip
//	W101 first-person description             → report only
//	W102 description lacks when-to-use clause → report only
//	W103 SKILL.md body > 500 lines            → report only

const (
	skillNameMaxLen        = 64
	skillDescriptionMaxLen = 1024
	skillBodyMaxLines      = 500
)

var (
	skillNameRe    = regexp.MustCompile(`^[a-z0-9-]+$`)
	xmlTagRe       = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	firstPersonRe  = regexp.MustCompile(`(?i)^\s*(i |i'm |i am |i can |i will |you can )`)
	whenClauseRe   = regexp.MustCompile(`(?i)\bwhen\b`)
	reservedWords  = []string{"anthropic", "claude"}
	slugSeparators = regexp.MustCompile(`[\s_]+`)
	slugInvalid    = regexp.MustCompile(`[^a-z0-9-]+`)
	slugCollapse   = regexp.MustCompile(`-{2,}`)
)

// LintFinding is one issue found (and possibly fixed) by CmdLint.
type LintFinding struct {
	Path     string // display path, e.g. "skills/foo/SKILL.md"
	Code     string // "E001".."W103"
	Severity string // "error" | "warn"
	Message  string
	Fixed    bool
}

// CmdLint validates skill frontmatter, optionally amending fixable
// findings in place. v1 lints the skills bucket only; the [typ]
// argument exists so future buckets slot in per the registry.
func (a *App) CmdLint(typ string, fix bool) error {
	switch typ {
	case "", "all", "skill", "skills":
	default:
		a.Error(fmt.Sprintf("lint currently supports: skills (got %q)", typ))
		return fmt.Errorf("unsupported lint type")
	}

	if err := a.EnsureAgentsDir(); err != nil {
		return err
	}

	skillsDir := filepath.Join(a.ProjectRoot, ".agents", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			a.Info("no .agents/skills/ directory; nothing to lint")
			return nil
		}
		return err
	}

	var findings []LintFinding
	checked := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		skillPath := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			// Dirs without SKILL.md are scratch dirs by convention
			// (see DiscoverArtifacts); not lint's concern.
			continue
		}
		checked++
		fs, err := a.lintSkill(e.Name(), skillPath, fix)
		if err != nil {
			a.Warn(fmt.Sprintf("skills/%s: %v", e.Name(), err))
			findings = append(findings, LintFinding{
				Path: "skills/" + e.Name() + "/SKILL.md", Code: "E001",
				Severity: "error", Message: err.Error(),
			})
			continue
		}
		findings = append(findings, fs...)
	}

	fixed, errsLeft, warns := 0, 0, 0
	for _, f := range findings {
		status := ""
		if f.Fixed {
			status = " (fixed)"
			fixed++
		} else if f.Severity == "error" {
			errsLeft++
		} else {
			warns++
		}
		line := fmt.Sprintf("%s: %s %s%s", f.Path, f.Code, f.Message, status)
		if f.Severity == "error" && !f.Fixed {
			a.Error(line)
		} else if f.Fixed {
			a.Info(line)
		} else {
			a.Warn(line)
		}
	}

	a.Info(fmt.Sprintf("linted %d skill(s): %d fixed, %d error(s), %d warning(s)", checked, fixed, errsLeft, warns))
	if errsLeft > 0 {
		if !fix {
			a.Info("run `sync-agents lint --fix` to amend fixable findings")
		}
		return fmt.Errorf("%d lint error(s)", errsLeft)
	}
	return nil
}

// lintSkill checks one SKILL.md, rewriting it when fix is true and a
// fixable finding changed the frontmatter. The returned error is
// reserved for I/O and unterminated-frontmatter failures.
func (a *App) lintSkill(dirName, skillPath string, fix bool) ([]LintFinding, error) {
	display := "skills/" + dirName + "/SKILL.md"
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, err
	}

	block, err := parseFMBlock(string(raw))
	if err != nil {
		return nil, err
	}

	var findings []LintFinding
	add := func(code, severity, msg string, fixed bool) {
		findings = append(findings, LintFinding{
			Path: display, Code: code, Severity: severity, Message: msg, Fixed: fixed,
		})
	}

	dirSlug := slugifySkillName(dirName)
	slugReserved := containsReservedWord(dirSlug)
	changed := false

	if !block.present {
		// E001: no frontmatter at all. Fix injects a block; the name
		// and description checks below then populate it.
		canFix := fix && !slugReserved
		add("E001", "error", "frontmatter block missing", canFix)
		if !fix {
			// Without --fix the remaining checks would all fire on an
			// empty block; one actionable finding is clearer.
			return findings, nil
		}
		block = fmBlock{present: true, body: string(raw)}
		changed = true
	}

	// --- name ---
	name, nameIdx := block.get("name")
	fixName := func(code, msg string) {
		canFix := fix && !slugReserved
		add(code, "error", msg, canFix)
		if canFix {
			block.set("name", dirSlug)
			changed = true
		}
	}
	switch {
	case nameIdx == -1 || name == "":
		fixName("E002", "name missing (want `name: "+dirSlug+"`)")
	case xmlTagRe.MatchString(name):
		fixName("E009", "name contains XML tags")
	case !skillNameRe.MatchString(name):
		fixName("E003", fmt.Sprintf("name %q must contain only lowercase letters, numbers, and hyphens", name))
	case len(name) > skillNameMaxLen:
		fixName("E004", fmt.Sprintf("name is %d chars (max %d)", len(name), skillNameMaxLen))
	case name != dirSlug:
		fixName("E006", fmt.Sprintf("name %q does not match skill directory %q", name, dirName))
	}
	if effectiveName, _ := block.get("name"); containsReservedWord(effectiveName) || slugReserved {
		add("E005", "error", fmt.Sprintf("skill name/directory contains a reserved word (%s) — rename the skill", strings.Join(reservedWords, ", ")), false)
	}

	// --- description ---
	desc, descIdx := block.get("description")
	multiline := strings.HasPrefix(desc, "|") || strings.HasPrefix(desc, ">")
	switch {
	case multiline:
		// Block scalars hold the real text on following lines; this
		// line-based linter can't analyze or safely rewrite them.
		add("W102", "warn", "multi-line description not analyzed; verify limits manually", false)
	case descIdx == -1 || desc == "":
		derived, ok := deriveSkillDescription(block.body, dirSlug)
		add("E007", "error", "description missing", fix)
		if fix {
			block.set("description", derived)
			changed = true
			if !ok {
				add("W102", "warn", "description is a TODO stub; describe what the skill does and when to use it", false)
			}
			desc = derived
		}
	default:
		if xmlTagRe.MatchString(desc) {
			add("E009", "error", "description contains XML tags", fix)
			if fix {
				desc = strings.TrimSpace(xmlTagRe.ReplaceAllString(desc, ""))
				block.set("description", desc)
				changed = true
			}
		}
		if len(desc) > skillDescriptionMaxLen {
			add("E008", "error", fmt.Sprintf("description is %d chars (max %d)", len(desc), skillDescriptionMaxLen), fix)
			if fix {
				desc = truncateAtWord(desc, skillDescriptionMaxLen)
				block.set("description", desc)
				changed = true
			}
		}
	}
	if !multiline && desc != "" {
		if firstPersonRe.MatchString(desc) {
			add("W101", "warn", "description should be third person (\"Processes X…\", not \"I can…\"/\"You can…\")", false)
		}
		if !whenClauseRe.MatchString(desc) {
			add("W102", "warn", "description should say when to use the skill (\"Use when …\")", false)
		}
	}

	// --- body ---
	if n := strings.Count(block.body, "\n"); n > skillBodyMaxLines {
		add("W103", "warn", fmt.Sprintf("body is %d lines (keep under %d; split into reference files)", n, skillBodyMaxLines), false)
	}

	if fix && changed {
		if err := writeFileAtomic(skillPath, []byte(block.render()), 0644); err != nil {
			return findings, err
		}
	}
	return findings, nil
}

// slugifySkillName converts an arbitrary directory name to a valid
// Claude skill name: lowercase, spaces/underscores → hyphens, other
// invalid characters dropped, runs of hyphens collapsed, trimmed, and
// truncated to the 64-char limit at a hyphen boundary when possible.
func slugifySkillName(s string) string {
	s = strings.ToLower(s)
	s = slugSeparators.ReplaceAllString(s, "-")
	s = slugInvalid.ReplaceAllString(s, "")
	s = slugCollapse.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > skillNameMaxLen {
		cut := s[:skillNameMaxLen]
		if i := strings.LastIndexByte(cut, '-'); i > 0 {
			cut = cut[:i]
		}
		s = strings.Trim(cut, "-")
	}
	return s
}

func containsReservedWord(name string) bool {
	for _, w := range reservedWords {
		if strings.Contains(name, w) {
			return true
		}
	}
	return false
}

// deriveSkillDescription extracts the first prose paragraph of the
// body as a one-line description, truncated to the limit. Returns
// ok=false (with a TODO stub) when the body has no usable prose.
func deriveSkillDescription(body, name string) (string, bool) {
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" ||
			strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "<!--") ||
			strings.HasPrefix(trimmed, "---") {
			continue
		}
		desc := strings.Join(strings.Fields(trimmed), " ")
		desc = strings.TrimSpace(xmlTagRe.ReplaceAllString(desc, ""))
		if desc == "" {
			continue
		}
		return truncateAtWord(desc, skillDescriptionMaxLen), true
	}
	return name + " skill. TODO: describe what it does and when to use it.", false
}

// truncateAtWord shortens s to at most max bytes, cutting at the last
// space when one exists in the tail.
func truncateAtWord(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut)
}

// writeFileAtomic writes via a temp file + rename in the target's
// directory so a crash never leaves a half-written SKILL.md.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".lint-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
