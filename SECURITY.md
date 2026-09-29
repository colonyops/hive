# Security Policy

## Supported Versions

Security fixes ship in the next release. Earlier releases are not patched.

| Version          | Supported |
| ---------------- | --------- |
| Latest release   | Yes       |
| Earlier releases | No        |

## Reporting a Vulnerability

Report vulnerabilities privately through GitHub:

https://github.com/colonyops/hive/security/advisories/new

Draft advisories are visible only to the repository maintainers. Please do not
open a public issue, pull request, or discussion for a suspected vulnerability.
Hive runs commands and coding agents on developer machines, and a public report
can be used against every installed copy before a fix is available.

A report is easier to act on when it includes:

- The affected version, from `hive --version`.
- Your operating system and version.
- The preconditions an attacker needs, such as local access, a malicious
  repository, or a crafted message from another session.
- Steps to reproduce, a proof of concept, or the code path you believe is
  affected. Please say whether you reproduced the issue or found it by reading
  the code.

You can expect:

- An acknowledgement within 7 days.
- An assessment within 30 days: confirmed, not a vulnerability, or a request
  for more information.
- Credit in the advisory and the release notes, unless you prefer not to be
  named.

If you have not heard back after 14 days, open a public issue stating that you
are waiting on a response to a security report. Do not include any details of
the report in that issue.

## Scope

Hive is a command line tool that runs commands configured by the user and
starts coding agents in tmux. The areas of most interest are:

- **Configured commands.** Hive runs the rule, spawn, recycle, and user
  commands from `config.yaml`. It renders them from templates, so a session
  name, branch name, or path that reaches the shell unquoted is in scope.
- **Repositories.** Rules match on the remote URL, and hive copies files and
  runs setup commands in each clone. A repository that makes hive run something
  the user did not configure is in scope.
- **The data directory.** `hive.db` holds sessions, tasks, and the messages
  that sessions send to each other. Every session on the machine reads the same
  database.
- **tmux sessions.** Hive starts tmux sessions and reads pane output to detect
  agent status. Commands in those sessions inherit the user's environment.

Any way to cross one of these boundaries without the user's action is in scope.

### Out of scope

- Commands, rules, or plugins that the user configured to run on their own
  machine.
- Automated scanner output without a demonstrated path to exploitation.

## Hive Desktop

Hive Desktop has its own repository and its own policy. Report desktop
vulnerabilities at
https://github.com/hay-kot/hive-desktop/security/advisories/new.
