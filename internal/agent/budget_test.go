package agent

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestBudgetCapMeasure counts UTF-8 bytes for Codex and UTF-16 units
// for OpenClaw and Windsurf, where an astral-plane emoji is two units
// (a rune count would say one).
func TestBudgetCapMeasure(t *testing.T) {
	cases := []struct {
		in           string
		bytes, units int
	}{
		{"", 0, 0},
		{"abc", 3, 3},
		{"é", 2, 1},
		{"中", 3, 1},
		{"😀", 4, 2},
		{"a😀b\n", 7, 5},
	}
	for _, tc := range cases {
		if got := (Cap{Unit: CapBytes}).Measure([]byte(tc.in)); got != tc.bytes {
			t.Errorf("bytes(%q) = %d, want %d", tc.in, got, tc.bytes)
		}
		if got := (Cap{Unit: CapUTF16}).Measure([]byte(tc.in)); got != tc.units {
			t.Errorf("utf16(%q) = %d, want %d", tc.in, got, tc.units)
		}
	}
}

// sec builds a section whose block and pointer have the given sizes.
func sec(name string, block, pointer int, onDemand bool) section {
	return section{
		Name:     name,
		OnDemand: onDemand,
		Block:    []byte(strings.Repeat("b", block)),
		Pointer:  []byte(strings.Repeat("p", pointer)),
	}
}

func sectionNames(ss []section) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.Name)
	}
	return out
}

// TestBudgetFitBudget covers the first-fit edges: unlimited, exact fit,
// one byte short, skipping a big rule for a later small one, the
// model_decision lever, all pointers, and overflow even as pointers.
func TestBudgetFitBudget(t *testing.T) {
	h := len(pointerHeading)
	cases := []struct {
		name         string
		sections     []section
		fixed, limit int
		in, out      []string
	}{
		{
			name:     "unlimited inlines everything, on-demand too",
			sections: []section{sec("a", 50, 5, false), sec("b", 50, 5, true)},
			limit:    0,
			in:       []string{"a", "b"},
		},
		{
			name:     "exact fit inlines everything",
			sections: []section{sec("a", 400, 5, false), sec("b", 300, 5, false)},
			fixed:    30,
			limit:    730,
			in:       []string{"a", "b"},
		},
		{
			name:     "one unit short demotes",
			sections: []section{sec("a", 400, 5, false), sec("b", 300, 5, false)},
			fixed:    30,
			limit:    729,
			in:       []string{"a"},
			out:      []string{"b"},
		},
		{
			name:     "a big rule is skipped and a later small one still fits",
			sections: []section{sec("a", 500, 5, false), sec("b", 1000, 5, false), sec("c", 20, 5, false)},
			fixed:    10,
			limit:    10 + h + 500 + 5 + 20,
			in:       []string{"a", "c"},
			out:      []string{"b"},
		},
		{
			name:     "room is kept for the pointers still owed",
			sections: []section{sec("a", 300, 10, false), sec("b", 300, 10, false)},
			limit:    h + 305,
			out:      []string{"a", "b"},
		},
		{
			name:     "model_decision is a pointer in a capped bundle even when it fits",
			sections: []section{sec("a", 10, 5, false), sec("b", 10, 5, true)},
			limit:    1000,
			in:       []string{"a"},
			out:      []string{"b"},
		},
		{
			name:     "all pointers fit exactly",
			sections: []section{sec("a", 100, 7, false), sec("b", 100, 9, false)},
			fixed:    4,
			limit:    4 + h + 7 + 9,
			out:      []string{"a", "b"},
		},
		{
			name:     "overflow even as pointers demotes everything",
			sections: []section{sec("a", 1, 7, false), sec("b", 1, 9, false)},
			fixed:    50,
			limit:    40,
			out:      []string{"a", "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, out := fitBudget(tc.sections, tc.fixed, Cap{Limit: tc.limit, Unit: CapBytes})
			if !reflect.DeepEqual(sectionNames(in), tc.in) || !reflect.DeepEqual(sectionNames(out), tc.out) {
				t.Errorf("in=%v out=%v, want in=%v out=%v", sectionNames(in), sectionNames(out), tc.in, tc.out)
			}
		})
	}
}

// TestBudgetRender_NeverExceedsWhenPointersFit sweeps every limit from
// the all-pointer size up to the uncapped size and checks the rendered
// file never exceeds the cap: sync stays under the limit so no tool
// truncates mid-rule.
func TestBudgetRender_NeverExceedsWhenPointersFit(t *testing.T) {
	entries := channelFixture(t)
	for _, unit := range []CapUnit{CapBytes, CapUTF16} {
		full, err := renderChannel(FormatCopilotInstructions, entries, Frame{Banner: localBanner}, Budget{})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		for limit := 1; limit <= len(full.Bytes)+5; limit++ {
			b := Budget{Cap: Cap{Limit: limit, Unit: unit}, Reserved: 3}
			r, err := renderChannel(FormatCopilotInstructions, entries, Frame{Banner: localBanner}, b)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if r.OverCap() {
				if len(r.Inlined) != 0 {
					t.Errorf("limit %d: over cap with %v still inlined", limit, r.Inlined)
				}
				continue
			}
			if r.Size > limit {
				t.Errorf("limit %d: size %d", limit, r.Size)
			}
			if len(r.Inlined)+len(r.Pointers) != len(entries) {
				t.Errorf("limit %d: lost entries: %v + %v", limit, r.Inlined, r.Pointers)
			}
		}
	}
}

// TestBudgetRender_PointerGolden pins the capped layout: inlined
// sections, then the pointer section with path and description.
func TestBudgetRender_PointerGolden(t *testing.T) {
	entries := channelFixture(t)
	entries[1].OnDemand = true // security
	r, err := renderChannel(FormatMarkdown, entries, Frame{Banner: globalBanner}, Budget{Cap: Cap{Limit: 10000, Unit: CapUTF16}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := globalBanner + "\n\n" +
		"<!-- OS: macos -->\n## macos/brew\n\nUse brew.\n\n" +
		"## testing\n\nWrite tests.\n\n" +
		pointerHeading +
		"- security (.agents/rules/security.md): Never leak secrets\n"
	if got := string(r.Bytes); got != want {
		t.Errorf("bytes mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if !reflect.DeepEqual(r.Pointers, []string{"security"}) || !reflect.DeepEqual(r.Inlined, []string{"macos/brew", "testing"}) {
		t.Errorf("inlined=%v pointers=%v", r.Inlined, r.Pointers)
	}
}

// TestBudgetRender_UTF16Emoji fits an emoji-heavy rule under a UTF-16
// cap that its byte size would exceed, and demotes it under a byte cap
// of the same number.
func TestBudgetRender_UTF16Emoji(t *testing.T) {
	src := writeArtifact(t, t.TempDir(), "rules/emoji.md", strings.Repeat("😀", 100)+"\n")
	entries := []Entry{{Name: "emoji", Source: src, Display: ".agents/rules/emoji.md"}}
	whole, err := renderChannel(FormatMarkdown, entries, Frame{Banner: localBanner}, Budget{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	limit := (Cap{Unit: CapUTF16}).Measure(whole.Bytes)
	if len(whole.Bytes) <= limit {
		t.Fatalf("fixture: %d bytes should exceed %d units", len(whole.Bytes), limit)
	}

	r, err := renderChannel(FormatMarkdown, entries, Frame{Banner: localBanner}, Budget{Cap: Cap{Limit: limit, Unit: CapUTF16}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(r.Inlined) != 1 || r.Size != limit {
		t.Errorf("utf16: inlined=%v size=%d, want inlined at exactly %d", r.Inlined, r.Size, limit)
	}

	r, err = renderChannel(FormatMarkdown, entries, Frame{Banner: localBanner}, Budget{Cap: Cap{Limit: limit, Unit: CapBytes}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(r.Pointers) != 1 {
		t.Errorf("bytes: pointers=%v, want the rule demoted", r.Pointers)
	}
}

// TestBudgetRender_Reserved charges other content loaded under the same
// cap: what fits alone is demoted once Reserved is added, and Size
// includes Reserved.
func TestBudgetRender_Reserved(t *testing.T) {
	dir := t.TempDir()
	var entries []Entry
	for _, n := range []string{"a", "b", "c"} {
		src := writeArtifact(t, dir, "rules/"+n+".md", strings.Repeat(n, 500)+"\n")
		entries = append(entries, Entry{Name: n, Source: src, Display: ".agents/rules/" + n + ".md"})
	}
	whole, err := renderChannel(FormatMarkdown, entries, Frame{Banner: localBanner}, Budget{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	limit := len(whole.Bytes)

	r, err := renderChannel(FormatMarkdown, entries, Frame{Banner: localBanner}, Budget{Cap: Cap{Limit: limit}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(r.Pointers) != 0 || r.Size != limit {
		t.Fatalf("no reserve: pointers=%v size=%d", r.Pointers, r.Size)
	}

	r, err = renderChannel(FormatMarkdown, entries, Frame{Banner: localBanner}, Budget{Cap: Cap{Limit: limit}, Reserved: 1})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(r.Pointers) == 0 || r.Size > limit || r.Size != len(r.Bytes)+1 {
		t.Errorf("reserve 1: pointers=%v size=%d bytes=%d limit=%d", r.Pointers, r.Size, len(r.Bytes), limit)
	}

	r, err = renderChannel(FormatMarkdown, entries, Frame{Banner: localBanner}, Budget{Cap: Cap{Limit: limit}, Reserved: limit})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.OverCap() || len(r.Inlined) != 0 {
		t.Errorf("reserve = limit: overCap=%v inlined=%v", r.OverCap(), r.Inlined)
	}
}

// TestBudgetFixedSources pins the vendor limits for Windsurf and
// OpenClaw, including openclaw.json's bootstrapMaxChars override.
func TestBudgetFixedSources(t *testing.T) {
	w, err := windsurfGlobalBudget(ToolContext{})
	if err != nil || w.Cap != (Cap{Limit: 6000, Unit: CapUTF16}) {
		t.Errorf("windsurf = %+v, %v", w, err)
	}

	o, err := openClawBudget(ToolContext{Parent: "/home/u", Env: mapEnv(nil, nil)})
	if err != nil || o.Cap.Limit != 20000 || o.Cap.Unit != CapUTF16 || o.Cap.Knob == "" {
		t.Errorf("openclaw default = %+v, %v", o, err)
	}

	cfg := filepath.Join("/home/u", ".openclaw", "openclaw.json")
	o, err = openClawBudget(ToolContext{Parent: "/home/u", Env: mapEnv(nil, map[string]string{
		cfg: `{"agents":{"defaults":{"bootstrapMaxChars":12345}}}`,
	})})
	if err != nil || o.Cap.Limit != 12345 {
		t.Errorf("openclaw configured = %+v, %v", o, err)
	}

	if _, err := openClawBudget(ToolContext{Parent: "/home/u", Env: mapEnv(nil, map[string]string{cfg: "{"})}); err == nil {
		t.Error("openclaw: unparseable config should be an error")
	}
}
