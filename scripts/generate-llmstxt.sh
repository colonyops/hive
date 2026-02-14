#!/usr/bin/env bash
set -euo pipefail

# Generate llms.txt and llms-full.txt from docs/ markdown files.
# Output goes to site/ directory (run after zensical build).

SITE_URL="https://colonyops.github.io/hive"
PROJECT_NAME="Hive"
PROJECT_DESC="The command center for your AI colony — manage multiple AI agent sessions in isolated git environments with real-time status monitoring."

DOCS_DIR="docs"
OUT_DIR="site"

mkdir -p "$OUT_DIR"

# Navigation order — matches zensical.toml nav structure.
# Format: "Section Name|relative/path.md|Link Title|Description"
NAV=(
  "Getting Started|getting-started/index.md|Getting Started|Quick start, prerequisites, and first session"
  "Getting Started|getting-started/sessions.md|Sessions|Sessions, agents, lifecycle, and status indicators"
  "Getting Started|getting-started/context.md|Context & Review|Shared context directories and the review tool"
  "Getting Started|getting-started/messaging.md|Messaging|Inter-agent pub/sub communication"
  "Configuration|configuration/index.md|Configuration|Config file overview, options reference, and data storage"
  "Configuration|configuration/rules.md|Rules|Repository-specific rules, templates, and pattern matching"
  "Configuration|configuration/commands.md|User Commands|User commands, command palette, and form fields"
  "Configuration|configuration/keybindings.md|Keybindings|Key mappings, default keys, and palette commands"
  "Configuration|configuration/plugins.md|Plugins|Tmux, Claude, GitHub, and Beads plugin configuration"
  "Configuration|configuration/themes.md|Themes|Built-in themes and custom color palettes"
  "Reference|cli-reference.md|CLI Reference|All CLI commands and flags"
  "Recipes|recipes/inter-agent-code-review.md|Inter-Agent Code Review|Using messaging for collaborative code review"
  "FAQ|faq.md|FAQ|Common questions and answers"
)

# --- Generate llms.txt ---

{
  echo "# ${PROJECT_NAME}"
  echo ""
  echo "> ${PROJECT_DESC}"
  echo ""

  current_section=""
  for entry in "${NAV[@]}"; do
    IFS='|' read -r section path title desc <<< "$entry"

    if [[ "$section" != "$current_section" ]]; then
      # Blank line before new section (except the first)
      if [[ -n "$current_section" ]]; then
        echo ""
      fi
      echo "## ${section}"
      echo ""
      current_section="$section"
    fi

    # URL: strip index.md, convert .md to /
    url_path="${path%.md}"
    url_path="${url_path%/index}"
    echo "- [${title}](${SITE_URL}/${url_path}/): ${desc}"
  done

  echo ""
} > "${OUT_DIR}/llms.txt"

# --- Generate llms-full.txt ---

{
  echo "# ${PROJECT_NAME}"
  echo ""
  echo "> ${PROJECT_DESC}"
  echo ""

  current_section=""
  for entry in "${NAV[@]}"; do
    IFS='|' read -r section path title desc <<< "$entry"

    if [[ "$section" != "$current_section" ]]; then
      echo "## ${section}"
      echo ""
      current_section="$section"
    fi

    filepath="${DOCS_DIR}/${path}"
    if [[ ! -f "$filepath" ]]; then
      echo "Warning: ${filepath} not found, skipping" >&2
      continue
    fi

    # Shift headings down by 2 levels (# -> ###, ## -> ####, etc.)
    # Uses awk to skip lines inside fenced code blocks.
    awk '
      /^```/ { in_code = !in_code }
      !in_code && /^###### / { sub(/^###### /, "######## "); print; next }
      !in_code && /^##### /  { sub(/^##### /,  "####### ");  print; next }
      !in_code && /^#### /   { sub(/^#### /,   "###### ");   print; next }
      !in_code && /^### /    { sub(/^### /,    "##### ");    print; next }
      !in_code && /^## /     { sub(/^## /,     "#### ");     print; next }
      !in_code && /^# /      { sub(/^# /,      "### ");      print; next }
      { print }
    ' "$filepath"
    echo ""
    echo "---"
    echo ""
  done
} > "${OUT_DIR}/llms-full.txt"

echo "Generated ${OUT_DIR}/llms.txt ($(wc -c < "${OUT_DIR}/llms.txt" | tr -d ' ') bytes)"
echo "Generated ${OUT_DIR}/llms-full.txt ($(wc -c < "${OUT_DIR}/llms-full.txt" | tr -d ' ') bytes)"
