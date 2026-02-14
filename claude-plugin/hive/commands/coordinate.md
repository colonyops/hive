---
description: Connect and coordinate with other hive agents
argument-hint: [context or instructions]
allowed-tools: Bash(hive:*)
---

Establish coordination with other hive agents based on the provided context: $ARGUMENTS

## Steps

1. **Get session identity:**
   Run `hive session info --json` to get current session ID and inbox topic.

2. **Discover other sessions:**
   Run `hive ls` to see available sessions and their status.

3. **Check inbox for existing messages:**
   Run `hive msg inbox --peek` to see if other agents have already sent coordination messages.

4. **Based on the user's context ($ARGUMENTS), determine the coordination approach:**

   - **If connecting with a specific agent:** Compose and send a message to their inbox topic (`agent.<id>.inbox`) introducing the coordination purpose and sharing the current session's inbox for replies.

   - **If broadcasting to multiple agents:** Use wildcard publishing (`agent.*.inbox`) to notify all active sessions.

   - **If requesting work or information:** Send a clear request message specifying what is needed, the expected response format, and the reply topic.

   - **If handing off work:** Include branch name, relevant issue/PR numbers, current status, and next steps in the handoff message.

5. **Report coordination status:**
   Summarize what was sent, to whom, and what to expect next. If waiting for a reply, suggest using `/hive:wait` or periodically checking `/hive:inbox`.
