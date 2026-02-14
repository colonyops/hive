#!/bin/bash
set -euo pipefail

# Check if hive CLI is available
if ! command -v hive &>/dev/null; then
  exit 0
fi

# Check if we're in a hive session (hive session info exits non-zero if not)
if ! hive session info --json &>/dev/null; then
  exit 0
fi

# Peek at inbox without marking messages as read
messages=$(hive msg inbox --peek 2>/dev/null || echo "")

# Skip if no output or empty
if [ -z "$messages" ]; then
  exit 0
fi

# Count lines (each message is one or more lines, count non-empty lines)
count=$(echo "$messages" | grep -c '.' 2>/dev/null || echo "0")

if [ "$count" -gt 0 ]; then
  # Truncate preview to first 500 chars to avoid huge output
  preview=$(echo "$messages" | head -c 500)

  cat <<EOF
{
  "systemMessage": "You have unread message(s) in your hive inbox. Run \`hive msg inbox\` to read and acknowledge them.\n\nPreview:\n$preview"
}
EOF
fi
