package agent

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
)

// This file renders artifact bodies as "## <name>" sections. Every
// delivery channel (SPEC-013) builds its index file and region body
// from these sections, so what status expects can never drift from
// what sync writes.

// ConcatEntry is one artifact's section in a rendered body. renderChannel
// builds one per channel Entry and buildEntriesBody writes it.
type ConcatEntry struct {
	// Name is the artifact's identifier, also the heading text in the
	// rendered body.
	Name string

	// SourcePath is the absolute path to the artifact's content file
	// under ~/.agents/. For rules and workflows, the .md file
	// itself; for skills, the SKILL.md inside the dir.
	SourcePath string
}

// buildEntriesBody renders entries as `## <name>` sections sorted by
// name, each followed by the artifact body with frontmatter stripped.
// The artifact's YAML frontmatter is sync-agents metadata, not content
// the tool should see, so only the body is written.
func buildEntriesBody(entries []ConcatEntry) ([]byte, error) {
	// Sort by Name so the output is deterministic regardless of how
	// the orchestration layer discovered the entries.
	sorted := make([]ConcatEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var buf bytes.Buffer
	for _, e := range sorted {
		body, err := readArtifactBody(e.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.SourcePath, err)
		}
		// SPEC-006: an OS-scoped artifact ("macos/brew") gets a header
		// comment so a reader of the flat bundle knows which platform
		// the block targets. Invisible when the tool renders Markdown.
		if scope, _, ok := strings.Cut(e.Name, "/"); ok && isOSScopeDir(scope) {
			fmt.Fprintf(&buf, "<!-- OS: %s -->\n", scope)
		}
		fmt.Fprintf(&buf, "## %s\n\n", e.Name)
		buf.Write(body)
		// Always end an entry on a blank line so the next heading
		// starts cleanly. We trim the body's trailing whitespace
		// first to avoid stacking blank lines if the source already
		// ended with one.
		if !bytes.HasSuffix(bytes.TrimRight(buf.Bytes(), " \t"), []byte("\n")) {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// readArtifactBody returns the body of an artifact file with any
// leading YAML frontmatter stripped.
//
// Detection: a file begins with `---\n` or `---\r\n`, and the
// frontmatter ends at the next `---` on its own line. Anything after
// the closing delimiter is the body. Files without frontmatter
// return their full content.
//
// The body's leading blank lines are trimmed so rendered output doesn't
// stack empty lines after the heading. Trailing whitespace is
// preserved.
func readArtifactBody(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(raw, []byte("---\n")) && !bytes.HasPrefix(raw, []byte("---\r\n")) {
		return raw, nil
	}

	text := string(raw)
	if strings.HasPrefix(text, "---\r\n") {
		text = text[5:]
	} else {
		text = text[4:]
	}

	endIdx := indexFrontmatterEnd(text)
	if endIdx == -1 {
		// No closing delimiter — treat the whole file as content
		// rather than swallowing it silently. The section will look
		// a bit odd but the user can see why.
		return raw, nil
	}

	// Skip the closing `---` line itself.
	afterClose := text[endIdx:]
	if i := strings.IndexByte(afterClose, '\n'); i != -1 {
		afterClose = afterClose[i+1:]
	} else {
		afterClose = ""
	}

	body := strings.TrimLeft(afterClose, "\n\r ")
	return []byte(body), nil
}
