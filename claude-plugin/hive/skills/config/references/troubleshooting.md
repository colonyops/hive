# Config Troubleshooting

## Config Not Loading

**Problem:** Changes to config.yaml not taking effect

**Solutions:**
- Check config location: `~/.config/hive/config.yaml`
- Verify YAML syntax: use `yamllint` or online validator
- Check for error messages: run `hive ls` and look for warnings
- Restart hive TUI: exit and relaunch

## Pattern Not Matching

**Problem:** Rule not applying to expected repos

**Solutions:**
- Test regex: `echo "git@github.com:org/repo" | grep -E "pattern"`
- Check remote URL: `cd repo && git remote -v`
- Escape special chars: use `\\.` for literal dots
- Check pattern order: more specific patterns must come first

## Spawn Command Fails

**Problem:** Terminal doesn't open or crashes

**Solutions:**
- Test command manually: run with substituted variables
- Check template syntax: verify `{{ .Variable }}` format
- Quote paths: use `{{ .Path | shq }}` for spaces
- Check executable exists: `which tmux`, `which wezterm`, etc.

## Template Variable Empty

**Problem:** Variable expands to empty string

**Solutions:**
- Check variable availability: some only work in specific contexts (e.g., `.Prompt` only in `batch_spawn`)
- Verify spelling: variable names are case-sensitive
- Check session metadata: `hive ls` shows available data
- Use default values: `{{ .Variable | default "fallback" }}`
