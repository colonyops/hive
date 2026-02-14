# Publish Troubleshooting

## Message Not Received

**Problem:** Published message doesn't appear in recipient's inbox

**Solutions:**
- Verify topic name: check recipient's session ID with `hive ls`
- Correct format: use `agent.<session-id>.inbox` for direct messages
- Check message file: look in `$XDG_DATA_HOME/hive/messages/topics/<topic>.json`

## Wildcard Not Expanding

**Problem:** Wildcard pattern doesn't match expected topics

**Solutions:**
- Check existing topics: `hive msg list`
- Verify pattern syntax: wildcards only work within segments
- Create topics first: topics must exist to be matched

## Sender ID Incorrect

**Problem:** Messages show wrong sender

**Solutions:**
- Verify session context: run from correct hive session directory
- Check session: `hive ls` to see active sessions
- Override if needed: use `--sender` flag explicitly

## Large Message Failures

**Problem:** Publishing large content fails or truncates

**Solutions:**
- Use file-based publishing: `hive msg pub -t topic -f large-file.md`
- Pipe content: `cat report.txt | hive msg pub -t topic`
- Split large messages across multiple publishes
