# Session Info Troubleshooting

## Not in a Hive Session

**Problem:** Command fails with "not in a hive session" error

**Solutions:**
- Verify working directory: `pwd`
- Check if in hive session: look for `.hive-session` file
- List sessions: `hive ls`
- Change to correct session directory

## Session State is "corrupted"

**Problem:** Session info shows state as "corrupted"

**Solutions:**
- Check session directory exists: verify path from output
- Look for missing files: `.hive-session`, tmux socket, etc.
- Consider recreating session
- Check disk space and permissions

## Inbox Topic Not Working

**Problem:** Messages sent to inbox don't appear

**Solutions:**
- Verify exact inbox topic: copy from `hive session info` output
- Check for typos: topics are case-sensitive
- Confirm messages exist: `hive msg sub --topic <your-inbox>`
- Test with known-good message: send to yourself
