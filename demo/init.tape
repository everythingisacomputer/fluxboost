# vhs demo/init.tape — records the init wizard scaffolding a platform repo.
Output demo-init.gif

Set FontSize 15
Set Width 1150
Set Height 680
Set TypingSpeed 40ms
Set Padding 20

Hide
Type "export PATH=$HOME/.fluxswitch/versions/2.9.3:/Users/ohaney/code/etc/fluxboost:$PATH"
Enter
Type "cd $(mktemp -d)"
Enter
Type "clear"
Enter
Show

Type "fluxboost init platform"
Sleep 500ms
Enter
Sleep 1.5s

# Base domain
Type "everythingisacomputer.io"
Sleep 300ms
Enter
Sleep 500ms

# Domain manager email
Type "ops@everythingisacomputer.io"
Sleep 300ms
Enter
Sleep 500ms

# Cloud platform: accept gcp (first option)
Sleep 700ms
Enter
Sleep 1s

# GCP project id
Type "everythingisacomputer"
Sleep 300ms
Enter
Sleep 1s

# Profile: standard
Sleep 500ms
Enter
Sleep 400ms
# Environments: dev (prefilled)
Enter
Sleep 400ms
# Git remote URL
Type "git@github.com:everythingisacomputer/platform"
Sleep 300ms
Enter
Sleep 1s

# Enable SOPS: yes (default)
Sleep 600ms
Enter
Sleep 400ms
# Set up tenants now: no (default) — see the tenant demo
Enter
Sleep 1s

# Recap → Scaffold
Sleep 1.5s
Enter
Sleep 2.5s

Type "ls platform"
Sleep 300ms
Enter
Sleep 4s
