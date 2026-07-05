package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brickhouse-tech/sync-agents/internal/agent"
	"github.com/brickhouse-tech/sync-agents/internal/version"
	"github.com/spf13/cobra"
)

// app is the singleton sync-agents state container used by every
// command. PersistentPreRunE populates it from CLI flags before any
// command's RunE fires.
//
// The package-level globals here are intentionally minimal — only
// flag-target variables and the App instance. Anything more elaborate
// (sub-app structs, dependency wiring) lives in internal/agent so
// tests can construct an App without going through cobra.
var (
	app = agent.NewApp()

	// customDir backs the persistent --dir/-d flag. Empty means "auto-
	// detect via agent.FindProjectRoot".
	customDir string

	// customTargets backs the persistent --targets flag. Empty means
	// "read from .agents/config" (agent.ReadConfigTargets).
	customTargets string

	// customGlobalRoot backs the persistent --global-root flag. Empty
	// means "consult $SYNC_AGENTS_GLOBAL_ROOT then fall back to
	// $HOME/.agents". See agent.ResolveGlobalRoot for the precedence
	// chain and SPEC-002 §Configurable global root for the
	// requirement.
	customGlobalRoot string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "sync-agents",
		Short: fmt.Sprintf("sync-agents v%s - One set of agent rules to rule them all.", version.Version),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if customDir != "" {
				abs, err := filepath.Abs(customDir)
				if err != nil {
					return err
				}
				app.ProjectRoot = abs
			} else {
				app.ProjectRoot = agent.FindProjectRoot("")
			}

			// --global-root takes precedence over the env var and the
			// $HOME default, per SPEC-002 §Configurable global root.
			// We assign the raw flag value to app.GlobalRoot; absolute-
			// path normalization happens inside ResolveGlobalRoot so a
			// non-CLI caller (test, library) sees the same behavior.
			if customGlobalRoot != "" {
				app.GlobalRoot = customGlobalRoot
			}

			if customTargets != "" {
				var targets []string
				for _, t := range strings.Split(customTargets, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						targets = append(targets, t)
					}
				}
				app.ActiveTargets = targets
			} else {
				app.ActiveTargets = agent.ReadConfigTargets(app.ProjectRoot)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			printUsage(cmd)
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Cobra's SetHelpFunc is inherited by every child command, so a naive
	// override would make `sync-agents global --help` (and every other
	// subcommand help) print the root usage. Capture cobra's default help
	// function first, then short-circuit only when the help target is the
	// root command — subcommands fall through to the default renderer,
	// which prints each subcommand's own usage from its registered flags
	// and children.
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			printUsage(rootCmd)
			return
		}
		defaultHelp(cmd, args)
	})

	rootCmd.PersistentFlags().StringVarP(&customDir, "dir", "d", "", "Set project root directory")
	rootCmd.PersistentFlags().StringVar(&customTargets, "targets", "", "Comma-separated targets")
	rootCmd.PersistentFlags().BoolVar(&app.DryRun, "dry-run", false, "Show what would be done")
	rootCmd.PersistentFlags().BoolVar(&app.Force, "force", false, "Overwrite existing files")
	// --global-root overrides the user's global .agents/ tree
	// location. Wins over $SYNC_AGENTS_GLOBAL_ROOT and the default
	// $HOME/.agents per SPEC-002 §Configurable global root.
	// Currently a no-op until SPEC-002's promote and global-sync
	// commands land in subsequent PRs.
	rootCmd.PersistentFlags().StringVar(&customGlobalRoot, "global-root", "", "Override the global .agents/ root (default: $SYNC_AGENTS_GLOBAL_ROOT or $HOME/.agents)")

	// version flag
	rootCmd.Flags().BoolP("version", "v", false, "Show version")
	rootCmd.SetVersionTemplate(fmt.Sprintf("sync-agents v%s\n", version.Version))
	rootCmd.Version = version.Version

	// version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(os.Stdout, "sync-agents v%s\n", version.Version)
		},
	})

	// init
	rootCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize .agents/ directory structure",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdInit()
		},
	})

	// add
	rootCmd.AddCommand(&cobra.Command{
		Use:   "add [type] [name]",
		Short: "Add a new rule, skill, workflow, agent, plan, spec, hook, or adr",
		RunE: func(cmd *cobra.Command, args []string) error {
			var typ, name string
			if len(args) >= 1 {
				typ = args[0]
			}
			if len(args) >= 2 {
				name = args[1]
			}
			return app.CmdAdd(typ, name)
		},
	})

	// sync
	rootCmd.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Sync .agents/ to agent directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdSync()
		},
	})

	// status
	rootCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show current sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdStatus()
		},
	})

	// index
	var indexNoFix bool
	indexCmd := &cobra.Command{
		Use:   "index",
		Short: "Regenerate AGENTS.md (backfills skill frontmatter first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !indexNoFix {
				app.CmdBackfillSkills()
			}
			return app.CmdIndex()
		},
	}
	indexCmd.Flags().BoolVar(&indexNoFix, "no-fix", false, "Skip the skill frontmatter backfill")
	rootCmd.AddCommand(indexCmd)

	// clean
	rootCmd.AddCommand(&cobra.Command{
		Use:   "clean",
		Short: "Remove all synced symlinks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdClean()
		},
	})

	// watch
	rootCmd.AddCommand(&cobra.Command{
		Use:   "watch",
		Short: "Watch .agents/ for changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdWatch()
		},
	})

	// import
	var importTrust bool
	importCmd := &cobra.Command{
		Use:   "import [url]",
		Short: "Import a rule/skill/workflow from URL (quarantined + scanned by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var url string
			if len(args) > 0 {
				url = args[0]
			}
			return app.CmdImport(url, importTrust)
		},
	}
	importCmd.Flags().BoolVar(&importTrust, "trust", false, "Bypass the quarantine gate (the scan still runs and prints findings)")
	rootCmd.AddCommand(importCmd)

	// pull — install every entry declared in .agents/sources.yaml
	// (SPEC-003). --force/--dry-run come from the persistent root
	// flags like every other command; --offline/--only/--global are
	// pull-specific.
	var pullOffline bool
	var pullOnly string
	var pullGlobal bool
	var pullTrust bool
	pullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch every source declared in .agents/sources.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdPull(agent.SourceCmdOpts{
				Offline: pullOffline,
				Only:    pullOnly,
				Global:  pullGlobal,
				Trust:   pullTrust,
			})
		},
	}
	pullCmd.Flags().BoolVar(&pullOffline, "offline", false, "Serve from the local cache only; fail on cache misses")
	pullCmd.Flags().BoolVar(&pullTrust, "trust", false, "Bypass the quarantine gate (the scan still runs and prints findings)")
	pullCmd.Flags().StringVar(&pullOnly, "only", "", "Pull only the named entry")
	pullCmd.Flags().BoolVar(&pullGlobal, "global", false, "Operate on the global ~/.agents/ tree")
	rootCmd.AddCommand(pullCmd)

	// update — re-resolve refs and re-pull entries whose SHA moved.
	// SHA-pinned entries are skipped (nothing to advance).
	var updateGlobal bool
	var updateTrust bool
	updateCmd := &cobra.Command{
		Use:   "update [NAME]",
		Short: "Re-resolve source refs and pull entries whose SHA advanced",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return app.CmdUpdate(name, agent.SourceCmdOpts{Global: updateGlobal, Trust: updateTrust})
		},
	}
	updateCmd.Flags().BoolVar(&updateGlobal, "global", false, "Operate on the global ~/.agents/ tree")
	updateCmd.Flags().BoolVar(&updateTrust, "trust", false, "Bypass the quarantine gate (the scan still runs and prints findings)")
	rootCmd.AddCommand(updateCmd)

	// quarantine / approve / reject — SPEC-005 Part B review flow.
	var quarGlobal bool
	quarCmd := &cobra.Command{
		Use:   "quarantine",
		Short: "List remotely-fetched artifacts awaiting review",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdQuarantine(agent.SourceCmdOpts{Global: quarGlobal})
		},
	}
	quarCmd.Flags().BoolVar(&quarGlobal, "global", false, "Operate on the global ~/.agents/ tree")
	rootCmd.AddCommand(quarCmd)

	var approveAll, approveGlobal bool
	approveCmd := &cobra.Command{
		Use:   "approve [name]",
		Short: "Promote a quarantined artifact into .agents/ (--force accepts critical findings)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return app.CmdApprove(name, approveAll, agent.SourceCmdOpts{Global: approveGlobal})
		},
	}
	approveCmd.Flags().BoolVar(&approveAll, "all", false, "Approve everything in quarantine")
	approveCmd.Flags().BoolVar(&approveGlobal, "global", false, "Operate on the global ~/.agents/ tree")
	rootCmd.AddCommand(approveCmd)

	var rejectAll, rejectGlobal bool
	rejectCmd := &cobra.Command{
		Use:   "reject [name]",
		Short: "Delete a quarantined artifact without installing it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return app.CmdReject(name, rejectAll, agent.SourceCmdOpts{Global: rejectGlobal})
		},
	}
	rejectCmd.Flags().BoolVar(&rejectAll, "all", false, "Reject everything in quarantine")
	rejectCmd.Flags().BoolVar(&rejectGlobal, "global", false, "Operate on the global ~/.agents/ tree")
	rootCmd.AddCommand(rejectCmd)

	// source — manifest maintenance command group (SPEC-003):
	// add/remove/list/bundle/detach.
	var srcGlobal bool
	sourceCmd := &cobra.Command{
		Use:   "source",
		Short: "Manage upstream sources declared in .agents/sources.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	// --global is persistent on the group so every subcommand honors
	// it uniformly (AC-12).
	sourceCmd.PersistentFlags().BoolVar(&srcGlobal, "global", false, "Operate on the global ~/.agents/ tree")

	var addOffline bool
	sourceAddCmd := &cobra.Command{
		Use:   "add <entry>",
		Short: "Declare a source entry and pull it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry := ""
			if len(args) > 0 {
				entry = args[0]
			}
			return app.CmdSourceAdd(entry, agent.SourceCmdOpts{Global: srcGlobal, Offline: addOffline})
		},
	}
	sourceAddCmd.Flags().BoolVar(&addOffline, "offline", false, "Serve from the local cache only; fail on cache misses")
	sourceCmd.AddCommand(sourceAddCmd)

	var removeKeep bool
	sourceRemoveCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a source entry and delete its artifact(s)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return app.CmdSourceRemove(name, agent.SourceCmdOpts{Global: srcGlobal, Keep: removeKeep})
		},
	}
	sourceRemoveCmd.Flags().BoolVar(&removeKeep, "keep", false, "Keep the artifact on disk as a manual (untracked) artifact")
	sourceCmd.AddCommand(sourceRemoveCmd)

	var listJSON bool
	sourceListCmd := &cobra.Command{
		Use:   "list",
		Short: "List declared sources with their resolved SHAs and local status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdSourceList(agent.SourceCmdOpts{Global: srcGlobal, JSON: listJSON})
		},
	}
	sourceListCmd.Flags().BoolVar(&listJSON, "json", false, "Emit machine-readable JSON")
	sourceCmd.AddCommand(sourceListCmd)

	sourceCmd.AddCommand(&cobra.Command{
		Use:   "bundle",
		Short: "Reconstruct sources.yaml from installed artifacts' origin metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdSourceBundle(agent.SourceCmdOpts{Global: srcGlobal})
		},
	})

	sourceCmd.AddCommand(&cobra.Command{
		Use:   "detach <name>",
		Short: "Convert a pulled artifact to a manual one and drop its manifest entry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return app.CmdSourceDetach(name, agent.SourceCmdOpts{Global: srcGlobal})
		},
	})
	rootCmd.AddCommand(sourceCmd)

	// adr — transition Architecture Decision Records between statuses
	rootCmd.AddCommand(&cobra.Command{
		Use:   "adr <accept|deny|propose> <name>",
		Short: "Move an ADR between proposed/accepted/denied and reindex",
		RunE: func(cmd *cobra.Command, args []string) error {
			var action, name string
			if len(args) >= 1 {
				action = args[0]
			}
			if len(args) >= 2 {
				name = args[1]
			}
			return app.CmdADR(action, name)
		},
	})

	// lint
	var lintFix bool
	lintCmd := &cobra.Command{
		Use:   "lint [type]",
		Short: "Validate skill frontmatter against Claude authoring rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			lintType := ""
			if len(args) > 0 {
				lintType = args[0]
			}
			return app.CmdLint(lintType, lintFix)
		},
	}
	lintCmd.Flags().BoolVar(&lintFix, "fix", false, "Amend fixable findings in place")
	rootCmd.AddCommand(lintCmd)

	// git-hook — install git pre-commit hook. Renamed from `hook`
	// (SPEC-004 Part C) to free the name for the hooks bucket; the
	// old name remains a deprecated alias.
	rootCmd.AddCommand(&cobra.Command{
		Use:     "git-hook",
		Aliases: []string{"hook"},
		Short:   "Install git pre-commit hook that syncs + indexes on commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.CalledAs() == "hook" {
				fmt.Fprintln(os.Stderr, "[warn] `sync-agents hook` is deprecated; use `sync-agents git-hook` (same behavior).")
			}
			return app.CmdHook()
		},
	})

	// fix
	var noClobber bool
	fixCmd := &cobra.Command{
		Use:   "fix [type]",
		Short: "Migrate legacy dirs + repair broken symlinks",
		RunE: func(cmd *cobra.Command, args []string) error {
			fixType := "all"
			if len(args) > 0 {
				fixType = args[0]
			}
			return app.CmdFix(fixType, noClobber)
		},
	}
	fixCmd.Flags().BoolVar(&noClobber, "no-clobber", false, "Skip items that already exist")
	rootCmd.AddCommand(fixCmd)

	// inherit
	var inheritList bool
	var inheritRemove string
	inheritCmd := &cobra.Command{
		Use:   "inherit [label] [path]",
		Short: "Manage inheritance links",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inheritList {
				return app.CmdInheritList()
			}
			if inheritRemove != "" {
				return app.CmdInheritRemove(inheritRemove)
			}
			if len(args) < 2 {
				app.Error("Usage: sync-agents inherit <label> <path>")
				app.Error("       sync-agents inherit --list")
				app.Error("       sync-agents inherit --remove <label>")
				return fmt.Errorf("missing args")
			}
			return app.CmdInheritAdd(args[0], args[1])
		},
	}
	inheritCmd.Flags().BoolVar(&inheritList, "list", false, "List inheritance links")
	inheritCmd.Flags().StringVar(&inheritRemove, "remove", "", "Remove inheritance link by label")
	rootCmd.AddCommand(inheritCmd)

	// promote — copy a local .agents/ artifact into ~/.agents/.
	//
	// Two invocation forms (SPEC-002 §Requirement: Promote command):
	//   sync-agents promote <type> <name>     // canonical
	//   sync-agents promote <path>            // path-form sugar
	//
	// The canonical form takes type ∈ {rule, skill, workflow} and a
	// bare name; the path form auto-detects both from a path under
	// .agents/. --force and --dry-run are honored via the persistent
	// flags wired at the rootCmd level. --sync (added in PR C.2)
	// runs `global sync` automatically after a successful promote.
	var promoteSync bool
	var promoteTargets string
	promoteCmd := &cobra.Command{
		Use:   "promote <type> <name> | <path>",
		Short: "Promote a local .agents/ artifact to the global ~/.agents/ tree",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var typ agent.ArtifactType
			var name string
			if len(args) == 2 {
				// Canonical form: validate the type string against
				// the documented set; we accept singular and plural
				// here to match the existing `add` command.
				t, ok := agent.NormalizeArtifactType(args[0])
				if !ok {
					app.Error(fmt.Sprintf("unknown type %q; must be one of: rule, skill, workflow", args[0]))
					return fmt.Errorf("unknown type")
				}
				typ = t
				name = args[1]
			} else {
				// Path form: detect type+name from the path.
				t, n, err := agent.DetectArtifact(args[0])
				if err != nil {
					app.Error(err.Error())
					return err
				}
				typ = t
				name = n
			}
			if err := app.CmdPromote(typ, name, agent.PromoteOpts{
				Force:  app.Force,
				DryRun: app.DryRun,
			}); err != nil {
				return err
			}
			if promoteSync {
				// Compose: run global sync immediately after promote.
				// The --targets flag on promote is honored only when
				// --sync is set; without --sync there's nothing for
				// the targets list to constrain.
				targets := parseTargetList(promoteTargets)
				return app.CmdGlobalSync(agent.GlobalSyncOpts{Targets: targets})
			}
			return nil
		},
	}
	promoteCmd.Flags().BoolVar(&promoteSync, "sync", false, "Run `global sync` after a successful promote")
	promoteCmd.Flags().StringVar(&promoteTargets, "sync-targets", "", "Comma-separated tools to sync (only honored with --sync)")
	rootCmd.AddCommand(promoteCmd)

	// global — command group for user-scope (~/.agents) operations.
	// Subcommands today: init. SPEC-002's promote/global sync/status/
	// clean land in subsequent PRs.
	globalCmd := &cobra.Command{
		Use:   "global",
		Short: "Manage the user-scope ~/.agents/ tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default RunE on the parent prints help; cobra handles
			// this automatically when there are subcommands, but
			// being explicit matches the surrounding command style.
			return cmd.Help()
		},
	}
	globalCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize ~/.agents/ with the standard skeleton",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdGlobalInit()
		},
	})

	// global sync — fan ~/.agents/ out to per-tool global directories.
	//
	// Routing happens via semantic (frontmatter → bucket default)
	// rather than bucket name; see SPEC-002 §Semantic-aware routing
	// and docs/architecture/semantic-routing.md. Per-tool destinations
	// are computed by TargetDestination; concat targets (Windsurf
	// memories, Copilot/Codex instructions.md) are regenerated atomically
	// at the end of the sync.
	var globalSyncTargets string
	globalSyncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Fan ~/.agents/ out to per-tool global directories via semantic-aware routing",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdGlobalSync(agent.GlobalSyncOpts{
				Targets: parseTargetList(globalSyncTargets),
			})
		},
	}
	globalSyncCmd.Flags().StringVar(&globalSyncTargets, "targets", "", "Comma-separated tools to sync (default: all registered)")
	globalCmd.AddCommand(globalSyncCmd)

	// global status — read-only report of every per-tool destination's
	// state. Useful for sanity-checking what a sync would do or
	// confirming that a clean wiped everything.
	var globalStatusTargets string
	globalStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Report the state of per-tool global directories vs ~/.agents/",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdGlobalStatus(agent.GlobalStatusOpts{
				Targets: parseTargetList(globalStatusTargets),
			})
		},
	}
	globalStatusCmd.Flags().StringVar(&globalStatusTargets, "targets", "", "Comma-separated tools to report on (default: all registered)")
	globalCmd.AddCommand(globalStatusCmd)

	// global clean — remove sync-agents-owned symlinks and concat
	// files from per-tool global dirs. Safety-gated: user-owned
	// files (without the banner) and user-owned symlinks (pointing
	// outside ~/.agents/) are left alone with warnings.
	var globalCleanTargets string
	globalCleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove sync-agents-owned global symlinks and concat files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdGlobalClean(agent.GlobalCleanOpts{
				Targets: parseTargetList(globalCleanTargets),
			})
		},
	}
	globalCleanCmd.Flags().StringVar(&globalCleanTargets, "targets", "", "Comma-separated tools to clean (default: all registered)")
	globalCmd.AddCommand(globalCleanCmd)

	rootCmd.AddCommand(globalCmd)

	if err := rootCmd.Execute(); err != nil {
		// Check for unknown command/option patterns
		errStr := err.Error()
		if strings.Contains(errStr, "unknown command") {
			fmt.Fprintf(os.Stderr, "[error] Unknown command: %s\n", extractUnknownCmd(errStr))
			printUsage(rootCmd)
		} else if strings.Contains(errStr, "unknown flag") || strings.Contains(errStr, "unknown shorthand") {
			flag := extractUnknownFlag(errStr)
			fmt.Fprintf(os.Stderr, "[error] Unknown option: %s\n", flag)
			printUsage(rootCmd)
		}
		os.Exit(1)
	}
}

// printUsage renders the top-level `sync-agents` help block. The COMMANDS
// and OPTIONS sections are loop-rendered from cmd's own Commands() and
// flag sets so they cannot drift when a new subcommand or persistent
// flag is added (see issue #38). The uppercase USAGE/COMMANDS/OPTIONS
// headers are preserved on purpose — the bats integration suite asserts
// on those literal strings, and they match the project's prior visual
// style. Subcommand help still goes through cobra's default renderer.
func printUsage(cmd *cobra.Command) {
	fmt.Printf("sync-agents v%s - One set of agent rules to rule them all.\n\n", version.Version)
	fmt.Println("USAGE")
	fmt.Printf("  %s <command> [options]\n\n", cmd.Name())

	fmt.Println("COMMANDS")
	// Compute column width across visible subcommand Use strings so the
	// Short descriptions line up regardless of how many commands exist.
	const minPad = 28
	pad := minPad
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		if w := len(sub.Use); w > pad {
			pad = w
		}
	}
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		fmt.Printf("  %-*s  %s\n", pad, sub.Use, sub.Short)
	}
	fmt.Println()

	fmt.Println("OPTIONS")
	// LocalFlags includes both rootCmd.Flags() and PersistentFlags() for
	// the root command, which is exactly the surface a user sees at the
	// top level. FlagUsages already pads aligned columns.
	fmt.Print(cmd.LocalFlags().FlagUsages())
}

func extractUnknownCmd(errStr string) string {
	// cobra: "unknown command "foo" for "sync-agents""
	if i := strings.Index(errStr, `"`); i >= 0 {
		rest := errStr[i+1:]
		if j := strings.Index(rest, `"`); j >= 0 {
			return rest[:j]
		}
	}
	return errStr
}

func extractUnknownFlag(errStr string) string {
	// cobra: "unknown flag: --bogus"
	if i := strings.Index(errStr, ": "); i >= 0 {
		return strings.TrimSpace(errStr[i+2:])
	}
	return errStr
}

// parseTargetList splits a comma-separated tool-name list into a
// trimmed string slice. Empty input returns nil so callers can pass
// the result straight to GlobalSyncOpts.Targets, where nil means
// "all tools."
func parseTargetList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
