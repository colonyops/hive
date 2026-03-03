---
allowed-tools: Bash(gh issue view:*)
---

review the contents and discussion in github issue #$ARGUMENTS. After reviewing the contents wait another command from the user to better understand the issue and to provide a fix.

Here is an example command that you should use

```shell
gh issue view $ARGUMENTS --json author,body,comments,createdAt,labels,number,state,title,url
```
