#!/usr/bin/env bash
set -euo pipefail

AGENTS_DIR=".agents"
AGENTS_MD="AGENTS.md"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_JSON="${SCRIPT_DIR}/../../package.json"
TEMPLATES_DIR="${SCRIPT_DIR}/../md"

# Pull version from package.json
# Try relative path first (works in repo / npm local install),
# then resolve via node for global installs where the symlink target differs.
if [[ -f "$PACKAGE_JSON" ]]; then
  VERSION="$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$PACKAGE_JSON" | head -1)"
elif command -v node >/dev/null 2>&1; then
  VERSION="$(node -p "require('@brickhouse-tech/sync-agents/package.json').version" 2>/dev/null || echo "unknown")"
else
  VERSION="unknown"
fi

# Agent target directories
TARGETS=("claude" "windsurf")

# Colors (disabled if not a terminal)
if [[ -t 1 ]]; then
  BOLD='\033[1m'
  GREEN='\033[0;32m'
  YELLOW='\033[0;33m'
  RED='\033[0;31m'
  CYAN='\033[0;36m'
  RESET='\033[0m'
else
  BOLD='' GREEN='' YELLOW='' RED='' CYAN='' RESET=''
fi

info()  { echo -e "${GREEN}[info]${RESET} $*"; }
warn()  { echo -e "${YELLOW}[warn]${RESET} $*"; }
error() { echo -e "${RED}[error]${RESET} $*" >&2; }

usage() {
  cat <<EOF
${BOLD}sync-agents${RESET} v${VERSION} - One set of agent rules to rule them all.

${BOLD}USAGE${RESET}
  sync-agents <command> [options]

${BOLD}COMMANDS${RESET}
  init                          Initialize .agents/ directory structure and AGENTS.md
  sync                          Sync .agents/ to .claude/ and .windsurf/ via symlinks
  status                        Show current sync status
  add <type> <name>             Add a new rule, skill, or workflow from template
  index                         Regenerate AGENTS.md index from .agents/ contents
  clean                         Remove all synced symlinks (does not remove .agents/)

${BOLD}OPTIONS${RESET}
  -h, --help                    Show this help message
  -v, --version                 Show version
  -d, --dir <path>              Set project root directory (default: current directory)
  --targets <list>              Comma-separated targets to sync (default: claude,windsurf)
  --dry-run                     Show what would be done without making changes
  --force                       Overwrite existing files/symlinks

${BOLD}EXAMPLES${RESET}
  sync-agents init              # Initialize .agents/ structure
  sync-agents add rule no-eval  # Add a new rule called "no-eval"
  sync-agents sync              # Sync to .claude/ and .windsurf/
  sync-agents sync --targets claude
  sync-agents status            # Show current state
  sync-agents clean             # Remove synced symlinks

EOF
}

# --------------------------------------------------------------------------
# Helpers
# --------------------------------------------------------------------------

find_project_root() {
  local dir="${1:-.}"
  dir="$(cd "$dir" && pwd)"

  # Walk up to find a directory with .agents or .git
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/$AGENTS_DIR" ]] || [[ -d "$dir/.git" ]]; then
      echo "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done

  # Fallback to current directory
  pwd
}

ensure_agents_dir() {
  if [[ ! -d "$PROJECT_ROOT/$AGENTS_DIR" ]]; then
    error ".agents/ directory not found. Run 'sync-agents init' first."
    exit 1
  fi
}

create_symlink() {
  local source="$1"
  local target="$2"
  local dry_run="${3:-false}"

  if [[ "$dry_run" == "true" ]]; then
    echo "  would link: $target -> $source"
    return 0
  fi

  local target_dir
  target_dir="$(dirname "$target")"
  mkdir -p "$target_dir"

  if [[ -L "$target" ]]; then
    local existing
    existing="$(readlink "$target")"
    if [[ "$existing" == "$source" ]]; then
      return 0  # Already correct
    fi
    if [[ "$FORCE" == "true" ]]; then
      rm "$target"
    else
      warn "Symlink already exists: $target -> $existing (use --force to overwrite)"
      return 1
    fi
  elif [[ -e "$target" ]]; then
    if [[ "$FORCE" == "true" ]]; then
      rm -rf "$target"
    else
      warn "File already exists: $target (use --force to overwrite)"
      return 1
    fi
  fi

  ln -sf "$source" "$target"
  info "Linked: $target -> $source"
}

# --------------------------------------------------------------------------
# Commands
# --------------------------------------------------------------------------

cmd_init() {
  info "Initializing $AGENTS_DIR/ directory structure..."

  mkdir -p "$PROJECT_ROOT/$AGENTS_DIR/rules"
  mkdir -p "$PROJECT_ROOT/$AGENTS_DIR/skills"
  mkdir -p "$PROJECT_ROOT/$AGENTS_DIR/workflows"

  # Copy STATE.md template if STATE.md doesn't exist
  if [[ ! -f "$PROJECT_ROOT/$AGENTS_DIR/STATE.md" ]]; then
    if [[ -f "$TEMPLATES_DIR/STATE_TEMPLATE.md" ]]; then
      cp "$TEMPLATES_DIR/STATE_TEMPLATE.md" "$PROJECT_ROOT/$AGENTS_DIR/STATE.md"
      info "Created $AGENTS_DIR/STATE.md from template"
    else
      # Inline fallback if template not found
      cat > "$PROJECT_ROOT/$AGENTS_DIR/STATE.md" <<'STATE_EOF'

---
trigger: always_on
---

# State

## STATE HISTORY BELOW


STATE_EOF
      info "Created $AGENTS_DIR/STATE.md"
    fi
  else
    warn "$AGENTS_DIR/STATE.md already exists, skipping"
  fi

  # Generate AGENTS.md if it doesn't exist
  if [[ ! -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
    generate_agents_md
    info "Created $AGENTS_MD"
  else
    warn "$AGENTS_MD already exists, skipping (run 'sync-agents index' to regenerate)"
  fi

  info "Initialization complete. Directory structure:"
  print_tree "$PROJECT_ROOT/$AGENTS_DIR"
}

cmd_add() {
  local type="${1:-}"
  local name="${2:-}"

  if [[ -z "$type" ]] || [[ -z "$name" ]]; then
    error "Usage: sync-agents add <rule|skill|workflow> <name>"
    exit 1
  fi

  case "$type" in
    rule|rules)     type="rules" ;;
    skill|skills)   type="skills" ;;
    workflow|workflows) type="workflows" ;;
    *)
      error "Unknown type: $type. Must be one of: rule, skill, workflow"
      exit 1
      ;;
  esac

  ensure_agents_dir

  local filepath="$PROJECT_ROOT/$AGENTS_DIR/$type/$name.md"

  if [[ -f "$filepath" ]] && [[ "$FORCE" != "true" ]]; then
    error "File already exists: $filepath (use --force to overwrite)"
    exit 1
  fi

  # Use RULE_TEMPLATE for all types, substituting name
  if [[ -f "$TEMPLATES_DIR/RULE_TEMPLATE.md" ]]; then
    sed "s/\${NAME}/$name/g" "$TEMPLATES_DIR/RULE_TEMPLATE.md" > "$filepath"
  else
    cat > "$filepath" <<TMPL_EOF

---
trigger: always_on
---

# $name
TMPL_EOF
  fi

  info "Created $type: $filepath"

  # Regenerate index
  generate_agents_md
  info "Updated $AGENTS_MD index"
}

cmd_sync() {
  ensure_agents_dir

  local agents_abs
  agents_abs="$(cd "$PROJECT_ROOT/$AGENTS_DIR" && pwd)"

  info "Syncing $AGENTS_DIR/ to agent directories..."

  for target in "${ACTIVE_TARGETS[@]}"; do
    local target_dir="$PROJECT_ROOT/.$target"
    info "Syncing to .${target}/"

    # Sync subdirectories: rules, skills, workflows
    for subdir in rules skills workflows; do
      if [[ -d "$agents_abs/$subdir" ]]; then
        # Both .agents/ and .<target>/ live at PROJECT_ROOT, so the relative
        # path from .<target>/<subdir> back to .agents/<subdir> is always one
        # level up: ../.agents/<subdir>
        local source_rel="../$AGENTS_DIR/$subdir"
        create_symlink "$source_rel" "$target_dir/$subdir" "$DRY_RUN"
      fi
    done
  done

  # Symlink AGENTS.md -> CLAUDE.md
  if [[ -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
    create_symlink "$AGENTS_MD" "$PROJECT_ROOT/CLAUDE.md" "$DRY_RUN"
  fi

  info "Sync complete."
}

cmd_status() {
  echo -e "${BOLD}sync-agents${RESET} v${VERSION}"
  echo ""

  # Check .agents/ directory
  if [[ -d "$PROJECT_ROOT/$AGENTS_DIR" ]]; then
    echo -e "${GREEN}[ok]${RESET} $AGENTS_DIR/ exists"
    print_tree "$PROJECT_ROOT/$AGENTS_DIR"
  else
    echo -e "${RED}[missing]${RESET} $AGENTS_DIR/ not found"
  fi

  echo ""

  # Check AGENTS.md
  if [[ -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
    echo -e "${GREEN}[ok]${RESET} $AGENTS_MD exists"
  else
    echo -e "${RED}[missing]${RESET} $AGENTS_MD not found"
  fi

  # Check CLAUDE.md symlink
  if [[ -L "$PROJECT_ROOT/CLAUDE.md" ]]; then
    local link_target
    link_target="$(readlink "$PROJECT_ROOT/CLAUDE.md")"
    echo -e "${GREEN}[ok]${RESET} CLAUDE.md -> $link_target"
  elif [[ -f "$PROJECT_ROOT/CLAUDE.md" ]]; then
    echo -e "${YELLOW}[warn]${RESET} CLAUDE.md exists but is not a symlink"
  else
    echo -e "${RED}[missing]${RESET} CLAUDE.md not found"
  fi

  echo ""

  # Check each target
  for target in "${TARGETS[@]}"; do
    local target_dir="$PROJECT_ROOT/.$target"
    if [[ -d "$target_dir" ]] || [[ -L "$target_dir/rules" ]]; then
      echo -e "${CYAN}.$target/${RESET}"
      for subdir in rules skills workflows; do
        if [[ -L "$target_dir/$subdir" ]]; then
          local link_target
          link_target="$(readlink "$target_dir/$subdir")"
          echo -e "  ${GREEN}[synced]${RESET} $subdir -> $link_target"
        elif [[ -d "$target_dir/$subdir" ]]; then
          echo -e "  ${YELLOW}[local]${RESET} $subdir (not symlinked)"
        else
          echo -e "  ${RED}[missing]${RESET} $subdir"
        fi
      done
    else
      echo -e "${RED}[not synced]${RESET} .$target/"
    fi
  done
}

cmd_index() {
  ensure_agents_dir
  generate_agents_md
  info "Regenerated $AGENTS_MD"
}

cmd_clean() {
  info "Removing synced symlinks..."

  for target in "${ACTIVE_TARGETS[@]}"; do
    local target_dir="$PROJECT_ROOT/.$target"
    for subdir in rules skills workflows; do
      if [[ -L "$target_dir/$subdir" ]]; then
        rm "$target_dir/$subdir"
        info "Removed: .$target/$subdir"
      fi
    done

    # Remove target dir if empty
    if [[ -d "$target_dir" ]] && [[ -z "$(ls -A "$target_dir" 2>/dev/null)" ]]; then
      rmdir "$target_dir"
      info "Removed empty directory: .$target/"
    fi
  done

  # Remove CLAUDE.md symlink
  if [[ -L "$PROJECT_ROOT/CLAUDE.md" ]]; then
    rm "$PROJECT_ROOT/CLAUDE.md"
    info "Removed: CLAUDE.md symlink"
  fi

  info "Clean complete."
}

# --------------------------------------------------------------------------
# Index generator
# --------------------------------------------------------------------------

generate_agents_md() {
  local outfile="$PROJECT_ROOT/$AGENTS_MD"
  local agents_dir="$PROJECT_ROOT/$AGENTS_DIR"

  cat > "$outfile" <<'HEADER'

---
trigger: always_on
---

# AGENTS

> Auto-generated by [sync-agents](https://github.com/brickhouse-tech/sync-agents). Do not edit manually.
> Run `sync-agents index` to regenerate.

This file indexes all rules, skills, and workflows defined in `.agents/`.

HEADER

  {
    # Rules
    echo "## Rules"
    echo ""
    if compgen -G "$agents_dir/rules/*.md" > /dev/null 2>&1; then
      for f in "$agents_dir/rules/"*.md; do
        local name
        name="$(basename "$f" .md)"
        echo "- [$name](.agents/rules/$name.md)"
      done
    else
      echo "_No rules defined yet. Add one with \`sync-agents add rule <name>\`._"
    fi
    echo ""

    # Skills
    echo "## Skills"
    echo ""
    if compgen -G "$agents_dir/skills/*.md" > /dev/null 2>&1; then
      for f in "$agents_dir/skills/"*.md; do
        local name
        name="$(basename "$f" .md)"
        echo "- [$name](.agents/skills/$name.md)"
      done
    else
      echo "_No skills defined yet. Add one with \`sync-agents add skill <name>\`._"
    fi
    echo ""

    # Workflows
    echo "## Workflows"
    echo ""
    if compgen -G "$agents_dir/workflows/*.md" > /dev/null 2>&1; then
      for f in "$agents_dir/workflows/"*.md; do
        local name
        name="$(basename "$f" .md)"
        echo "- [$name](.agents/workflows/$name.md)"
      done
    else
      echo "_No workflows defined yet. Add one with \`sync-agents add workflow <name>\`._"
    fi
    echo ""

    # State reference
    echo "## State"
    echo ""
    echo "- [STATE.md](.agents/STATE.md)"
    echo ""
  } >> "$outfile"
}

# --------------------------------------------------------------------------
# Tree printer (lightweight, no dependency on `tree`)
# --------------------------------------------------------------------------

print_tree() {
  local dir="$1"
  local prefix="${2:-}"
  local entries=()

  # Collect entries
  while IFS= read -r entry; do
    entries+=("$entry")
  done < <(find "$dir" -maxdepth 1 -mindepth 1 -exec basename {} \; 2>/dev/null | sort)

  local count=${#entries[@]}
  if [[ $count -eq 0 ]]; then
    return 0
  fi
  local i=0

  for entry in "${entries[@]}"; do
    i=$((i + 1))
    local connector="├── "
    local child_prefix="│   "
    if [[ $i -eq $count ]]; then
      connector="└── "
      child_prefix="    "
    fi

    if [[ -d "$dir/$entry" ]]; then
      echo "${prefix}${connector}${entry}/"
      print_tree "$dir/$entry" "${prefix}${child_prefix}"
    elif [[ -L "$dir/$entry" ]]; then
      local link_target
      link_target="$(readlink "$dir/$entry")"
      echo "${prefix}${connector}${entry} -> ${link_target}"
    else
      echo "${prefix}${connector}${entry}"
    fi
  done
}

# --------------------------------------------------------------------------
# Main
# --------------------------------------------------------------------------

main() {
  local command=""
  local custom_dir=""
  local custom_targets=""
  DRY_RUN="false"
  FORCE="false"

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      -v|--version)
        echo "sync-agents v${VERSION}"
        exit 0
        ;;
      -d|--dir)
        custom_dir="$2"
        shift 2
        ;;
      --targets)
        custom_targets="$2"
        shift 2
        ;;
      --dry-run)
        DRY_RUN="true"
        shift
        ;;
      --force)
        FORCE="true"
        shift
        ;;
      -*)
        error "Unknown option: $1"
        usage
        exit 1
        ;;
      *)
        if [[ -z "$command" ]]; then
          command="$1"
        else
          # Collect remaining args for subcommands
          break
        fi
        shift
        ;;
    esac
  done

  # Resolve project root
  if [[ -n "$custom_dir" ]]; then
    PROJECT_ROOT="$(cd "$custom_dir" && pwd)"
  else
    PROJECT_ROOT="$(find_project_root)"
  fi

  # Resolve active targets
  if [[ -n "$custom_targets" ]]; then
    IFS=',' read -ra ACTIVE_TARGETS <<< "$custom_targets"
  else
    ACTIVE_TARGETS=("${TARGETS[@]}")
  fi

  # Dispatch command
  case "${command:-}" in
    init)
      cmd_init
      ;;
    sync)
      cmd_sync
      ;;
    status)
      cmd_status
      ;;
    add)
      cmd_add "$@"
      ;;
    index)
      cmd_index
      ;;
    clean)
      cmd_clean
      ;;
    "")
      usage
      exit 0
      ;;
    *)
      error "Unknown command: $command"
      usage
      exit 1
      ;;
  esac
}

main "$@"
