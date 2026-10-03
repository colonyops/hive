package content_test

type scorerFixture struct {
	name          string
	content       string
	expectedAgent bool
	expectedTool  string
	purpose       string
}

var scorerFixtures = []scorerFixture{
	{
		name: "claude busy session",
		content: `Claude Code

✳ thinking... (12s · 1.2k tokens)
Read(internal/core/terminal/content/scorer.go)
● Inspecting project files
ctrl+c to interrupt
## Plan
`,
		expectedAgent: true,
		expectedTool:  "claude",
		purpose:       "Detect active Claude Code sessions with thinking, tool-use, interrupt, and markdown signals.",
	},
	{
		name: "claude ready session",
		content: `Welcome to Claude Code

● Completed the requested change
` + "```" + `
go test ./...
` + "```" + `
## Summary
❯
`,
		expectedAgent: true,
		expectedTool:  "claude",
		purpose:       "Detect idle or ready Claude Code sessions after completed work.",
	},
	{
		name: "claude approval prompt",
		content: `Claude Code

Bash(go test ./...)
● Needs permission
Would you like to run the following command?
❯ Yes, allow once
  No, and tell Claude what to do differently
`,
		expectedAgent: true,
		expectedTool:  "claude",
		purpose:       "Detect Claude Code sessions waiting for command permission.",
	},
	{
		name: "aider session",
		content: `Aider v0.80.0

✦ Editing files
Edit(internal/core/terminal/content/rules.go)
## Changes
` + "```diff" + `
+ new scorer
` + "```" + `
> 
`,
		expectedAgent: true,
		expectedTool:  "aider",
		purpose:       "Detect Aider editing sessions with tool-use and diff output.",
	},
	{
		name: "normal shell",
		content: `user@host:~/project$ ls
README.md go.mod internal
user@host:~/project$ grep claude README.md
user@host:~/project$ 
`,
		expectedAgent: false,
		purpose:       "Do not classify ordinary shell history as an agent session.",
	},
	{
		name: "fancy shell prompt",
		content: `~/project on main ✦
❯ git status
On branch main
nothing to commit, working tree clean
~/project on main ✦
❯ 
`,
		expectedAgent: false,
		purpose:       "Avoid false positives from glyph-heavy shell prompts.",
	},
	{
		name: "python repl",
		content: `Python 3.12.1 (main, Jan  1 2026, 00:00:00) [Clang]
Type "help", "copyright", "credits" or "license" for more information.
>>> print("hello")
hello
>>> 
`,
		expectedAgent: false,
		purpose:       "Avoid classifying Python REPL prompts as agents.",
	},
	{
		name: "node repl",
		content: `Welcome to Node.js v24.0.0.
Type ".help" for more information.
> const x = 1
undefined
> 
`,
		expectedAgent: false,
		purpose:       "Avoid classifying Node REPL prompts as agents.",
	},
	{
		name: "gdb session",
		content: `GNU gdb (GDB) 15.1
Reading symbols from ./app...
(gdb) break main
Breakpoint 1 at 0x1000
(gdb) run
`,
		expectedAgent: false,
		purpose:       "Avoid classifying debugger prompts as agents.",
	},
	{
		name: "build log",
		content: `=== RUN   TestScorer
--- PASS: TestScorer (0.00s)
PASS
ok  	github.com/colonyops/hive/internal/core/terminal/content	0.123s
--- PASS: TestOther (0.00s)
`,
		expectedAgent: false,
		purpose:       "Avoid classifying test and build output as an agent session.",
	},
	{
		name: "pager session",
		content: `HIVE(1)                         Manual page                         HIVE(1)

NAME
       hive - manage agent sessions
lines 1-50
(END)
`,
		expectedAgent: false,
		purpose:       "Avoid classifying pager content as an agent session.",
	},
	{
		name: "package prompt",
		content: `npm init
package name: (demo)
version: (1.0.0)
Proceed? [Y/n]
> 
`,
		expectedAgent: false,
		purpose:       "Avoid classifying interactive package prompts as agents.",
	},
}

func scorerFixtureContent(name string) string {
	for _, fixture := range scorerFixtures {
		if fixture.name == name {
			return fixture.content
		}
	}
	panic("unknown scorer fixture: " + name)
}
