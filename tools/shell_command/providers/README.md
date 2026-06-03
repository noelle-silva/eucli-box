# shell_command Provider Assets

Provider binaries are not source code. Put bundled provider assets here before running the tool build command.

Expected Windows paths after build:

- `providers/git-bash/bin/bash.exe`
- `providers/powershell/pwsh.exe`
- `providers/nushell/nu.exe`

The tool does not fall back to host-installed shells when a bundled provider is missing.

Build-time asset roots:

- `-asset-root git-bash-root=<path-to-git-for-windows-root>` copies a root that contains `bin/bash.exe` into `providers/git-bash`.
- `-asset-root powershell-root=<path-to-powershell-root>` copies a root that contains `pwsh.exe` into `providers/powershell`.
- `-asset-root nushell-root=<path-to-nushell-root>` copies a root that contains `nu.exe` into `providers/nushell`.

Example:

```cmd
scripts\build-tools.cmd -tool shell_command -asset-root git-bash-root=D:\git\Git -asset-root powershell-root=E:\TOOOOOLSbox\powershell\7 -asset-root nushell-root=E:\TOOOOOLSbox\nushell\0.113.1
```
