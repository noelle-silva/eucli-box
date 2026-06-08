# file_operator

`file_operator` is the unified text-file tool for normal file work. It reads, lists, searches, writes, edits, and applies structured patches through one local tool entry.

## Path Behavior

- Relative paths resolve from the e-b host working directory.
- Absolute paths are allowed and may point outside the current project.
- Access is still limited by the operating system permissions of the running process.
- The tool no longer has a workspace-root allowlist. This is intentional so it can operate on files in other work areas when the user asks for them.

## Safety Boundaries

- Text files only. Binary-like content and null bytes are rejected.
- Large files are rejected by the configured size limit.
- `write` and `edit` support `expectedHash` so a stale read does not overwrite newer content.
- `edit` requires a unique match unless `replaceAll` is explicitly set.
- `apply_patch` plans the full change before writing, so a planning failure does not leave earlier partial changes.
- Patch delete and move operations keep the text-file boundary; they do not act as a general binary file remover.

## Recommended Workflow

1. Use `glob` or `grep` to find candidate files.
2. Use `read` to inspect the exact file or line window.
3. Use `edit`, `write`, or `apply_patch` for the smallest safe change.
4. Pass `expectedHash` when changing an existing file that was previously read.

## Actions

- `read`: read a file or list a directory with line or entry windows.
- `list`: list directory entries.
- `glob`: find files by glob pattern.
- `grep`: search text by regular expression.
- `write`: create or replace a bounded text file.
- `edit`: perform exact text replacement.
- `apply_patch`: apply a structured multi-file patch.

## Configuration

- `maxFileBytes`: maximum accepted text file size.
- `defaultReadLines`: default read/list window size.
- `maxReadLines`: maximum read/list window size.
- `maxLineChars`: maximum visible characters per returned line.
- `maxOutputChars`: maximum visible characters per tool call.
- `maxSearchResults`: maximum glob/grep matches per call.
