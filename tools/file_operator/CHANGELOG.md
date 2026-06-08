# Changelog

## Unreleased

- Added the unified `file_operator` local text-file tool.
- Added read, list, glob, grep, write, edit, and structured patch actions.
- Added text safety checks, size limits, stale-write protection, unique edit matching, and all-or-nothing patch planning.
- Removed the workspace-root allowlist so absolute paths can target files outside the current project when the operating system permits it.
- Added tool-local documentation for path behavior, safety boundaries, workflow, actions, and configuration.
