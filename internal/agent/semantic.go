package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Semantic is the *behavioral category* of an artifact — distinct
// from its bucket (rule/skill/workflow). See SPEC-002 §Semantic
// categories (bucket ≠ semantic) for the conceptual model and
// docs/architecture/semantic-routing.md for prose-level discussion.
//
// Two semantics:
//
//   - Invocable: the artifact is loaded only when triggered by name
//     — a user slash command, or an AI model's tool-call decision
//     based on the artifact's trigger description. Not in baseline
//     context.
//   - Passive: the artifact is part of baseline context for every
//     conversation. No trigger logic; always loaded.
//
// Each tool's per-semantic destination is encoded in SPEC-002's
// routing map. The Semantic value drives that lookup at sync time.
type Semantic string

const (
	// Invocable marks artifacts that are loaded on demand by name.
	Invocable Semantic = "invocable"

	// Passive marks artifacts that are always part of baseline context.
	Passive Semantic = "passive"

	// Reference marks documents that are neither preloaded nor
	// trigger-dispatched: plans and specs (SPEC-004 Part D). They are
	// indexed in AGENTS.md and reachable on demand (@-mention, file
	// read) but never enter the managed import block or any
	// invocable surface.
	Reference Semantic = "reference"
)

// String returns the lowercase semantic name. Useful for log messages
// and the `--semantic=` flag of any future inspector command. Returns
// the empty string for the zero value, which callers should treat as
// "unset" rather than passing it through.
func (s Semantic) String() string {
	return string(s)
}

// BucketDefaultSemantic returns the default Semantic for an artifact
// of the given bucket when its frontmatter does not declare
// invocable: explicitly.
//
// Defaults per SPEC-002 §Frontmatter contract:
//
//	rules/     → Passive   (always-on context)
//	skills/    → Invocable (loaded by trigger)
//	workflows/ → Invocable (loaded by slash invocation)
//	agents/    → Invocable (subagents are dispatched by name/trigger)
//	plans/     → Reference (indexed, read on demand)
//	specs/     → Reference (indexed, read on demand)
//	adrs/      → Reference (indexed by status, read on demand)
//
// An unknown ArtifactType returns Passive — the conservative choice,
// since at worst it ends up in always-on context rather than the
// possibly-empty invocable surface.
func BucketDefaultSemantic(typ ArtifactType) Semantic {
	switch typ {
	case ArtifactRule:
		return Passive
	case ArtifactSkill, ArtifactWorkflow, ArtifactAgent:
		return Invocable
	case ArtifactPlan, ArtifactSpec, ArtifactADR:
		return Reference
	default:
		return Passive
	}
}

// ParseFrontmatterInvocable inspects a byte slice for YAML frontmatter
// and returns information about the `invocable:` key:
//
//   - (val, true, nil) if the frontmatter contains a well-formed
//     `invocable: true` or `invocable: false`.
//   - (false, false, nil) if there is no frontmatter, or the
//     frontmatter exists but contains no `invocable:` key.
//   - (false, false, err) if the frontmatter is malformed (no closing
//     `---`) or `invocable:` has a non-boolean value.
//
// The parser is intentionally minimal — it understands `key: value`
// lines, blank lines, and `#` comments. It does NOT understand
// nested mappings, multi-line scalars, lists, anchors, or aliases.
// Frontmatter we encounter in agent artifacts is consistently flat,
// so a full YAML library is overkill and adds a dependency we'd
// rather avoid.
//
// Recognised invocable values (after whitespace + optional quote
// stripping):
//
//	true   → val=true
//	false  → val=false
//
// Anything else under the `invocable:` key (`yes`, `1`, `maybe`)
// triggers an error so a typo doesn't silently flip routing.
//
// The parser tolerates both LF (`\n`) and CRLF (`\r\n`) line endings.
// The frontmatter delimiter must be `---` on its own line at the
// very start of the file; leading whitespace or blank lines disqualify
// the file from having frontmatter at all (returns present=false with
// no error).
func ParseFrontmatterInvocable(content []byte) (val bool, present bool, err error) {
	// Quick reject: must begin with `---\n` or `---\r\n`.
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return false, false, nil
	}

	// Normalize: strip the opening `---` line. After this, `text` is
	// everything after the first newline.
	text := string(content)
	if strings.HasPrefix(text, "---\r\n") {
		text = text[5:]
	} else {
		text = text[4:]
	}

	// Find the closing `---` line. It MUST appear at the start of a
	// line. We search for `\n---` so the previous character is a
	// newline boundary.
	endIdx := indexFrontmatterEnd(text)
	if endIdx == -1 {
		return false, false, fmt.Errorf("frontmatter is not terminated: missing closing `---`")
	}

	fm := text[:endIdx]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimRight(line, "\r") // CRLF tolerance
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}

		colon := strings.IndexByte(stripped, ':')
		if colon == -1 {
			// Not a key:value line. Skip silently — frontmatter may
			// contain values we don't care about and we shouldn't
			// reject the file because of them.
			continue
		}

		key := strings.TrimSpace(stripped[:colon])
		if key != "invocable" {
			continue
		}

		valStr := strings.TrimSpace(stripped[colon+1:])
		// Strip a trailing inline comment if present (`true # note`).
		if hash := strings.IndexByte(valStr, '#'); hash != -1 {
			valStr = strings.TrimSpace(valStr[:hash])
		}
		// Strip a single layer of matching quotes.
		valStr = stripQuotes(valStr)
		switch valStr {
		case "true":
			return true, true, nil
		case "false":
			return false, true, nil
		default:
			return false, false, fmt.Errorf("invocable: must be `true` or `false`, got %q", valStr)
		}
	}

	return false, false, nil
}

// ResolveSemantic returns the Semantic of the artifact at `path`,
// taking into account both the bucket's default and any frontmatter
// override.
//
// For ArtifactSkill, the metadata lives in SKILL.md inside the skill
// directory. ResolveSemantic accepts either form — a path to the
// directory, in which case SKILL.md inside it is read; or a direct
// path to SKILL.md.
//
// For ArtifactRule and ArtifactWorkflow, `path` is the .md file
// directly.
//
// Errors:
//
//   - The file is missing → returns a wrapped os error. Callers that
//     want a "default if missing" semantic should check for not-exist
//     and fall back to BucketDefaultSemantic themselves; conflating
//     missing-file and no-frontmatter at this layer would hide bugs.
//   - Frontmatter is malformed or `invocable:` is non-boolean →
//     returns an error wrapping the file path so the user can find it.
//
// See docs/architecture/semantic-routing.md for the full conceptual
// model and SPEC-002 §Semantic-aware routing for the requirement
// this implements.
func ResolveSemantic(path string, typ ArtifactType) (Semantic, error) {
	actualFile := path
	if typ == ArtifactSkill {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			actualFile = filepath.Join(path, "SKILL.md")
		}
	}

	content, err := os.ReadFile(actualFile)
	if err != nil {
		return "", fmt.Errorf("read artifact %s: %w", actualFile, err)
	}

	val, present, err := ParseFrontmatterInvocable(content)
	if err != nil {
		return "", fmt.Errorf("frontmatter in %s: %w", actualFile, err)
	}
	if !present {
		return BucketDefaultSemantic(typ), nil
	}
	if val {
		return Invocable, nil
	}
	return Passive, nil
}

// indexFrontmatterEnd returns the index in `text` of the newline
// immediately preceding the closing `---` delimiter, or -1 if the
// delimiter is not present.
//
// The closing delimiter must appear at the start of a line. Splitting
// on `\n---\n` would miss the EOF case where the delimiter is the
// last line of the file with no trailing newline. We handle both.
func indexFrontmatterEnd(text string) int {
	// Search for the `---` delimiter on its own line. We accept it
	// followed by either a newline, a CR, or EOF.
	idx := 0
	for idx < len(text) {
		nl := strings.IndexByte(text[idx:], '\n')
		if nl == -1 {
			return -1
		}
		lineStart := idx
		lineEnd := idx + nl
		line := text[lineStart:lineEnd]
		line = strings.TrimRight(line, "\r")
		if line == "---" {
			return lineStart
		}
		idx = lineEnd + 1
	}
	// Last-line case: no trailing newline.
	tail := text[idx:]
	tail = strings.TrimRight(tail, "\r")
	if tail == "---" {
		return idx
	}
	return -1
}

// stripQuotes removes a single layer of matching `"..."` or `'...'`
// from the input. Returns the original string if quotes don't match
// or aren't present. Used to accept `invocable: "true"` and
// `invocable: 'false'` in addition to the bare forms.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
