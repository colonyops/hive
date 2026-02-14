# Inbox Troubleshooting

## No Messages Showing

**Problem:** Running `hive msg inbox` shows no messages

**Solutions:**
- Check if in the correct session: `hive ls`
- Verify inbox topic: inbox is `agent.<session-id>.inbox`
- Check if messages exist: `hive msg inbox --all`
- Ensure the sender used the correct topic

## Messages Not Marked as Read

**Problem:** Messages keep showing as unread

**Solutions:**
- Ensure NOT using `--peek` or `--all` flags (these don't mark as read)
- Use default command: `hive msg inbox`
- Check message file permissions in `$XDG_DATA_HOME/hive/messages/topics/`

## Can't Find Specific Message

**Problem:** Looking for a message that was sent earlier

**Solutions:**
- Use `--all` to see full history: `hive msg inbox --all`
- Search with grep: `hive msg inbox --all | grep "keyword"`
- Pipe to grep for specific senders: `hive msg inbox --all | grep "agent-123"`

## Acknowledgment Not Received

**Problem:** Sender doesn't receive acknowledgment after message is read

**Solutions:**
- Verify sender is subscribed to `<topic>.ack`
- Ensure recipient read with `hive msg inbox` (not `--peek` or `--all`)
- Check acknowledgment topic exists
