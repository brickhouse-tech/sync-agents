package source

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// scan.go implements SPEC-005 Part B's static scanner: heuristics run
// over a staged artifact BEFORE it can enter the live .agents/ tree.
// The scanner is deliberately dumb — regexes, not semantics — because
// its job is to force a human look at the suspicious 1%, not to
// adjudicate. False positives cost one `approve --force`; false
// negatives cost a compromised agent context, so thresholds lean
// toward flagging.

// FindingSeverity buckets scanner findings. Critical findings block
// `approve` without --force; warn/info are surfaced but never block.
type FindingSeverity string

const (
	SeverityInfo     FindingSeverity = "info"
	SeverityWarn     FindingSeverity = "warn"
	SeverityCritical FindingSeverity = "critical"
)

// Finding is one scanner hit inside a staged artifact.
type Finding struct {
	// Path is relative to the artifact root ("" for single files).
	Path     string          `json:"path"`
	Class    string          `json:"class"`
	Severity FindingSeverity `json:"severity"`
	Detail   string          `json:"detail"`
}

// HasCritical reports whether any finding blocks approval.
func HasCritical(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// scanRule is one heuristic: a compiled pattern plus its
// classification. Patterns run per file over raw content.
type scanRule struct {
	class    string
	severity FindingSeverity
	re       *regexp.Regexp
	detail   string
}

var scanRules = []scanRule{
	// Network-then-execute: the classic supply-chain payload shape.
	{"net-exec", SeverityCritical,
		regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|;]*\|\s*(ba)?sh\b`),
		"downloads and pipes straight into a shell"},
	{"net-exec", SeverityCritical,
		regexp.MustCompile(`(?i)\biwr\b[^\n|;]*\|\s*iex\b`),
		"PowerShell download piped into Invoke-Expression"},
	{"net-exec", SeverityCritical,
		regexp.MustCompile(`(?i)base64\s+(-d|--decode)[^\n]*\|\s*(ba)?sh\b`),
		"base64-decode piped into a shell"},

	// Credential exfiltration surface: reads of secret material.
	// Critical only when the same file also talks to the network
	// (checked in scanFile), otherwise a warn.
	{"secrets-access", SeverityWarn,
		regexp.MustCompile(`(?:~|\$HOME)/\.(ssh|aws|gnupg)\b|\.claude/\.credentials|\bcredentials\.json\b`),
		"touches credential material (~/.ssh, ~/.aws, credentials files)"},
	{"secrets-access", SeverityWarn,
		regexp.MustCompile(`\b[A-Z][A-Z0-9_]*_(TOKEN|API_KEY|SECRET)\b`),
		"references secret-bearing environment variables"},

	// Obfuscation: payloads hiding from review.
	{"obfuscation", SeverityWarn,
		regexp.MustCompile(`[A-Za-z0-9+/]{120,}={0,2}`),
		"long base64-like blob (≥120 chars)"},
	{"obfuscation", SeverityCritical,
		regexp.MustCompile("[​‌‍⁠‪-‮]"),
		"zero-width or bidi-control Unicode (content hidden from human review)"},

	// Prompt injection: instructions aimed at the agent reading the
	// artifact rather than at the human installing it.
	{"prompt-injection", SeverityCritical,
		regexp.MustCompile(`(?i)\b(ignore|disregard|forget)\b.{0,40}\b(previous|prior|above|all)\b.{0,40}\b(instructions?|rules?|prompts?)\b`),
		"tells the agent to discard its instructions"},
	{"prompt-injection", SeverityWarn,
		regexp.MustCompile(`(?i)<!--[^>]{0,200}\b(do not (tell|show|reveal)|secretly|without (telling|informing))\b`),
		"hidden HTML comment directing covert behavior"},

	// Exec surface: not dangerous alone, but worth a human glance on
	// remote content.
	{"exec-surface", SeverityInfo,
		regexp.MustCompile(`(?m)^#!\s*/`),
		"executable script (shebang)"},
	{"exec-surface", SeverityInfo,
		regexp.MustCompile(`\b(eval|exec)\s*\(`),
		"dynamic code execution (eval/exec)"},
}

// networkRe upgrades secrets-access findings to critical when the
// same file can move data off the machine.
var networkRe = regexp.MustCompile(`(?i)\b(curl|wget|https?://|net/http|fetch\(|requests\.(get|post)|nc\s+-)`)

// scannableExt limits scanning to text-artifact types; binaries in a
// skill tree are flagged wholesale instead of pattern-scanned.
func scannable(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".txt", ".sh", ".bash", ".zsh", ".py", ".js", ".ts", ".rb", ".json", ".yaml", ".yml", ".toml", "":
		return true
	}
	return false
}

// ScanTree walks a staged artifact (file or directory) and returns
// every heuristic hit. It never fails the pull by itself — the
// quarantine layer decides what findings mean.
func ScanTree(root string) []Finding {
	var findings []Finding
	fi, err := os.Stat(root)
	if err != nil {
		return nil
	}

	scanOne := func(path, rel string) {
		if IsOriginFile(filepath.Base(path)) {
			return
		}
		if !scannable(path) {
			findings = append(findings, Finding{
				Path: rel, Class: "binary", Severity: SeverityWarn,
				Detail: "non-text file in a remote artifact — review by hand",
			})
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		content := string(data)
		hasNetwork := networkRe.MatchString(content)
		for _, rule := range scanRules {
			if !rule.re.MatchString(content) {
				continue
			}
			sev := rule.severity
			if rule.class == "secrets-access" && hasNetwork {
				sev = SeverityCritical
			}
			findings = append(findings, Finding{
				Path: rel, Class: rule.class, Severity: sev, Detail: rule.detail,
			})
		}
	}

	if !fi.IsDir() {
		scanOne(root, filepath.Base(root))
		return findings
	}
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = d.Name()
		}
		scanOne(path, filepath.ToSlash(rel))
		return nil
	})
	return findings
}
