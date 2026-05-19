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
			printUsage()
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printUsage()
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
		Short: "Add a new rule, skill, or workflow",
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
	rootCmd.AddCommand(&cobra.Command{
		Use:   "index",
		Short: "Regenerate AGENTS.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CmdIndex()
		},
	})

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
	rootCmd.AddCommand(&cobra.Command{
		Use:   "import [url]",
		Short: "Import a rule/skill/workflow from URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			var url string
			if len(args) > 0 {
				url = args[0]
			}
			return app.CmdImport(url)
		},
	})

	// hook
	rootCmd.AddCommand(&cobra.Command{
		Use:   "hook",
		Short: "Install pre-commit git hook",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			printUsage()
		} else if strings.Contains(errStr, "unknown flag") || strings.Contains(errStr, "unknown shorthand") {
			flag := extractUnknownFlag(errStr)
			fmt.Fprintf(os.Stderr, "[error] Unknown option: %s\n", flag)
			printUsage()
		}
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`sync-agents v%s - One set of agent rules to rule them all.

USAGE
  sync-agents <command> [options]

COMMANDS
  init                          Initialize .agents/ directory structure and AGENTS.md
  sync                          Sync .agents/ to agent directories via symlinks
  status                        Show current sync status
  add <type> <name>             Add a new rule, skill, or workflow from template
  index                         Regenerate AGENTS.md index from .agents/ contents
  clean                         Remove all synced symlinks (does not remove .agents/)
  watch                         Watch .agents/ for changes and auto-regenerate index
  import <url>                  Import a rule/skill/workflow from a URL
  hook                          Install a pre-commit git hook for auto-sync
  fix [type]                    Migrate legacy dirs + repair broken symlinks
  inherit <label> <path>        Add an inheritance link to AGENTS.md
  inherit --list                List current inheritance links
  inherit --remove <label>      Remove an inheritance link by label
  version                       Show version (same as --version)

OPTIONS
  -h, --help                    Show this help message
  -v, --version                 Show version
  -d, --dir <path>              Set project root directory (default: current directory)
  --targets <list>              Comma-separated targets (overrides .agents/config)
  --dry-run                     Show what would be done without making changes
  --force                       Overwrite existing files/symlinks

EXAMPLES
  sync-agents init              # Initialize .agents/ structure
  sync-agents add rule no-eval  # Add a new rule called "no-eval"
  sync-agents sync              # Sync to .claude/ and .windsurf/
  sync-agents sync --targets claude
  sync-agents status            # Show current state
  sync-agents clean             # Remove synced symlinks

`, version.Version)
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
