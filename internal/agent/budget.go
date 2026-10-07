package agent

import "unicode/utf16"

// This file sizes delivery bundles against each tool's load limit
// (SPEC-013 §Budget). A tool that truncates keeps some arbitrary part
// of an oversized file: OpenClaw keeps head and tail and drops the
// middle, Codex stops at its byte limit. sync-agents therefore never
// hands a tool more than it keeps. Rules that do not fit become
// one-line pointers to their source, whole and named, instead of being
// cut mid-rule by the tool.

// CapUnit is what a tool counts when it enforces a size limit.
type CapUnit int

const (
	// CapBytes: UTF-8 bytes (Codex project_doc_max_bytes).
	CapBytes CapUnit = iota

	// CapUTF16: JavaScript string length, in UTF-16 code units
	// (OpenClaw bootstrapMaxChars, Windsurf's 6,000 characters). A rune
	// count undercounts: an emoji outside the Basic Multilingual Plane
	// is one rune but two units.
	CapUTF16
)

// Cap is a tool's limit on what it loads. The zero Cap is unlimited.
type Cap struct {
	Limit int
	Unit  CapUnit

	// Knob names the setting that raises Limit, quoted in warnings.
	// Empty when the vendor fixes the limit.
	Knob string
}

// Measure returns the size of b in c's unit. It is the only sizing
// function for delivery content, so a limit is never compared against
// a size in another unit.
func (c Cap) Measure(b []byte) int {
	if c.Unit != CapUTF16 {
		return len(b)
	}
	n := 0
	for _, r := range string(b) {
		n += utf16.RuneLen(r)
	}
	return n
}

// Budget is a tool's limit plus what other loaded content already uses
// of it, in the same unit.
type Budget struct {
	Cap Cap

	// Reserved charges what the tool loads under the same limit besides
	// this channel's bytes: the host's text outside our region, and,
	// for the Codex project override, the global file's size.
	Reserved int
}

// BudgetSource resolves a channel's Budget from the tool's own config.
// Reserved covers other files the tool counts against the same limit;
// the shell adds the bytes outside a RegionMount's markers.
type BudgetSource func(tc ToolContext) (Budget, error)

// section is one entry rendered both ways: in full, and as the pointer
// that replaces it when the full block does not fit or the entry is on
// demand.
type section struct {
	Name     string
	OnDemand bool
	Block    []byte // "<!-- OS: macos -->\n## macos/brew\n\n<body>\n\n"
	Pointer  []byte // "- macos/brew (.agents/rules/macos/brew.md): Homebrew conventions\n"
}

// pointerHeading opens the list of demoted entries.
const pointerHeading = "## Not inlined\n\nThese are not loaded here because of this tool's size limit or because they apply only to some tasks. Read one when it applies:\n\n"

// fitBudget decides which sections are inlined, first fit in name
// order (SPEC-013 §Budget, T4).
//
//   - An unlimited cap inlines everything, OnDemand included.
//   - When everything except the OnDemand sections fits whole, that is
//     the answer (with the OnDemand ones as pointers).
//   - Otherwise at least one pointer is needed, so the pointer heading
//     is charged up front, and each non-OnDemand section is inlined
//     when fixed, plus what is already placed, plus this block, plus
//     the pointers still owed for the rest, stays within the limit.
//     A section that does not fit is demoted to its pointer and the
//     walk continues, so a later, smaller rule can still fit.
//   - When even the all-pointer form overflows, every section is a
//     pointer and the caller warns that the tool will truncate.
//
// fixed is the size of everything outside the sections (frontmatter,
// banner, Reserved) in c's unit. The result keeps name order.
func fitBudget(sections []section, fixed int, c Cap) (inlined, pointers []section) {
	if c.Limit == 0 {
		return sections, nil
	}

	whole := fixed
	anyOnDemand := false
	for _, s := range sections {
		if s.OnDemand {
			whole += c.Measure(s.Pointer)
			anyOnDemand = true
		} else {
			whole += c.Measure(s.Block)
		}
	}
	if anyOnDemand {
		whole += c.Measure([]byte(pointerHeading))
	}
	if whole <= c.Limit {
		for _, s := range sections {
			if s.OnDemand {
				pointers = append(pointers, s)
			} else {
				inlined = append(inlined, s)
			}
		}
		return inlined, pointers
	}

	used := fixed + c.Measure([]byte(pointerHeading))
	owed := 0
	for _, s := range sections {
		owed += c.Measure(s.Pointer)
	}
	if used+owed > c.Limit {
		return nil, sections
	}
	for _, s := range sections {
		p := c.Measure(s.Pointer)
		owed -= p
		if b := c.Measure(s.Block); !s.OnDemand && used+b+owed <= c.Limit {
			inlined = append(inlined, s)
			used += b
			continue
		}
		pointers = append(pointers, s)
		used += p
	}
	return inlined, pointers
}

// windsurfGlobalRulesMaxChars is Windsurf's limit on global_rules.md.
const windsurfGlobalRulesMaxChars = 6000

// windsurfGlobalBudget is the codeium global budget. The vendor fixes
// the limit, so Knob is empty.
func windsurfGlobalBudget(ToolContext) (Budget, error) {
	return Budget{Cap: Cap{Limit: windsurfGlobalRulesMaxChars, Unit: CapUTF16}}, nil
}

// openClawBudget is bootstrapMaxChars from openclaw.json (default
// 20,000), counted in UTF-16 units because OpenClaw measures
// JavaScript string length. It covers the whole workspace AGENTS.md;
// the shell reserves the host's bytes outside our region.
func openClawBudget(tc ToolContext) (Budget, error) {
	layout, err := resolveOpenClaw(tc.Parent, tc.Env)
	if err != nil {
		return Budget{}, err
	}
	return Budget{Cap: Cap{
		Limit: layout.BootstrapMaxChars,
		Unit:  CapUTF16,
		Knob:  "agents.defaults.bootstrapMaxChars in openclaw.json",
	}}, nil
}
