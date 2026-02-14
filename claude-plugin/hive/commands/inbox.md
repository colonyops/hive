---
description: Check inbox for inter-agent messages
allowed-tools: Bash(hive:*)
---

Check the current hive session inbox for unread messages.

Run: `hive msg inbox`

If there are unread messages:
1. Read each message carefully
2. Summarize who sent what and when
3. Identify any action items, handoff requests, or tasks mentioned
4. If messages reference issues, PRs, or branches, note them for follow-up

If there are no unread messages:
1. Report that the inbox is empty
2. Suggest using `/hive:publish` to send messages to other agents if coordination is needed
