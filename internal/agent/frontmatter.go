package agent

import (
	"fmt"
	"strings"
)

// fmBlock is a line-based view of a file's YAML frontmatter. Unlike
// a full YAML parse, it preserves every raw line inside the block, so
// a rewrite only touches the specific `key: value` lines we edit —
// unknown keys, comments, lists, and multi-line scalars survive a
// round-trip verbatim. This is the safety property `lint --fix`
// depends on (SPEC-004 Part E: "unknown frontmatter keys preserved").
type fmBlock struct {
	// present is false when the file has no frontmatter at all.
	present bool

	// lines are the raw lines between the `---` delimiters,
	// excluding the delimiters themselves.
	lines []string

	// body is everything after the closing `---` line (or the whole
	// file when present is false), byte-for-byte.
	body string
}

// parseFMBlock splits content into frontmatter block + body using the
// same delimiter rules as ParseFrontmatterInvocable: the opening
// `---` must be the very first line; the closing `---` must start a
// line. Returns an error for an unterminated block.
func parseFMBlock(content string) (fmBlock, error) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return fmBlock{present: false, body: content}, nil
	}

	text := content
	if strings.HasPrefix(text, "---\r\n") {
		text = text[5:]
	} else {
		text = text[4:]
	}

	endIdx := indexFrontmatterEnd(text)
	if endIdx == -1 {
		return fmBlock{}, fmt.Errorf("frontmatter is not terminated: missing closing `---`")
	}

	fm := text[:endIdx]
	// endIdx points at the start of the closing `---` line; skip past
	// it (and its newline) to find the body.
	rest := text[endIdx:]
	if i := strings.IndexByte(rest, '\n'); i != -1 {
		rest = rest[i+1:]
	} else {
		rest = ""
	}

	var lines []string
	if fm != "" {
		lines = strings.Split(strings.TrimSuffix(fm, "\n"), "\n")
	}
	return fmBlock{present: true, lines: lines, body: rest}, nil
}

// get returns the scalar value of a top-level `key: value` line,
// with surrounding whitespace, one layer of quotes, and any inline
// `#` comment stripped. Indented lines are skipped (they belong to
// nested structures we don't edit). The second return is the line
// index, -1 when the key is absent.
func (b fmBlock) get(key string) (string, int) {
	for i, line := range b.lines {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Top-level keys only: a leading space/tab means nesting.
		if strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "\t") {
			continue
		}
		colon := strings.IndexByte(trimmed, ':')
		if colon == -1 || strings.TrimSpace(trimmed[:colon]) != key {
			continue
		}
		val := strings.TrimSpace(trimmed[colon+1:])
		if hash := strings.IndexByte(val, '#'); hash != -1 && !strings.HasPrefix(val, `"`) && !strings.HasPrefix(val, `'`) {
			val = strings.TrimSpace(val[:hash])
		}
		return stripQuotes(val), i
	}
	return "", -1
}

// set replaces the key's line in place, or appends a new line to the
// end of the block when the key is absent. Values that would confuse
// a YAML parser as a plain scalar are double-quoted.
func (b *fmBlock) set(key, value string) {
	line := key + ": " + quoteYAMLScalar(value)
	if _, i := b.get(key); i != -1 {
		b.lines[i] = line
		return
	}
	b.lines = append(b.lines, line)
}

// render reassembles the file: delimiters + block lines + body.
func (b *fmBlock) render() string {
	var sb strings.Builder
	sb.WriteString("---\n")
	for _, line := range b.lines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(b.body)
	return sb.String()
}

// quoteYAMLScalar wraps value in double quotes when it contains
// characters that change meaning in a plain YAML scalar. Plain values
// pass through untouched to keep diffs minimal.
func quoteYAMLScalar(value string) string {
	needsQuote := strings.ContainsAny(value, ":#\"'") ||
		strings.TrimSpace(value) != value ||
		strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") ||
		strings.HasPrefix(value, "-") || value == ""
	if !needsQuote {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
