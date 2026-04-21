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
TARGETS=("claude" "windsurf" "cursor" "copilot")

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

# Resolve target directory path (copilot uses .github/copilot/ instead of .copilot/)
resolve_target_dir() {
  local target="$1"
  local root="$2"
  if [[ "$target" == "copilot" ]]; then
    echo "$root/.github/copilot"
  else
    echo "$root/.$target"
  fi
}

# Resolve relative path from target dir back to .agents/ (accounts for depth)
resolve_agents_rel() {
  local target="$1"
  if [[ "$target" == "copilot" ]]; then
    echo "../../$AGENTS_DIR"
  else
    echo "../$AGENTS_DIR"
  fi
}

usage() {
  cat <<EOF
${BOLD}sync-agents${RESET} v${VERSION} - One set of agent rules to rule them all.

${BOLD}USAGE${RESET}
  sync-agents <command> [options]

${BOLD}COMMANDS${RESET}
  init                          Initialize .agents/ directory structure and AGENTS.md
  sync                          Sync .agents/ to agent directories via symlinks
  status                        Show current sync status
  add <type> <name>             Add a new rule, skill, or workflow from template
  index                         Regenerate AGENTS.md index from .agents/ contents
  clean                         Remove all synced symlinks (does not remove .agents/)
  watch                         Watch .agents/ for changes and auto-regenerate index
  import <url>                  Import a rule/skill/workflow from a URL
  hook                          Install a pre-commit git hook for auto-sync
  fix [type]                    Migrate legacy dirs into .agents/ (type: skills, rules, workflows, or all)
  inherit <label> <path>        Add an inheritance link to AGENTS.md (convention-based)
  inherit --list                List current inheritance links
  inherit --remove <label>      Remove an inheritance link by label

${BOLD}OPTIONS${RESET}
  -h, --help                    Show this help message
  -v, --version                 Show version
  -d, --dir <path>              Set project root directory (default: current directory)
  --targets <list>              Comma-separated targets (overrides .agents/config)
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

  # Create default config if it doesn't exist
  if [[ ! -f "$PROJECT_ROOT/$AGENTS_DIR/config" ]]; then
    cat > "$PROJECT_ROOT/$AGENTS_DIR/config" <<CONFIG_EOF
# sync-agents configuration
# Comma-separated list of sync targets (available: claude, windsurf, cursor, copilot)
# Override per-command with: sync-agents sync --targets claude,cursor
targets = claude,windsurf,cursor,copilot
CONFIG_EOF
    info "Created $AGENTS_DIR/config"
  else
    warn "$AGENTS_DIR/config already exists, skipping"
  fi

  # Generate AGENTS.md if it doesn't exist
  if [[ ! -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
    generate_agents_md
    info "Created $AGENTS_MD"
  else
    warn "$AGENTS_MD already exists, skipping (run 'sync-agents index' to regenerate)"
  fi

  # Add default .gitignore entries for agent tool directories
  add_default_gitignore_entries

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

  # Skills use directory layout: skills/name/SKILL.md
  # Rules and workflows use flat files: rules/name.md, workflows/name.md
  local filepath
  if [[ "$type" == "skills" ]]; then
    filepath="$PROJECT_ROOT/$AGENTS_DIR/$type/$name/SKILL.md"
  else
    filepath="$PROJECT_ROOT/$AGENTS_DIR/$type/$name.md"
  fi

  if [[ -f "$filepath" ]] && [[ "$FORCE" != "true" ]]; then
    error "File already exists: $filepath (use --force to overwrite)"
    exit 1
  fi

  # Use type-specific template (RULE_TEMPLATE, SKILL_TEMPLATE, WORKFLOW_TEMPLATE)
  local template_name
  case "$type" in
    rules)     template_name="RULE_TEMPLATE.md" ;;
    skills)    template_name="SKILL_TEMPLATE.md" ;;
    workflows) template_name="WORKFLOW_TEMPLATE.md" ;;
    *)         template_name="RULE_TEMPLATE.md" ;;
  esac

  # Create parent directory for skills
  mkdir -p "$(dirname "$filepath")"

  if [[ -f "$TEMPLATES_DIR/$template_name" ]]; then
    sed "s/\${NAME}/$name/g" "$TEMPLATES_DIR/$template_name" > "$filepath"
  elif [[ -f "$TEMPLATES_DIR/RULE_TEMPLATE.md" ]]; then
    # Fallback to rule template if type-specific template missing
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

cmd_fix() {
  ensure_agents_dir

  local fix_type="${1:-all}"
  local subdirs=()

  case "$fix_type" in
    skills|rules|workflows)
      subdirs=("$fix_type")
      ;;
    all)
      subdirs=(skills rules workflows)
      ;;
    *)
      error "Unknown type: $fix_type (expected: skills, rules, workflows, or all)"
      exit 1
      ;;
  esac

  local agents_abs
  agents_abs="$(cd "$PROJECT_ROOT/$AGENTS_DIR" && pwd)"
  local fixed=0

  for subdir in "${subdirs[@]}"; do
    local legacy_dir="$PROJECT_ROOT/$subdir"
    local agents_subdir="$agents_abs/$subdir"

    # Skip if legacy dir doesn't exist or is already a symlink
    if [[ ! -d "$legacy_dir" ]] || [[ -L "$legacy_dir" ]]; then
      continue
    fi

    info "Found legacy directory: $subdir/"
    mkdir -p "$agents_subdir"

    # Move each item from legacy dir into .agents/subdir
    for item in "$legacy_dir"/*/; do
      [[ -d "$item" ]] || continue
      local name
      name="$(basename "$item")"

      if [[ -d "$agents_subdir/$name" ]]; then
        warn "Skipping $subdir/$name — already exists in $AGENTS_DIR/$subdir/"
        continue
      fi

      if [[ "$DRY_RUN" == "true" ]]; then
        echo "  would move: $subdir/$name -> $AGENTS_DIR/$subdir/$name"
      else
        mv "$item" "$agents_subdir/$name"
        info "Moved: $subdir/$name -> $AGENTS_DIR/$subdir/$name"
      fi
      ((fixed++))
    done

    # Also move any top-level files (e.g. loose .md rules)
    for item in "$legacy_dir"/*; do
      [[ -f "$item" ]] || continue
      local name
      name="$(basename "$item")"

      if [[ -f "$agents_subdir/$name" ]]; then
        warn "Skipping $subdir/$name — already exists in $AGENTS_DIR/$subdir/"
        continue
      fi

      if [[ "$DRY_RUN" == "true" ]]; then
        echo "  would move: $subdir/$name -> $AGENTS_DIR/$subdir/$name"
      else
        mv "$item" "$agents_subdir/$name"
        info "Moved: $subdir/$name -> $AGENTS_DIR/$subdir/$name"
      fi
      ((fixed++))
    done

    # Remove the now-empty legacy dir and replace with symlink
    if [[ "$DRY_RUN" == "true" ]]; then
      echo "  would replace $subdir/ with symlink -> $AGENTS_DIR/$subdir"
    else
      # Check if dir is empty (only . and .. remain)
      if [[ -z "$(ls -A "$legacy_dir" 2>/dev/null)" ]]; then
        rmdir "$legacy_dir"
        ln -s "$AGENTS_DIR/$subdir" "$legacy_dir"
        info "Replaced $subdir/ with symlink -> $AGENTS_DIR/$subdir"
      else
        warn "$subdir/ is not empty after migration — skipping symlink replacement"
        warn "Remaining items:"
        ls -A "$legacy_dir" | sed 's/^/    /'
      fi
    fi
  done

  if [[ "$fixed" -eq 0 ]]; then
    info "Nothing to fix — all directories are already in $AGENTS_DIR/ or symlinked."
  else
    info "Fixed $fixed item(s). Run 'sync-agents sync' to update agent target symlinks."
  fi
}

cmd_sync() {
  ensure_agents_dir

  local agents_abs
  agents_abs="$(cd "$PROJECT_ROOT/$AGENTS_DIR" && pwd)"

  info "Syncing $AGENTS_DIR/ to agent directories..."

  for target in "${ACTIVE_TARGETS[@]}"; do
    local target_dir
    target_dir="$(resolve_target_dir "$target" "$PROJECT_ROOT")"
    local agents_rel
    agents_rel="$(resolve_agents_rel "$target")"
    info "Syncing to ${target_dir#"$PROJECT_ROOT"/}/"

    # Sync subdirectories: rules, skills, workflows
    for subdir in rules skills workflows; do
      if [[ -d "$agents_abs/$subdir" ]]; then
        local source_rel="$agents_rel/$subdir"
        create_symlink "$source_rel" "$target_dir/$subdir" "$DRY_RUN"
      fi
    done
  done

  # Symlink AGENTS.md -> CLAUDE.md
  if [[ -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
    create_symlink "$AGENTS_MD" "$PROJECT_ROOT/CLAUDE.md" "$DRY_RUN"
  fi

  # Update .gitignore with synced symlink entries
  update_gitignore

  info "Sync complete."
}

# --------------------------------------------------------------------------
# .gitignore management
# --------------------------------------------------------------------------

# Add default .gitignore entries for agent tool directories (called during init)
add_default_gitignore_entries() {
  local gitignore="$PROJECT_ROOT/.gitignore"
  
  # Create .gitignore if it doesn't exist
  if [[ ! -f "$gitignore" ]]; then
    touch "$gitignore"
    info "Created .gitignore"
  fi

  # Check if .DS_Store is already present (case-insensitive check)
  if ! grep -qiE "^\.DS_Store$" "$gitignore" 2>/dev/null; then
    # Add .DS_Store if not present
    if [[ -s "$gitignore" ]] && ! tail -c1 "$gitignore" | grep -q '^$'; then
      echo "" >> "$gitignore"
    fi
    echo ".DS_Store" >> "$gitignore"
    info "Added .DS_Store to .gitignore"
  fi

  # Define default entries (tool artifacts, not symlinks)
  # Using pattern: ignore everything in dir, except specific files we want to track
  local marker="# sync-agents — ignore tool artifacts, keep symlinks"
  
  # Check if sync-agents section already exists
  if grep -qF "$marker" "$gitignore"; then
    # Section exists - check if we need to add any missing entries
    local needs_update=false
    
    # Check for each pattern
    if ! grep -qF ".cursor/*" "$gitignore"; then needs_update=true; fi
    if ! grep -qF "!.cursor/rules" "$gitignore"; then needs_update=true; fi
    if ! grep -qF ".codex/*" "$gitignore"; then needs_update=true; fi
    if ! grep -qF "!.codex/instructions.md" "$gitignore"; then needs_update=true; fi
    if ! grep -qF ".github/copilot/*" "$gitignore"; then needs_update=true; fi
    if ! grep -qF "!.github/copilot/instructions.md" "$gitignore"; then needs_update=true; fi
    
    if [[ "$needs_update" == "true" ]]; then
      # Rebuild section by reading the file, preserving everything else
      local tmp
      tmp="$(mktemp)"
      local in_section=false
      
      while IFS= read -r line; do
        if [[ "$line" == "$marker" ]]; then
          in_section=true
          # Output the marker
          {
            echo "$line"
            echo ".cursor/*"
            echo "!.cursor/rules"
            echo ".codex/*"
            echo "!.codex/instructions.md"
            echo ".github/copilot/*"
            echo "!.github/copilot/instructions.md"
          } >> "$tmp"
          continue
        fi
        
        # Skip old entries in the sync-agents section (until we hit empty line or new section)
        if [[ "$in_section" == "true" ]]; then
          if [[ -z "$line" ]] || [[ "$line" == "#"* ]]; then
            in_section=false
            echo "$line" >> "$tmp"
          fi
          # Skip old entry lines (they're replaced above)
          continue
        fi
        
        echo "$line" >> "$tmp"
      done < "$gitignore"
      
      mv "$tmp" "$gitignore"
      info "Updated sync-agents section in .gitignore"
    fi
  else
    # Section doesn't exist, add entire block
    # Add separator if file is non-empty
    if [[ -s "$gitignore" ]] && ! tail -c1 "$gitignore" | grep -q '^$'; then
      echo "" >> "$gitignore"
    fi
    
    # Add all entries
    {
      echo "$marker"
      echo ".cursor/*"
      echo "!.cursor/rules"
      echo ".codex/*"
      echo "!.codex/instructions.md"
      echo ".github/copilot/*"
      echo "!.github/copilot/instructions.md"
    } >> "$gitignore"
    
    info "Added sync-agents section to .gitignore with 7 entries"
  fi
}

update_gitignore() {
  local gitignore="$PROJECT_ROOT/.gitignore"

  # Build list of entries that should be ignored (synced symlinks)
  local entries=()
  for target in "${ACTIVE_TARGETS[@]}"; do
    local target_dir
    target_dir="$(resolve_target_dir "$target" "$PROJECT_ROOT")"
    local rel_path="${target_dir#"$PROJECT_ROOT"/}/"
    entries+=("$rel_path")
  done
  entries+=("CLAUDE.md")

  if [[ "$DRY_RUN" == "true" ]]; then
    for entry in "${entries[@]}"; do
      if [[ ! -f "$gitignore" ]] || ! grep -qxF "$entry" "$gitignore"; then
        echo "  would add to .gitignore: $entry"
      fi
    done
    return 0
  fi

  # Create .gitignore if it doesn't exist
  [[ -f "$gitignore" ]] || touch "$gitignore"

  local added=0
  for entry in "${entries[@]}"; do
    if ! grep -qxF "$entry" "$gitignore"; then
      # Add sync-agents header on first addition
      if [[ "$added" -eq 0 ]]; then
        # Check if header already exists
        if ! grep -qF "# sync-agents" "$gitignore"; then
          # Add a blank line separator if file is non-empty
          if [[ -s "$gitignore" ]]; then
            echo "" >> "$gitignore"
          fi
          echo "# sync-agents (generated symlinks)" >> "$gitignore"
        fi
      fi
      echo "$entry" >> "$gitignore"
      added=$((added + 1))
    fi
  done

  if [[ "$added" -gt 0 ]]; then
    info "Added $added entries to .gitignore"
  fi
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
    local target_dir
    target_dir="$(resolve_target_dir "$target" "$PROJECT_ROOT")"
    local display_dir="${target_dir#"$PROJECT_ROOT"/}"
    if [[ -d "$target_dir" ]] || [[ -L "$target_dir/rules" ]]; then
      echo -e "${CYAN}${display_dir}/${RESET}"
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
      echo -e "${RED}[not synced]${RESET} ${display_dir}/"
    fi
  done
}

cmd_index() {
  ensure_agents_dir
  generate_agents_md
  info "Regenerated $AGENTS_MD"
}

cmd_watch() {
  ensure_agents_dir

  local watch_dir="$PROJECT_ROOT/$AGENTS_DIR"

  if command -v fswatch >/dev/null 2>&1; then
    info "Watching $AGENTS_DIR/ for changes... (Ctrl+C to stop)"
    cmd_index
    fswatch -o "$watch_dir" | while read -r _; do
      info "Change detected, regenerating index..."
      cmd_index
    done
  elif command -v inotifywait >/dev/null 2>&1; then
    info "Watching $AGENTS_DIR/ for changes... (Ctrl+C to stop)"
    cmd_index
    inotifywait -m -r -e modify,create,delete,move --format '%w%f' "$watch_dir" | while read -r _; do
      info "Change detected, regenerating index..."
      cmd_index
    done
  else
    error "Neither fswatch (macOS) nor inotifywait (Linux) found."
    error "Install with: brew install fswatch  OR  apt install inotify-tools"
    exit 1
  fi
}

cmd_import() {
  local url="${1:-}"
  if [[ -z "$url" ]]; then
    error "Usage: sync-agents import <url>"
    exit 1
  fi

  ensure_agents_dir

  local filename
  filename="$(basename "$url")"
  if [[ "$filename" != *.md ]]; then
    filename="${filename}.md"
  fi

  # Auto-detect type from URL path
  local type=""
  case "$url" in
    */rules/*)    type="rules" ;;
    */skills/*)   type="skills" ;;
    */workflows/*) type="workflows" ;;
  esac

  if [[ -z "$type" ]]; then
    echo "Could not detect type from URL. Choose:"
    echo "  1) rule"
    echo "  2) skill"
    echo "  3) workflow"
    read -rp "Selection (1-3): " choice
    case "$choice" in
      1) type="rules" ;;
      2) type="skills" ;;
      3) type="workflows" ;;
      *) error "Invalid selection"; exit 1 ;;
    esac
  fi

  mkdir -p "$PROJECT_ROOT/$AGENTS_DIR/$type"
  local dest="$PROJECT_ROOT/$AGENTS_DIR/$type/$filename"

  info "Importing $url → $AGENTS_DIR/$type/$filename"

  if ! curl -fsSL "$url" -o "$dest"; then
    error "Failed to download: $url"
    exit 1
  fi

  info "Imported successfully."
  cmd_index
}

cmd_hook() {
  if [[ ! -d "$PROJECT_ROOT/.git" ]]; then
    error "Not a git repository (no .git/ found)."
    exit 1
  fi

  local hook_dir="$PROJECT_ROOT/.git/hooks"
  local hook_file="$hook_dir/pre-commit"
  mkdir -p "$hook_dir"

  local marker="sync-agents start"

  if [[ -f "$hook_file" ]] && grep -q "$marker" "$hook_file"; then
    info "Git hook already installed in $hook_file"
    return 0
  fi

  local hook_block
  hook_block="$(cat <<'HOOK'

# --- sync-agents start ---
if command -v sync-agents >/dev/null 2>&1; then
  sync-agents sync 2>/dev/null
  sync-agents index 2>/dev/null
  git add AGENTS.md CLAUDE.md .claude/ .windsurf/ .cursor/ .github/copilot/ 2>/dev/null || true
fi
# --- sync-agents end ---
HOOK
)"

  if [[ -f "$hook_file" ]]; then
    echo "$hook_block" >> "$hook_file"
    info "Appended sync-agents hook to existing $hook_file"
  else
    printf '#!/bin/sh\n%s\n' "$hook_block" > "$hook_file"
    chmod +x "$hook_file"
    info "Created git hook: $hook_file"
  fi
}

cmd_clean() {
  info "Removing synced symlinks..."

  for target in "${ACTIVE_TARGETS[@]}"; do
    local target_dir
    target_dir="$(resolve_target_dir "$target" "$PROJECT_ROOT")"
    local display_dir="${target_dir#"$PROJECT_ROOT"/}"
    for subdir in rules skills workflows; do
      if [[ -L "$target_dir/$subdir" ]]; then
        rm "$target_dir/$subdir"
        info "Removed: ${display_dir}/$subdir"
      fi
    done

    # Remove target dir if empty
    if [[ -d "$target_dir" ]] && [[ -z "$(ls -A "$target_dir" 2>/dev/null)" ]]; then
      rmdir "$target_dir"
      info "Removed empty directory: ${display_dir}/"
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
# Inherit
# --------------------------------------------------------------------------

cmd_inherit() {
  local action="${1:-}"

  # --list: show current inherits
  if [[ "$action" == "--list" ]]; then
    if [[ ! -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
      info "No AGENTS.md found."
      return 0
    fi
    local in_section="false"
    while IFS= read -r line; do
      if [[ "$line" =~ ^##[[:space:]]+Inherits ]]; then
        in_section="true"
        continue
      fi
      if [[ "$in_section" == "true" ]] && [[ "$line" =~ ^## ]]; then
        break
      fi
      if [[ "$in_section" == "true" ]] && [[ "$line" =~ ^-[[:space:]]+\[ ]]; then
        echo "$line"
      fi
    done < "$PROJECT_ROOT/$AGENTS_MD"
    return 0
  fi

  # --remove <label>: remove an inherit entry
  if [[ "$action" == "--remove" ]]; then
    local label="${2:-}"
    if [[ -z "$label" ]]; then
      error "Usage: sync-agents inherit --remove <label>"
      exit 1
    fi
    if [[ ! -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
      error "No AGENTS.md found."
      exit 1
    fi
    # Remove the line matching [label](...) from the Inherits section
    local tmp
    tmp="$(mktemp)"
    local in_section="false"
    local removed="false"
    while IFS= read -r line; do
      if [[ "$line" =~ ^##[[:space:]]+Inherits ]]; then
        in_section="true"
        echo "$line" >> "$tmp"
        continue
      fi
      if [[ "$in_section" == "true" ]] && [[ "$line" =~ ^## ]]; then
        in_section="false"
      fi
      if [[ "$in_section" == "true" ]] && [[ "$line" == *"[$label]("* ]]; then
        removed="true"
        continue
      fi
      echo "$line" >> "$tmp"
    done < "$PROJECT_ROOT/$AGENTS_MD"
    mv "$tmp" "$PROJECT_ROOT/$AGENTS_MD"
    if [[ "$removed" == "true" ]]; then
      info "Removed inherit: $label"
    else
      warn "No inherit found with label: $label"
    fi
    return 0
  fi

  # Default: add <label> <path>
  local label="$action"
  local path="${2:-}"

  if [[ -z "$label" ]] || [[ -z "$path" ]]; then
    error "Usage: sync-agents inherit <label> <path>"
    error "       sync-agents inherit --list"
    error "       sync-agents inherit --remove <label>"
    exit 1
  fi

  # Validate the path exists (resolve relative to PROJECT_ROOT)
  local resolved_path
  if [[ "$path" == /* ]] || [[ "$path" == ~* ]]; then
    resolved_path="${path/#\~/$HOME}"
  else
    resolved_path="$PROJECT_ROOT/$path"
  fi

  if [[ ! -f "$resolved_path" ]] && [[ ! -d "$resolved_path" ]]; then
    warn "Path does not exist: $path (link will be added anyway)"
  fi

  # Check if AGENTS.md exists
  if [[ ! -f "$PROJECT_ROOT/$AGENTS_MD" ]]; then
    error "No AGENTS.md found. Run 'sync-agents init' first."
    exit 1
  fi

  # Check if Inherits section exists; if not, add it after the header
  if ! grep -q "^## Inherits" "$PROJECT_ROOT/$AGENTS_MD"; then
    # Insert Inherits section right after the header block (after first blank line following description)
    local tmp
    tmp="$(mktemp)"
    local header_done="false"
    local inherits_written="false"
    while IFS= read -r line; do
      echo "$line" >> "$tmp"
      # Write inherits section after the description paragraph (first line starting with "This file")
      if [[ "$header_done" == "false" ]] && [[ "$line" == "This file indexes"* ]]; then
        header_done="true"
        {
          echo ""
          echo "## Inherits"
          echo ""
          echo "- [$label]($path)"
        } >> "$tmp"
        inherits_written="true"
      fi
    done < "$PROJECT_ROOT/$AGENTS_MD"
    # Fallback: if header pattern wasn't found, append at the end before ## Rules
    if [[ "$inherits_written" == "false" ]]; then
      rm "$tmp"
      tmp="$(mktemp)"
      while IFS= read -r line; do
        if [[ "$line" == "## Rules" ]] && [[ "$inherits_written" == "false" ]]; then
          {
            echo "## Inherits"
            echo ""
            echo "- [$label]($path)"
            echo ""
          } >> "$tmp"
          inherits_written="true"
        fi
        echo "$line" >> "$tmp"
      done < "$PROJECT_ROOT/$AGENTS_MD"
    fi
    mv "$tmp" "$PROJECT_ROOT/$AGENTS_MD"
  else
    # Inherits section exists — check for duplicate label
    if grep -q "\[$label\](" "$PROJECT_ROOT/$AGENTS_MD"; then
      warn "Inherit with label '$label' already exists. Use --remove first to update."
      return 1
    fi
    # Append to existing Inherits section (after last inherit entry or section header)
    local tmp
    tmp="$(mktemp)"
    local in_section="false"
    local added="false"
    while IFS= read -r line; do
      if [[ "$line" =~ ^##[[:space:]]+Inherits ]]; then
        in_section="true"
        echo "$line" >> "$tmp"
        continue
      fi
      # When we hit the next section or blank line after entries, insert
      if [[ "$in_section" == "true" ]] && [[ "$added" == "false" ]]; then
        if [[ "$line" =~ ^## ]] || [[ -z "$line" ]]; then
          # Check if previous content had entries; add after them
          if [[ "$line" =~ ^## ]]; then
            {
              echo "- [$label]($path)"
              echo ""
            } >> "$tmp"
            added="true"
            in_section="false"
          fi
        fi
        if [[ "$line" =~ ^-[[:space:]]+\[ ]]; then
          echo "$line" >> "$tmp"
          continue
        fi
        if [[ -z "$line" ]] && [[ "$added" == "false" ]]; then
          echo "- [$label]($path)" >> "$tmp"
          added="true"
          in_section="false"
          echo "$line" >> "$tmp"
          continue
        fi
      fi
      echo "$line" >> "$tmp"
    done < "$PROJECT_ROOT/$AGENTS_MD"
    # If we never added (section was at end of file)
    if [[ "$added" == "false" ]]; then
      echo "- [$label]($path)" >> "$tmp"
      echo "" >> "$tmp"
    fi
    mv "$tmp" "$PROJECT_ROOT/$AGENTS_MD"
  fi

  info "Added inherit: [$label]($path)"
}

# --------------------------------------------------------------------------
# Index generator
# --------------------------------------------------------------------------

generate_agents_md() {
  local outfile="$PROJECT_ROOT/$AGENTS_MD"
  local agents_dir="$PROJECT_ROOT/$AGENTS_DIR"

  # Preserve existing Inherits section before regenerating
  local inherits_block=""
  if [[ -f "$outfile" ]]; then
    local in_section="false"
    while IFS= read -r line; do
      if [[ "$line" =~ ^##[[:space:]]+Inherits ]]; then
        in_section="true"
        inherits_block+="$line"$'\n'
        continue
      fi
      if [[ "$in_section" == "true" ]] && [[ "$line" =~ ^## ]]; then
        break
      fi
      if [[ "$in_section" == "true" ]]; then
        inherits_block+="$line"$'\n'
      fi
    done < "$outfile"
  fi

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
    # Inherits (preserved from previous AGENTS.md)
    if [[ -n "$inherits_block" ]]; then
      printf '%s\n' "$inherits_block"
    fi

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

    # Skills (directory layout: skills/name/SKILL.md, or legacy flat: skills/name.md)
    echo "## Skills"
    echo ""
    local has_skills="false"
    # Directory skills: skills/name/SKILL.md
    for d in "$agents_dir/skills/"*/; do
      [[ -d "$d" ]] || continue
      local name
      name="$(basename "$d")"
      if [[ -f "$d/SKILL.md" ]]; then
        echo "- [$name](.agents/skills/$name/SKILL.md)"
        has_skills="true"
      fi
    done
    # Legacy flat skills: skills/name.md
    if compgen -G "$agents_dir/skills/*.md" > /dev/null 2>&1; then
      for f in "$agents_dir/skills/"*.md; do
        local name
        name="$(basename "$f" .md)"
        echo "- [$name](.agents/skills/$name.md)"
        has_skills="true"
      done
    fi
    if [[ "$has_skills" == "false" ]]; then
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
        if [[ -n "$command" ]]; then
          # Unknown flag after command — pass to subcommand
          break
        fi
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

  # Resolve active targets (priority: --targets flag > .agents/config > built-in defaults)
  if [[ -n "$custom_targets" ]]; then
    IFS=',' read -ra ACTIVE_TARGETS <<< "$custom_targets"
  else
    local config_file="$PROJECT_ROOT/$AGENTS_DIR/config"
    if [[ -f "$config_file" ]]; then
      local config_targets
      config_targets="$(sed -n 's/^targets *= *//p' "$config_file" | tr -d ' ')"
      if [[ -n "$config_targets" ]]; then
        IFS=',' read -ra ACTIVE_TARGETS <<< "$config_targets"
      else
        ACTIVE_TARGETS=("${TARGETS[@]}")
      fi
    else
      ACTIVE_TARGETS=("${TARGETS[@]}")
    fi
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
    watch)
      cmd_watch
      ;;
    import)
      cmd_import "$@"
      ;;
    fix)
      cmd_fix "$@"
      ;;
    hook)
      cmd_hook
      ;;
    inherit)
      cmd_inherit "$@"
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
