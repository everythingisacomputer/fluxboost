# vhs demo/tenant.tape — records adding a tenant to an existing platform repo.
Output demo-tenant.gif

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
Type "printf 'everythingisacomputer.io\nops@everythingisacomputer.io\ngcp\neverythingisacomputer\nstandard\ndev\ngit@github.com:everythingisacomputer/platform\ny\nn\n' | fluxboost init platform"
Enter
Sleep 2s
Type "cd platform && clear"
Enter
Show

Type "fluxboost tenant add team-a"
Sleep 500ms
Enter
Sleep 2.5s

Type "find tenants -type f"
Sleep 300ms
Enter
Sleep 3s

Type "cat tenants/team-a/_base/rbac.yaml"
Sleep 300ms
Enter
Sleep 5s
