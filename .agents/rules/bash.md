---
trigger: always_on
---

# bash / zsh

This is the bash/zsh implementation of the `sync-agents` CLI tool. It provides the same functionality as the Node.js version, but is implemented as a shell script for environments where Node.js may not be available or desired.

## standards

- Use `#!/usr/bin/env bash` shebang for maximum compatibility, but ensure it works in `zsh` as well.
- Use $() for command substitution instead of backticks for better readability.
- Use `set -e` to exit on any command failure, and `set -u` to treat unset variables as errors.
- Use functions to organize code and improve readability.
- Use `getopts` for parsing command-line options and arguments.
- Provide clear usage instructions and error messages for invalid commands or options.
- Use consistent naming conventions for variables and functions (e.g., lowercase with underscores).
- Include comments to explain the purpose of functions and complex code sections.
- utilize while loops to parse arguments and options, allowing for flexible command structures.
- utilize conditional script loading where if the script is being sourced, it should not execute the main function, but if it is being run directly, it should execute the main function. This allows for better modularity and reusability of the script's functions in other contexts.
- allow options to be passed in any order, and for commands to be specified with or without options (e.g., `sync-agents sync --force` or `sync-agents --force sync` should both work).
