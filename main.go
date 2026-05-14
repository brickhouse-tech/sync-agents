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

var (
	app           = agent.NewApp()
	customDir     string
	customTargets string
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
