# Wait Troubleshooting

## Timeout Reached Without Message

**Problem:** Wait command times out before message arrives

**Solutions:**
- Increase timeout: use longer `--timeout` duration
- Check topic name: verify exact topic spelling and format
- Verify sender: ensure other agent is publishing to correct topic
- Check timing: other agent may need more time to complete work

## Message Missed

**Problem:** Message was sent but wait didn't receive it

**Solutions:**
- Check timing: was message sent BEFORE wait started? Messages sent before the wait command is issued won't be detected
- Verify topic: ensure exact topic match (case-sensitive)
- Review messages: `hive msg sub --topic <topic>` to see all messages
- Check existing messages before waiting

## Wait Blocks Forever

**Problem:** Wait command never returns and never times out

**Solutions:**
- Check default timeout: `--wait` defaults to 24h, set explicit `--timeout`
- Verify topic exists: use `hive msg list` to check topics
- Test with short timeout: use `--timeout 5s` to debug
- Check polling: verify no errors in polling loop

## Wrong Message Received

**Problem:** Wait returns unexpected message

**Solutions:**
- Use specific topics: avoid overly broad wildcards
- Check message history: review with `hive msg sub --topic <topic>`
- Filter results: use `jq` or other tools to filter messages
