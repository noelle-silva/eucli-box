//! Impact analysis ported from two sources into one module:
//! - Claude Code `tools/BashTool/pathValidation.ts`: PATH_EXTRACTORS +
//!   filterOutFlags + checkDangerousRemovalPaths.
//! - Claude Code `utils/permissions/pathValidation.ts`: dangerous-removal
//!   path table.
//! - OpenCode `src/tool/shell.ts`: FILES/CMD_FILES/CWD command sets, path
//!   argument extraction with unquote/home/env expansion, containsPath
//!   (workdir boundary).
//! - OpenCode `src/permission/arity.ts`: BashArity prefix table.



use crate::readonly::contains_vulnerable_unc_path;

pub const CWD: &[&str] = &["cd", "chdir", "popd", "pushd", "push-location", "set-location"];
pub const FILES: &[&str] = &[
    "cd", "chdir", "popd", "pushd", "push-location", "set-location", "rm", "cp", "mv", "mkdir",
    "touch", "chmod", "chown", "cat", "get-content", "set-content", "add-content", "copy-item",
    "move-item", "remove-item", "new-item", "rename-item",
];
pub const CMD_FILES: &[&str] = &[
    "copy", "del", "dir", "erase", "md", "mkdir", "move", "rd", "ren", "rename", "rmdir", "type",
];

fn filter_out_flags(args: &[String]) -> Vec<String> {
    let mut result = Vec::new();
    let mut after_double_dash = false;
    for arg in args {
        if after_double_dash {
            result.push(arg.clone());
        } else if arg == "--" {
            after_double_dash = true;
        } else if !arg.starts_with('-') {
            result.push(arg.clone());
        }
    }
    result
}

fn parse_pattern_command(args: &[String], flags_with_args: &[&str], defaults: &[&str]) -> Vec<String> {
    let mut paths = Vec::new();
    let mut pattern_found = false;
    let mut after_double_dash = false;
    let mut i = 0;
    while i < args.len() {
        let arg = args[i].clone();
        if arg.is_empty() {
            i += 1;
            continue;
        }
        if !after_double_dash && arg == "--" {
            after_double_dash = true;
            i += 1;
            continue;
        }
        if !after_double_dash && arg.starts_with('-') {
            let flag = arg.split('=').next().unwrap_or("");
            if matches!(flag, "-e" | "--regexp" | "-f" | "--file") {
                pattern_found = true;
            }
            if flags_with_args.contains(&flag) && !arg.contains('=') {
                i += 1;
            }
            i += 1;
            continue;
        }
        if !pattern_found {
            pattern_found = true;
            i += 1;
            continue;
        }
        paths.push(arg);
        i += 1;
    }
    if !paths.is_empty() {
        paths
    } else {
        defaults.iter().map(|s| s.to_string()).collect()
    }
}

/// Claude's PATH_EXTRACTORS: extract filesystem paths from a command's args.
pub fn extract_paths(command: &str, args: &[String]) -> Vec<String> {
    match command {
        "cd" => {
            if args.is_empty() {
                vec![home_dir()]
            } else {
                vec![args.join(" ")]
            }
        }
        "ls" => {
            let paths = filter_out_flags(args);
            if paths.is_empty() {
                vec![".".into()]
            } else {
                paths
            }
        }
        "find" => {
            let mut paths = Vec::new();
            let path_flags = [
                "-newer", "-anewer", "-cnewer", "-mnewer", "-samefile", "-path", "-wholename",
                "-ilname", "-lname", "-ipath", "-iwholename",
            ];
            let newer_pattern = Regex::new(r"^-newer[acmBt][acmtB]$").unwrap();
            let mut found_non_global = false;
            let mut after_double_dash = false;
            let mut i = 0;
            while i < args.len() {
                let arg = args[i].clone();
                if arg.is_empty() {
                    i += 1;
                    continue;
                }
                if after_double_dash {
                    paths.push(arg);
                    i += 1;
                    continue;
                }
                if arg == "--" {
                    after_double_dash = true;
                    i += 1;
                    continue;
                }
                if arg.starts_with('-') {
                    if matches!(arg.as_str(), "-H" | "-L" | "-P") {
                        i += 1;
                        continue;
                    }
                    found_non_global = true;
                    if path_flags.contains(&arg.as_str()) || newer_pattern.is_match(&arg) {
                        if i + 1 < args.len() {
                            paths.push(args[i + 1].clone());
                            i += 1;
                        }
                    }
                    i += 1;
                    continue;
                }
                if !found_non_global {
                    paths.push(arg);
                }
                i += 1;
            }
            if paths.is_empty() {
                vec![".".into()]
            } else {
                paths
            }
        }
        "tr" => {
            let has_delete = args.iter().any(|a| {
                a == "-d"
                    || a == "--delete"
                    || (a.starts_with('-') && a.contains('d'))
            });
            let non_flags = filter_out_flags(args);
            let skip = if has_delete { 1 } else { 2 };
            if non_flags.len() > skip {
                non_flags[skip..].to_vec()
            } else {
                Vec::new()
            }
        }
        "grep" => {
            let flags = [
                "-e", "--regexp", "-f", "--file", "--exclude", "--include", "--exclude-dir",
                "--include-dir", "-m", "--max-count", "-A", "--after-context", "-B",
                "--before-context", "-C", "--context",
            ];
            let paths = parse_pattern_command(args, &flags, &[]);
            if paths.is_empty()
                && args.iter().any(|a| matches!(a.as_str(), "-r" | "-R" | "--recursive"))
            {
                vec![".".into()]
            } else {
                paths
            }
        }
        "rg" => {
            let flags = [
                "-e", "--regexp", "-f", "--file", "-t", "--type", "-T", "--type-not", "-g",
                "--glob", "-m", "--max-count", "--max-depth", "-r", "--replace", "-A",
                "--after-context", "-B", "--before-context", "-C", "--context",
            ];
            parse_pattern_command(args, &flags, &["."])
        }
        "sed" => {
            let mut paths = Vec::new();
            let mut skip_next = false;
            let mut script_found = false;
            let mut after_double_dash = false;
            let mut i = 0;
            while i < args.len() {
                if skip_next {
                    skip_next = false;
                    i += 1;
                    continue;
                }
                let arg = args[i].clone();
                if arg.is_empty() {
                    i += 1;
                    continue;
                }
                if !after_double_dash && arg == "--" {
                    after_double_dash = true;
                    i += 1;
                    continue;
                }
                if !after_double_dash && arg.starts_with('-') {
                    if matches!(arg.as_str(), "-f" | "--file") {
                        if i + 1 < args.len() {
                            paths.push(args[i + 1].clone());
                            skip_next = true;
                        }
                        script_found = true;
                    } else if matches!(arg.as_str(), "-e" | "--expression") {
                        skip_next = true;
                        script_found = true;
                    } else if arg.contains('e') || arg.contains('f') {
                        script_found = true;
                    }
                    i += 1;
                    continue;
                }
                if !script_found {
                    script_found = true;
                    i += 1;
                    continue;
                }
                paths.push(arg);
                i += 1;
            }
            paths
        }
        "jq" => {
            let flags_with_args = [
                "-e", "--expression", "-f", "--from-file", "--arg", "--argjson", "--slurpfile",
                "--rawfile", "--args", "--jsonargs", "-L", "--library-path", "--indent", "--tab",
            ];
            let mut paths = Vec::new();
            let mut filter_found = false;
            let mut after_double_dash = false;
            let mut i = 0;
            while i < args.len() {
                let arg = args[i].clone();
                if arg.is_empty() {
                    i += 1;
                    continue;
                }
                if !after_double_dash && arg == "--" {
                    after_double_dash = true;
                    i += 1;
                    continue;
                }
                if !after_double_dash && arg.starts_with('-') {
                    let flag = arg.split('=').next().unwrap_or("");
                    if matches!(flag, "-e" | "--expression") {
                        filter_found = true;
                    }
                    if flags_with_args.contains(&flag) && !arg.contains('=') {
                        i += 1;
                    }
                    i += 1;
                    continue;
                }
                if !filter_found {
                    filter_found = true;
                    i += 1;
                    continue;
                }
                paths.push(arg);
                i += 1;
            }
            paths
        }
        "git" => {
            if args.first().map(String::as_str) == Some("diff") {
                if args.contains(&"--no-index".to_string()) {
                    let file_paths = filter_out_flags(&args[1..]);
                    return file_paths.into_iter().take(2).collect();
                }
            }
            Vec::new()
        }
        "mkdir" | "touch" | "rm" | "rmdir" | "mv" | "cp" | "cat" | "head" | "tail" | "sort"
        | "uniq" | "wc" | "cut" | "paste" | "column" | "file" | "stat" | "diff" | "awk"
        | "strings" | "hexdump" | "od" | "base64" | "nl" | "sha256sum" | "sha1sum" | "md5sum" => {
            filter_out_flags(args)
        }
        _ => Vec::new(),
    }
}

use once_cell::sync::Lazy;
use regex::Regex;

/// Claude's isDangerousRemovalPath (utils/permissions/pathValidation.ts).
fn is_dangerous_removal_path(path: &str) -> bool {
    let forward_slashed = path.replace(['\\', '/'], "/").replace("*", "*");
    // Collapse runs: C:\\Windows -> C:/Windows.
    let mut collapsed = String::with_capacity(forward_slashed.len());
    let mut prev_slash = false;
    for c in forward_slashed.chars() {
        if c == '/' {
            if prev_slash {
                continue;
            }
            prev_slash = true;
        } else {
            prev_slash = false;
        }
        collapsed.push(c);
    }
    let forward_slashed = collapsed;

    if forward_slashed == "*" || forward_slashed.ends_with("/*") {
        return true;
    }
    let normalized_path = if forward_slashed == "/" {
        forward_slashed
    } else {
        forward_slashed.trim_end_matches('/').to_string()
    };
    if normalized_path == "/" {
        return true;
    }
    static DRIVE_ROOT: Lazy<Regex> =
        Lazy::new(|| Regex::new(r"^[A-Za-z]:/?$").unwrap());
    if DRIVE_ROOT.is_match(&normalized_path) {
        return true;
    }
    let home_dir = home_dir().replace(['\\', '/'], "/");
    if normalized_path == home_dir {
        return true;
    }
    // Direct children of root: /usr, /tmp, /etc (but not /usr/local).
    let parent = std::path::Path::new(&normalized_path)
        .parent()
        .map(|p| p.to_string_lossy().into_owned())
        .unwrap_or_default();
    if parent == "/" {
        return true;
    }
    static DRIVE_CHILD: Lazy<Regex> =
        Lazy::new(|| Regex::new(r"^[A-Za-z]:/[^/]+$").unwrap());
    if DRIVE_CHILD.is_match(&normalized_path) {
        return true;
    }
    false
}

fn home_dir() -> String {
    std::env::var("HOME")
        .or_else(|_| std::env::var("USERPROFILE"))
        .unwrap_or_default()
}

/// Resolve an extracted path against workdir (OpenCode resolvePath semantic).
/// Posix-style absolute paths (leading `/`) keep themselves: a bash command
/// `rm -rf /etc/passwd` must land on `/etc/passwd`, not on workdir-joined.
pub fn resolve_path(text: &str, workdir: &str) -> String {
    let text = expand_home(text);
    let joined = if is_absolute(&text) || text.starts_with('/') {
        text
    } else {
        std::path::Path::new(workdir)
            .join(&text)
            .to_string_lossy()
            .into_owned()
    };
    normalize(joined)
}

fn is_absolute(path: &str) -> bool {
    std::path::Path::new(path).is_absolute()
        || (path.len() >= 3
            && path.as_bytes()[1] == b':'
            && matches!(path.as_bytes()[0].to_ascii_uppercase(), b'A'..=b'Z'))
}

fn expand_home(text: &str) -> String {
    if text == "~" {
        home_dir()
    } else if text.starts_with("~/") || text.starts_with("~\\") {
        format!("{}{}", home_dir(), &text[1..])
    } else {
        text.to_string()
    }
}

fn normalize(path: String) -> String {
    // OpenCode FSUtil.normalizePath: backslashes to forward slashes on win.
    let mut out = path;
    if cfg!(windows) {
        out = out.replace('\\', "/");
    }
    if out.len() > 1 {
        while out.len() > 1 && out.ends_with('/') {
            out.pop();
        }
    }
    out
}

fn contains_path(filepath: &str, root: &str) -> bool {
    let filepath = filepath.trim_end_matches('/');
    if filepath == root {
        return true;
    }
    filepath.starts_with(&format!("{}/", root.trim_end_matches('/')))
}

/// OpenCode collect: for file-command arguments, extract paths and mark
/// those outside workdir. Returns (paths, outside).
pub fn collect_file_paths(
    command: String,
    args: &[String],
    ps_mode: bool,
    cmd_mode: bool,
) -> (Vec<String>, Vec<String>) {
    let is_file_cmd = FILES.contains(&command.as_str()) || (cmd_mode && CMD_FILES.contains(&command.as_str()));
    if !is_file_cmd {
        return (Vec::new(), Vec::new());
    }
    let mut raw_args: Vec<String> = args.to_vec();
    if ps_mode {
        // PS: filter switches; collect -Path/-LiteralPath values; skip others.
        let mut out: Vec<String> = Vec::new();
        let mut want = false;
        for item in &raw_args {
            if want {
                out.push(item.clone());
                want = false;
                continue;
            }
            let lower = item.to_ascii_lowercase();
            if matches!(lower.as_str(), "-confirm" | "-debug" | "-force" | "-nonewline" | "-recurse" | "-verbose" | "-whatif") {
                continue;
            }
            if matches!(lower.as_str(), "-destination" | "-literalpath" | "-path") {
                want = true;
            }
        }
        raw_args = out;
    } else {
        raw_args = raw_args
            .into_iter()
            .filter(|a| {
                if cmd_mode && a.starts_with('/') {
                    return false;
                }
                !a.starts_with('-')
            })
            .collect();
    }
    let mut items = Vec::new();
    let mut outside = Vec::new();
    for arg in raw_args {
        let expanded = expand_value(&arg);
        let prefixed = glob_prefix(&expanded);
        if let Some(prefix) = prefixed {
            if prefix.is_empty() {
                continue;
            }
            if is_dynamic(&prefix, ps_mode) {
                continue;
            }
            items.push(prefix.clone());
            outside.push(resolve_path(&prefix, ""));
        }
    }
    (items, outside)
}

fn expand_value(text: &str) -> String {
    let unquoted = if text.len() >= 2
        && ((text.starts_with('"') && text.ends_with('"'))
            || (text.starts_with('\'') && text.ends_with('\'')))
    {
        &text[1..text.len() - 1]
    } else {
        text
    };
    expand_home(unquoted)
}

fn glob_prefix(text: &str) -> Option<String> {
    match text.find(['?', '*', '[']) {
        Some(idx) if idx > 0 => Some(text[..idx].to_string()),
        Some(_) => None,
        None => Some(text.to_string()),
    }
}

fn is_dynamic(text: &str, ps: bool) -> bool {
    if text.starts_with('(') || text.starts_with("@(") {
        return true;
    }
    if text.contains("$(") || text.contains("${") || text.contains('`') {
        return true;
    }
    if ps {
        text.contains('$')
    } else {
        text.contains('$')
    }
}

/// OpenCode BashArity.prefix: longest matching prefix from the ARITY table.
pub fn bash_arity_prefix(tokens: &[String]) -> Vec<String> {
    for len in (1..=tokens.len()).rev() {
        let prefix = tokens[..len].join(" ");
        if let Some(arity) = arity_table().get(&prefix.as_str()) {
            return tokens[..*arity].to_vec();
        }
    }
    if tokens.is_empty() {
        Vec::new()
    } else {
        vec![tokens[0].clone()]
    }
}

fn arity_table() -> &'static std::collections::HashMap<&'static str, usize> {
    static TABLE: Lazy<std::collections::HashMap<&'static str, usize>> = Lazy::new(|| {
        let entries: &[(&str, usize)] = &[
            ("cat", 1), ("cd", 1), ("chmod", 1), ("chown", 1), ("cp", 1), ("echo", 1),
            ("env", 1), ("export", 1), ("grep", 1), ("kill", 1), ("killall", 1), ("ln", 1),
            ("ls", 1), ("mkdir", 1), ("mv", 1), ("ps", 1), ("pwd", 1), ("rm", 1),
            ("rmdir", 1), ("sleep", 1), ("source", 1), ("tail", 1), ("touch", 1),
            ("unset", 1), ("which", 1), ("aws", 3), ("az", 3), ("bazel", 2), ("brew", 2),
            ("bun", 2), ("bun run", 3), ("bun x", 3), ("cargo", 2), ("cargo add", 3),
            ("cargo run", 3), ("cdk", 2), ("cf", 2), ("cmake", 2), ("composer", 2),
            ("consul", 2), ("consul kv", 3), ("crictl", 2), ("deno", 2), ("deno task", 3),
            ("doctl", 3), ("docker", 2), ("docker builder", 3), ("docker compose", 3),
            ("docker container", 3), ("docker image", 3), ("docker network", 3),
            ("docker volume", 3), ("eksctl", 2), ("eksctl create", 3), ("firebase", 2),
            ("flyctl", 2), ("gcloud", 3), ("gh", 3), ("git", 2), ("git config", 3),
            ("git remote", 3), ("git stash", 3), ("go", 2), ("gradle", 2), ("helm", 2),
            ("heroku", 2), ("hugo", 2), ("ip", 2), ("ip addr", 3), ("ip link", 3),
            ("ip netns", 3), ("ip route", 3), ("kind", 2), ("kind create", 3),
            ("kubectl", 2), ("kubectl kustomize", 3), ("kubectl rollout", 3),
            ("kustomize", 2), ("make", 2), ("mc", 2), ("mc admin", 3), ("minikube", 2),
            ("mongosh", 2), ("mysql", 2), ("mvn", 2), ("ng", 2), ("npm", 2),
            ("npm exec", 3), ("npm init", 3), ("npm run", 3), ("npm view", 3),
            ("nvm", 2), ("nx", 2), ("openssl", 2), ("openssl req", 3), ("openssl x509", 3),
            ("pip", 2), ("pipenv", 2), ("pnpm", 2), ("pnpm dlx", 3), ("pnpm exec", 3),
            ("pnpm run", 3), ("poetry", 2), ("podman", 2), ("podman container", 3),
            ("podman image", 3), ("psql", 2), ("pulumi", 2), ("pulumi stack", 3),
            ("pyenv", 2), ("python", 2), ("rake", 2), ("rbenv", 2), ("redis-cli", 2),
            ("rustup", 2), ("serverless", 2), ("sfdx", 3), ("skaffold", 2), ("sls", 2),
            ("sst", 2), ("swift", 2), ("systemctl", 2), ("terraform", 2),
            ("terraform workspace", 3), ("tmux", 2), ("turbo", 2), ("ufw", 2),
            ("vault", 2), ("vault auth", 3), ("vault kv", 3), ("vercel", 2),
            ("volta", 2), ("wp", 2), ("yarn", 2), ("yarn dlx", 3), ("yarn run", 3),
        ];
        entries.iter().map(|(k, v)| (*k, *v)).collect()
    });
    &TABLE
}

/// Collect impact items for one command. `workdir` is the base to resolve
/// relative paths against. Returns (items, dangerous_removal_paths).
pub fn analyze_command_impact(
    argv: &[String],
    redirects: &[crate::protocol::InternalRedirect],
    workdir: &str,
) -> (Vec<crate::protocol::ImpactItem>, Vec<String>) {
    let mut items: Vec<crate::protocol::ImpactItem> = Vec::new();
    let mut dangerous: Vec<String> = Vec::new();

    if let Some(first) = argv.first().cloned() {
        let paths = extract_paths(&first, &argv[1..]);
        for p in paths {
            let p = p.trim_matches(|c| c == '"' || c == '\'');
            if p.is_empty() {
                continue;
            }
            let resolved = resolve_path(p, workdir);
            let outside = !contains_path(&resolved, &workdir.trim_end_matches('/'));
            items.push(crate::protocol::ImpactItem::new(resolved.clone(), "argument"));
            if let Some(last) = items.last_mut() {
                last.outside_workdir = outside;
                last.effects.push(if matches!(first.as_str(), "rm" | "rmdir" | "del" | "rd") {
                    "delete".into()
                } else if matches!(first.as_str(), "cd") {
                    "cd".into()
                } else if matches!(first.as_str(), "mkdir" | "touch" | "cp" | "mv" | "chmod" | "chown" | "set-content" | "add-content" | "copy-item" | "move-item" | "new-item" | "rename-item") {
                    "write".into()
                } else {
                    "read".into()
                });
            }
            if matches!(first.as_str(), "rm" | "rmdir") && is_dangerous_removal_path(&resolved) {
                dangerous.push(resolved);
            }
        }
    }

    for r in redirects {
        if r.target.is_empty() {
            continue;
        }
        // fd duplication (2>&1, <&0) is not a filesystem redirect.
        if matches!(r.op.as_str(), ">&" | "<&")
            && r.target.chars().all(|c| c.is_ascii_digit())
        {
            continue;
        }
        let mut item = crate::protocol::ImpactItem::new(r.target.clone(), "redirect");
        let resolved = resolve_path(&item.path, workdir);
        item.path = resolved;
        item.outside_workdir = !contains_path(&item.path, &workdir.trim_end_matches('/'));
        item.effects.push("write".into());
        items.push(item);
    }

    (items, dangerous)
}

pub fn contains_unc(path: &str) -> bool {
    contains_vulnerable_unc_path(path)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn v(items: &[&str]) -> Vec<String> {
        items.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn filter_out_flags_handles_double_dash() {
        assert_eq!(
            filter_out_flags(&v(&["--", "-/../.claude/settings.json"])),
            v(&["-/../.claude/settings.json"])
        );
    }

    #[test]
    fn ls_defaults_to_dot() {
        assert_eq!(extract_paths("ls", &v(&["-la"])), v(&["."]));
        assert_eq!(extract_paths("ls", &v(&["src"])), v(&["src"]));
    }

    #[test]
    fn grep_extracts_pattern_then_paths() {
        assert_eq!(
            extract_paths("grep", &v(&["-rn", "pattern", "src", "docs"])),
            v(&["src", "docs"])
        );
        assert_eq!(
            extract_paths("grep", &v(&["-rn", "-r", "pattern"])),
            v(&["."])
        );
    }

    #[test]
    fn rg_defaults_to_current_dir() {
        assert_eq!(extract_paths("rg", &v(&["pattern"])), v(&["."]));
    }

    #[test]
    fn sed_script_not_a_path() {
        assert_eq!(
            extract_paths("sed", &v(&["-n", "1,5p", "file.txt"])),
            v(&["file.txt"])
        );
    }

    #[test]
    fn git_diff_no_index_extracts_two_files() {
        assert_eq!(
            extract_paths(
                "git",
                &v(&["diff", "--no-index", "a.txt", "b.txt", "extra"])
            ),
            v(&["a.txt", "b.txt"])
        );
    }

    #[test]
    fn bash_arity_prefix_longest_match_wins() {
        let tokens = v(&["git", "checkout", "main"]);
        assert_eq!(bash_arity_prefix(&tokens), v(&["git", "checkout"]));
        let tokens2 = v(&["npm", "run", "dev"]);
        assert_eq!(bash_arity_prefix(&tokens2), v(&["npm", "run", "dev"]));
        let tokens3 = v(&["touch", "foo.txt"]);
        assert_eq!(bash_arity_prefix(&tokens3), v(&["touch"]));
    }

    #[test]
    fn resolve_path_joins_workdir() {
        assert_eq!(
            resolve_path("src/main.go", "E:/repo"),
            "E:/repo/src/main.go"
        );
    }
}
