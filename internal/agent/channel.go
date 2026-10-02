package agent

import (
	"bytes"
	"os"
	"sort"
	"strings"
)

// This file is the pure core of SPEC-013's delivery channels: what each
// tool receives at each scope, in which dialect, through which kind of
// mount, and under which size limit. Nothing here writes a file or
// decides what exists on disk; the shell around it (deliver.go) observes
// the filesystem, asks mayMount, renders through renderChannel, and
// mounts. Size limits and first-fit demotion live in budget.go, the
// Codex layout in codex.go, and the opencode.json edit in jsonentry.go.

// ChannelSpec is the static, vendor-derived description of how one tool
// receives aggregated passive content at one scope. It is pure data: no
// IO, no absolute paths.
//
// A tool/scope pair with no spec gets passive content another way
// (Claude and Windsurf read the folded .agents/rules directory) or not
// at all (channelGaps says why).
type ChannelSpec struct {
	// Format is the dialect of the generated index file.
	Format Format

	// Mount is how the tool's native read path reaches the content. Its
	// kind also decides consent (mayMount).
	Mount Mount

	// Budget reports the tool's size limit and what else already uses
	// it. Nil means the vendor documents no limit.
	Budget BudgetSource

	// ShadowedBy, when set, names a file the tool reads instead of the
	// mount, or returns "" (Codex prefers ~/.codex/AGENTS.override.md
	// over the AGENTS.md that carries our region).
	ShadowedBy func(tc ToolContext) string

	// MergeGlobal (local scope only): the index also carries the global
	// root's passive entries, project entries winning on Name. Set for a
	// tool with no global file surface (Cursor), so global rules reach
	// it through each project. Applied only under `index = local`: a
	// committed index must never carry one developer's personal rules.
	MergeGlobal bool
}

// channelSpecs is the per-tool delivery table (SPEC-013 §Per-tool
// delivery). Every row is a vendor-documented read path. When a vendor
// moves a path, or a tool turns out not to follow symlinks, the row
// changes and nothing else in the package does.
//
// Keys are canonical Tool IDs. Claude has no row: it reads
// .claude/rules natively. Windsurf has no local row for the same
// reason (.windsurf/rules). OpenClaw has no local row: it is a
// global-only target (SPEC-012).
var channelSpecs = map[string]map[Scope]ChannelSpec{
	"codeium": {
		// Windsurf loads global_rules.md always-on and keeps 6,000
		// characters of it. Its UI edits the same file, so this is a
		// region, not an owned file or a symlink.
		ScopeGlobal: {
			Format: FormatMarkdown,
			Mount: RegionMount{
				Host:       NativePath{AnchorHome, "windsurf/memories/global_rules.md"}, // home: ~/.codeium
				Region:     CodeiumRulesRegion,
				CreateHost: true,
			},
			Budget: windsurfGlobalBudget,
		},
	},
	"cursor": {
		// Cursor reads .mdc rules only; .md files in .cursor/rules are
		// ignored. There is no user-scope rules file (channelGaps), so
		// the project index also carries global rules.
		ScopeLocal: {
			Format:      FormatCursorMDC,
			Mount:       LinkMount{At: NativePath{AnchorBase, ".cursor/rules/sync-agents.mdc"}},
			MergeGlobal: true,
		},
	},
	"copilot": {
		// An extra file in the instructions directories, never the
		// single copilot-instructions.md, which teams write by hand.
		ScopeLocal: {
			Format: FormatCopilotInstructions,
			Mount:  LinkMount{At: NativePath{AnchorBase, ".github/instructions/sync-agents.instructions.md"}},
		},
		ScopeGlobal: {
			Format: FormatCopilotInstructions,
			Mount:  LinkMount{At: NativePath{AnchorHome, "instructions/sync-agents.instructions.md"}}, // home: ~/.copilot
		},
	},
	"codex": {
		// Project: Codex reads AGENTS.override.md instead of AGENTS.md in
		// a directory (one file per directory), and no other tool reads
		// the override, so it is a Codex-only channel.
		ScopeLocal: {
			Format: FormatCodexOverride,
			Mount:  LinkMount{At: NativePath{AnchorBase, "AGENTS.override.md"}},
			Budget: codexProjectBudget,
		},
		// Global: a region in ~/.codex/AGENTS.md. No global override: the
		// user's own text stays in the file Codex reads.
		ScopeGlobal: {
			Format: FormatMarkdown,
			Mount: RegionMount{
				Host:       NativePath{AnchorHome, "AGENTS.md"}, // home: $CODEX_HOME or ~/.codex
				Region:     CodexRulesRegion,
				CreateHost: true,
			},
			Budget:     codexGlobalBudget,
			ShadowedBy: codexGlobalOverride,
		},
	},
	"opencode": {
		// opencode loads the files listed in opencode.json
		// "instructions" in addition to AGENTS.md. A config entry leaves
		// its CLAUDE.md fallbacks working; creating
		// ~/.config/opencode/AGENTS.md would switch them off.
		ScopeLocal: {
			Format: FormatMarkdown,
			Mount:  ConfigListMount{File: NativePath{AnchorBase, "opencode.json"}, Key: "instructions"},
		},
		ScopeGlobal: {
			Format: FormatMarkdown,
			Mount:  ConfigListMount{File: NativePath{AnchorHome, "opencode.json"}, Key: "instructions"}, // home: ~/.config/opencode
		},
	},
	"openclaw": {
		// OpenClaw reads <workspace>/AGENTS.md as raw text and nothing
		// else for instructions. The host is OpenClaw's bootstrap file,
		// so sync never creates it (SPEC-012 consent).
		ScopeGlobal: {
			Format: FormatMarkdown,
			Mount:  RegionMount{Host: NativePath{AnchorHome, "AGENTS.md"}, Region: OpenClawRulesRegion},
			Budget: openClawBudget,
		},
	},
}

// channelGaps explains, in status output, why a registered tool has no
// channel at a scope where users expect one.
var channelGaps = map[string]map[Scope]string{
	"cursor": {ScopeGlobal: "Cursor keeps user rules in app settings, not in files; global rules reach Cursor through each project's .cursor/rules/sync-agents.mdc when index = local"},
}

// Regions owned by channels, one per (tool, scope). OpenClawRulesRegion
// (region.go) keeps its SPEC-012 name.
var (
	// CodexRulesRegion carries the global rules in ~/.codex/AGENTS.md.
	CodexRulesRegion = ManagedRegion{Name: "codex-rules"}

	// CodeiumRulesRegion carries the global rules in Windsurf's
	// global_rules.md.
	CodeiumRulesRegion = ManagedRegion{Name: "codeium-rules"}
)

// Format is the dialect of a channel's index file. Every format carries
// the same entries; only the frame around them differs.
type Format int

const (
	// FormatMarkdown: the banner, one "## <name>" section per inlined
	// entry, then the pointer section. Region bodies use it as is.
	FormatMarkdown Format = iota

	// FormatCursorMDC: MDC frontmatter (description, alwaysApply: true),
	// then FormatMarkdown. Cursor ignores a rule file without
	// alwaysApply or globs unless the agent picks it.
	FormatCursorMDC

	// FormatCopilotInstructions: `applyTo: "**"` frontmatter, then
	// FormatMarkdown. Without applyTo, VS Code does not apply an
	// .instructions.md file on its own.
	FormatCopilotInstructions

	// FormatCodexOverride: the project's AGENTS.md (sync-agents regions
	// stripped), then FormatMarkdown. Codex reads AGENTS.override.md
	// instead of AGENTS.md, so the override carries AGENTS.md's text or
	// Codex loses it.
	FormatCodexOverride
)

// IndexName is the file name under .agents/index/ for toolID's channel:
// "cursor.mdc", "codex.md", "copilot.md", "opencode.md". The extension
// is the one the tool reads at its native path.
func (f Format) IndexName(toolID string) string {
	if f == FormatCursorMDC {
		return toolID + ".mdc"
	}
	return toolID + ".md"
}

// frontmatter is the dialect header a format puts before everything
// else, or "" for plain Markdown.
func (f Format) frontmatter() string {
	switch f {
	case FormatCursorMDC:
		return "---\ndescription: Shared rules from .agents/ (generated by sync-agents)\nalwaysApply: true\n---\n"
	case FormatCopilotInstructions:
		return "---\napplyTo: \"**\"\n---\n"
	default:
		return ""
	}
}

// Mount is how a tool's native read path reaches a channel's content.
// It is a closed sum: LinkMount, RegionMount, ConfigListMount. The
// sealing method also carries the one fact consent depends on.
type Mount interface {
	// editsSharedFile reports whether mounting changes bytes inside a
	// file that others also write (the user, the tool's UI, another
	// program).
	editsSharedFile() bool
}

// LinkMount: sync-agents owns one file name the tool reads, either a
// name it chose inside a directory the tool scans or a file only this
// tool reads. It places a relative symlink there that points at the
// index file. It never needs consent: the name is ours.
//
// A real file at At is a conflict. It is warned about, counted in the
// exit status, and never clobbered (placeLink's never-delete rule).
type LinkMount struct {
	At NativePath
}

// RegionMount: the tool reads one fixed file that other writers also
// edit. The rendered content is spliced between Region's markers. Every
// byte outside them is kept.
type RegionMount struct {
	Host   NativePath
	Region ManagedRegion

	// CreateHost lets sync create a missing host that holds only the
	// region. It is false when another program seeds the host
	// (OpenClaw's workspace AGENTS.md).
	CreateHost bool
}

// ConfigListMount: the tool loads extra instruction files listed in a
// JSON array (opencode.json "instructions"). An absent config is
// created with our one entry. An existing strict-JSON config gets the
// entry by a byte-range insert (ensureJSONArrayEntry) once consent is
// given. A config that is not strict JSON is left for the user to edit.
type ConfigListMount struct {
	File NativePath
	Key  string
}

func (LinkMount) editsSharedFile() bool       { return false }
func (RegionMount) editsSharedFile() bool     { return true }
func (ConfigListMount) editsSharedFile() bool { return true }

// Anchor is what a NativePath is relative to.
type Anchor int

const (
	// AnchorBase: the scope base. That is the project root at local
	// scope and the global root's parent (normally $HOME) at global
	// scope.
	AnchorBase Anchor = iota

	// AnchorHome: the tool's home at the scope. It is resolved on each
	// run because it moves with env and config: $CODEX_HOME, the
	// OpenClaw workspace, ~/.config/opencode, ~/.codeium, ~/.copilot.
	AnchorHome
)

// NativePath locates a vendor-defined read path. Rel is
// slash-separated.
type NativePath struct {
	Under Anchor
	Rel   string
}

// MountFacts is what the shell observed about one channel's native path
// before mounting. mayMount turns it into a verdict.
type MountFacts struct {
	Scope Scope

	// Explicit: --targets named this tool on this invocation.
	Explicit bool

	// HomeExists: the tool's home directory exists (global scope: the
	// tool is installed).
	HomeExists bool

	// NativeExists: the link path, region host, or config file exists.
	NativeExists bool

	// CarriesOurs: our symlink, our region markers, or our config entry
	// is already there. A mount that carries ours is always refreshed.
	CarriesOurs bool
}

// Reasons mayMount gives for declining. The shell prefixes the tool and
// path; the sentences stay fixed so status output is stable.
const (
	whyNotInstalled = "tool not installed (its home directory is missing); run with --targets <tool> to create it"
	whyHostOwned    = "the host file is created by the tool itself; run the tool once, then sync"
	whyNeedsConsent = "the file exists and is yours; run once with --targets <tool> to let sync-agents edit it"
)

// mayMount is the gate and consent rule for creating a mount (SPEC-013
// §Mounts and consent), pure and table-tested. It generalizes
// SPEC-012's marker rule from OpenClaw to every mount that edits a
// shared file:
//
//	CarriesOurs                                    -> yes (refresh)
//	global, home missing, not Explicit             -> no: tool not installed
//	RegionMount, host missing, !CreateHost         -> no: the owning program creates it
//	editsSharedFile, native exists, not Explicit   -> no: run once with --targets
//	otherwise                                      -> yes
//
// Creating a file that did not exist edits nobody's bytes, so it needs
// no consent. Listing a tool in .agents/config `targets` does not count
// as Explicit: it says "deliver to this tool", not "edit my
// opencode.json".
func mayMount(m Mount, f MountFacts) (ok bool, why string) {
	if f.CarriesOurs {
		return true, ""
	}
	if f.Scope == ScopeGlobal && !f.HomeExists && !f.Explicit {
		return false, whyNotInstalled
	}
	if rm, isRegion := m.(RegionMount); isRegion && !rm.CreateHost && !f.NativeExists {
		return false, whyHostOwned
	}
	if m.editsSharedFile() && f.NativeExists && !f.Explicit {
		return false, whyNeedsConsent
	}
	return true, ""
}

// ToolContext is everything per-tool knowledge functions (budgets,
// shadowing) may read.
type ToolContext struct {
	Home   string  // the tool's home at the channel's scope
	Parent string  // the global root's parent ($HOME), for limits that span scopes
	Env    ToolEnv // env vars and config files, injected
}

// Entry is one passive artifact as every channel at a scope receives
// it.
type Entry struct {
	// Name is the heading and the sort key: "security", "macos/brew".
	Name string

	// Source is the absolute content file (SKILL.md for a skill).
	Source string

	// Display is the path a pointer line shows:
	// ".agents/rules/security.md" at local scope,
	// "~/.agents/rules/security.md" at global scope.
	Display string

	// Description is the frontmatter description on one line
	// (artifactDescription); pointer lines carry it.
	Description string

	// OnDemand is `trigger: model_decision` in the artifact's
	// frontmatter (the "agent decides" mode Windsurf and Cursor already
	// understand). In a capped bundle an on-demand entry is always a
	// pointer. Uncapped bundles inline it. Claude and Windsurf still
	// read the source file natively and are unaffected.
	OnDemand bool
}

// discoverChannelArtifacts lists the artifacts a channel set renders
// from tree. allOS=false gates OS-scoped subdirectories by
// effectiveGOOS, as DiscoverArtifacts does. allOS=true (`index =
// commit`) includes every OS scope, so a committed index does not churn
// between a macOS and a Linux contributor; the `<!-- OS: <scope> -->`
// header buildEntriesBody writes before each scoped section tells the
// reading agent which apply. .agents/index/ is not a bucket, so it is
// never walked.
func discoverChannelArtifacts(tree string, allOS bool) ([]Artifact, error) {
	if !allOS {
		return DiscoverArtifacts(tree)
	}
	return discoverArtifactsWithScopes(tree, func(string) bool { return true })
}

// passiveEntries selects what every channel at a scope delivers: each
// rule, workflow, and skill whose resolved semantic is Passive, sorted
// by Name (Display breaks ties). Agents, hooks, and reference documents
// never qualify. An artifact whose frontmatter does not parse is
// returned in errs and skipped, so one broken file does not stop
// delivery of the rest.
//
// displayRoot prefixes pointer paths: ".agents" at local scope,
// "~/.agents" at global scope.
func passiveEntries(arts []Artifact, displayRoot string) (entries []Entry, errs []error) {
	for _, art := range arts {
		switch art.Type {
		case ArtifactRule, ArtifactWorkflow, ArtifactSkill:
		default:
			continue
		}
		sem, err := ResolveSemantic(art.SourcePath, art.Type)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if sem != Passive {
			continue
		}
		b, _ := BucketForArtifact(art.Type)
		display := displayRoot + "/" + b.Dir + "/" + art.Name + ".md"
		if art.Type == ArtifactSkill {
			display = displayRoot + "/" + b.Dir + "/" + art.Name + "/SKILL.md"
		}
		src := concatSourcePath(art)
		entries = append(entries, Entry{
			Name:        art.Name,
			Source:      src,
			Display:     display,
			Description: artifactDescription(src),
			OnDemand:    isOnDemand(src),
		})
	}
	sortEntries(entries)
	return entries, errs
}

// isOnDemand reports whether the artifact at path declares
// `trigger: model_decision`. A file that cannot be read or parsed is not
// on demand; ResolveSemantic has already reported such files.
func isOnDemand(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, err := parseFMBlock(string(raw))
	if err != nil {
		return false
	}
	return block.value("trigger") == "model_decision"
}

// sortEntries orders entries by Name, then Display, so the render is
// deterministic even when a rule and a workflow share a name.
func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Display < entries[j].Display
	})
}

// mergeEntries returns project plus every global entry whose Name the
// project does not define, sorted. Used only for MergeGlobal channels
// under `index = local`: a project rule overrides the global rule of
// the same name, as it does for Claude.
func mergeEntries(project, global []Entry) []Entry {
	defined := make(map[string]bool, len(project))
	out := make([]Entry, 0, len(project)+len(global))
	for _, e := range project {
		defined[e.Name] = true
		out = append(out, e)
	}
	for _, e := range global {
		if !defined[e.Name] {
			out = append(out, e)
		}
	}
	sortEntries(out)
	return out
}

// Banners are fixed per scope, so status can compare bytes.
const (
	localBanner       = "<!-- Generated by sync-agents from .agents/. Edit those files, then run `sync-agents index`. -->"
	globalBanner      = "<!-- Generated by sync-agents from ~/.agents/. Edit those files, then run `sync-agents global sync`. -->"
	codexOverrideNote = "<!-- Codex reads this file instead of AGENTS.md, so AGENTS.md is copied above. -->"
)

// Frame is what a format wraps around the entries.
type Frame struct {
	// Banner is the one-line "generated; edit the sources" comment
	// (localBanner or globalBanner).
	Banner string

	// AgentsMD is the project's AGENTS.md as it is on disk, or nil when
	// there is none. Only FormatCodexOverride uses it. renderChannel
	// strips every sync-agents region from the copy, which keeps an
	// OpenClaw workspace's openclaw-rules region out of Codex: Codex
	// gets the global rules from its own global region.
	AgentsMD []byte
}

// Rendered is a channel's exact output.
type Rendered struct {
	// Bytes is the index file. For a RegionMount the region body is the
	// same bytes; FormatMarkdown has no frontmatter.
	Bytes []byte

	// Inlined names the entries whose bodies are in Bytes. Pointers
	// names the entries demoted to one-line pointers (by budget or
	// OnDemand). Both are in name order.
	Inlined  []string
	Pointers []string

	// Budget and Size back the over-cap warning. Size is Bytes measured
	// in Budget.Cap.Unit, plus Budget.Reserved.
	Budget Budget
	Size   int
}

// OverCap reports whether the tool will truncate what it loads: true
// only when even the all-pointer form does not fit.
func (r Rendered) OverCap() bool {
	return r.Budget.Cap.Limit > 0 && r.Size > r.Budget.Cap.Limit
}

// renderChannel renders entries in format f inside fr, fitted to b. It
// is pure apart from reading entry sources (readArtifactBody), so
// status compares against exactly what sync writes.
//
// Layout: the format's frontmatter; for FormatCodexOverride the
// region-stripped AGENTS.md and codexOverrideNote; the banner; the
// inlined sections; and, when anything was demoted, the "## Not
// inlined" pointer list. Sizes add up exactly (the bytes are the
// concatenation of the parts fitBudget measured), so a bundle sized to
// the limit is exactly the limit.
func renderChannel(f Format, entries []Entry, fr Frame, b Budget) (Rendered, error) {
	head := f.frontmatter()
	if f == FormatCodexOverride {
		if agents := strings.TrimRight(stripAllRegions(string(fr.AgentsMD)), "\n"); strings.TrimSpace(agents) != "" {
			head += agents + "\n\n" + codexOverrideNote + "\n"
		}
	}
	head += fr.Banner + "\n\n"

	sections := make([]section, 0, len(entries))
	for _, e := range entries {
		block, err := buildEntriesBody([]ConcatEntry{{Name: e.Name, SourcePath: e.Source}})
		if err != nil {
			return Rendered{}, err
		}
		pointer := "- " + e.Name + " (" + e.Display + ")"
		if e.Description != "" {
			pointer += ": " + e.Description
		}
		sections = append(sections, section{
			Name:     e.Name,
			OnDemand: e.OnDemand,
			Block:    block,
			Pointer:  []byte(pointer + "\n"),
		})
	}

	inlined, pointers := fitBudget(sections, b.Cap.Measure([]byte(head))+b.Reserved, b.Cap)

	var buf bytes.Buffer
	buf.WriteString(head)
	r := Rendered{Budget: b}
	for _, s := range inlined {
		buf.Write(s.Block)
		r.Inlined = append(r.Inlined, s.Name)
	}
	if len(pointers) > 0 {
		buf.WriteString(pointerHeading)
		for _, s := range pointers {
			buf.Write(s.Pointer)
			r.Pointers = append(r.Pointers, s.Name)
		}
	}
	r.Bytes = buf.Bytes()
	r.Size = b.Cap.Measure(r.Bytes) + b.Reserved
	return r, nil
}

// stripAllRegions removes every well-formed sync-agents region from
// text, whatever its writer. A start marker with no end marker is left
// as ordinary text, as spliceRegion and splitBlocks treat it.
func stripAllRegions(text string) string {
	seen := map[string]bool{}
	for _, m := range regionStartPattern.FindAllStringSubmatch(text, -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		r := ManagedRegion{Name: m[1]}
		for {
			next, ok := stripRegionCollapsing(text, r)
			if !ok {
				break
			}
			text = next
		}
	}
	return text
}
