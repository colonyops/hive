# Inbox Troubleshooting

## No Messages Showing

**Problem:** Running `hive msg inbox` shows no messages

**Solutions:**
- Check if in the correct session: `hive ls`
- Verify inbox topic: inbox is `agent.<session-id>.inbox`
- Check if messages exist: `hive msg inbox --all`
- Ensure the sender used the correct topic

## Messages Keep Reappearing

**Problem:** Same messages show up every time

**Solutions:**
- Use `--ack` to mark messages as read: `hive msg inbox --ack`
- Without `--ack`, messages remain unread by design

## Can't Find Specific Message

**Problem:** Looking for a message that was sent earlier

**Solutions:**
- Use `--all` to see full history: `hive msg inbox --all`
- Search with grep: `hive msg inbox --all | grep "keyword"`
- Pipe to grep for specific senders: `hive msg inbox --all | grep "agent-123"`

## Session Not Detected

**Problem:** "could not detect session from working directory" error

**Solutions:**
- Verify you're in a hive session directory: `pwd`
- Use `--session` flag explicitly: `hive msg inbox --session <id|name>`
- List sessions to find correct ID: `hive ls`
