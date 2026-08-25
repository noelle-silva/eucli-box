//! Safe-command whitelist ported from Codex
//! `command_safety/is_safe_command.rs`.

use crate::bash::parse_shell_lc_plain_commands;
use crate::safety::common::{executable_name_lookup_key, find_git_subcommand};

pub fn is_known_safe_command(command: &[String]) -> bool {
    let command: Vec<String> = command
        .iter()
        .map(|s| if s == "zsh" { "bash".to_string() } else { s.clone() })
        .collect();

    #[cfg(windows)]
    {
        if crate::safety::windows::is_safe_command_windows(&command) {
            return true;
        }
    }

    if is_safe_to_call_with_exec(&command) {
        return true;
    }

    if let Some(all_commands) = parse_shell_lc_plain_commands(&command) {
        if !all_commands.is_empty()
            && all_commands.iter().all(|cmd| is_safe_to_call_with_exec(cmd))
        {
            return true;
        }
    }
    false
}

/// Returns whether already-tokenized PowerShell words are read-only enough to
/// be auto-approved by the Windows safelist.
pub fn is_safe_powershell_words(command: &[String]) -> bool {
    #[cfg(windows)]
    {
        crate::safety::windows::is_safe_powershell_words(command)
    }
    #[cfg(not(windows))]
    {
        let _ = command;
        false
    }
}

fn is_safe_to_call_with_exec(command: &[String]) -> bool {
    let Some(cmd0) = command.first().map(String::as_str) else {
        return false;
    };

    match executable_name_lookup_key(cmd0).as_deref() {
        Some(cmd) if cfg!(target_os = "linux") && matches!(cmd, "numfmt" | "tac") => true,

        #[rustfmt::skip]
        Some(
            "cat" | "cd" | "cut" | "echo" | "expr" | "false" | "grep" | "head" |
            "id" | "ls" | "nl" | "paste" | "pwd" | "rev" | "seq" | "stat" |
            "tail" | "tr" | "true" | "uname" | "uniq" | "wc" | "which" |
            "whoami"
        ) => {
            true
        }

        Some("base64") => {
            const UNSAFE_BASE64_OPTIONS: &[&str] = &["-o", "--output"];
            !command.iter().skip(1).any(|arg| {
                UNSAFE_BASE64_OPTIONS.contains(&arg.as_str())
                    || arg.starts_with("--output=")
                    || (arg.starts_with("-o") && arg != "-o")
            })
        }

        Some("find") => {
            #[rustfmt::skip]
            const UNSAFE_FIND_OPTIONS: &[&str] = &[
                "-exec", "-execdir", "-ok", "-okdir",
                "-delete",
                "-fls", "-fprint", "-fprint0", "-fprintf",
            ];
            !command.iter().any(|arg| UNSAFE_FIND_OPTIONS.contains(&arg.as_str()))
        }

        Some("rg") => {
            const UNSAFE_RIPGREP_OPTIONS_WITH_ARGS: &[&str] = &["--pre", "--hostname-bin"];
            const UNSAFE_RIPGREP_OPTIONS_WITHOUT_ARGS: &[&str] = &["--search-zip", "-z"];

            !command.iter().any(|arg| {
                UNSAFE_RIPGREP_OPTIONS_WITHOUT_ARGS.contains(&arg.as_str())
                    || UNSAFE_RIPGREP_OPTIONS_WITH_ARGS
                        .iter()
                        .any(|&opt| arg == opt || arg.starts_with(&format!("{opt}=")))
            })
        }

        Some("git") => is_safe_git_command(command),

        Some("sed")
            if {
                command.len() <= 4
                    && command.get(1).map(String::as_str) == Some("-n")
                    && is_valid_sed_n_arg(command.get(2).map(String::as_str))
            } =>
        {
            true
        }

        _ => false,
    }
}

pub(crate) fn is_safe_git_command(command: &[String]) -> bool {
    let Some((subcommand_idx, subcommand)) =
        find_git_subcommand(command, &["status", "log", "diff", "show", "branch"])
    else {
        return false;
    };

    let global_args = &command[1..subcommand_idx];
    if git_has_unsafe_global_option(global_args) {
        return false;
    }

    let subcommand_args = &command[subcommand_idx + 1..];

    match subcommand {
        "status" | "log" | "diff" | "show" => git_subcommand_args_are_read_only(subcommand_args),
        "branch" => {
            git_subcommand_args_are_read_only(subcommand_args)
                && git_branch_is_read_only(subcommand_args)
        }
        _ => false,
    }
}

fn git_branch_is_read_only(branch_args: &[String]) -> bool {
    if branch_args.is_empty() {
        return true;
    }
    let mut saw_read_only_flag = false;
    for arg in branch_args.iter().map(String::as_str) {
        match arg {
            "--list" | "-l" | "--show-current" | "-a" | "--all" | "-r" | "--remotes" | "-v"
            | "-vv" | "--verbose" => {
                saw_read_only_flag = true;
            }
            _ if arg.starts_with("--format=") => {
                saw_read_only_flag = true;
            }
            _ => {
                return false;
            }
        }
    }
    saw_read_only_flag
}

#[derive(Clone, Copy)]
enum GitOptionPattern {
    Exact(&'static str),
    ShortWithInlineValue(&'static str),
    Prefix(&'static str),
}

const UNSAFE_GIT_GLOBAL_OPTIONS: &[GitOptionPattern] = &[
    GitOptionPattern::Exact("-C"),
    GitOptionPattern::ShortWithInlineValue("-C"),
    GitOptionPattern::Exact("-c"),
    GitOptionPattern::ShortWithInlineValue("-c"),
    GitOptionPattern::Exact("-p"),
    GitOptionPattern::Exact("--config-env"),
    GitOptionPattern::Prefix("--config-env="),
    GitOptionPattern::Exact("--exec-path"),
    GitOptionPattern::Prefix("--exec-path="),
    GitOptionPattern::Exact("--git-dir"),
    GitOptionPattern::Prefix("--git-dir="),
    GitOptionPattern::Exact("--namespace"),
    GitOptionPattern::Prefix("--namespace="),
    GitOptionPattern::Exact("--paginate"),
    GitOptionPattern::Exact("--super-prefix"),
    GitOptionPattern::Prefix("--super-prefix="),
    GitOptionPattern::Exact("--work-tree"),
    GitOptionPattern::Prefix("--work-tree="),
];

const UNSAFE_GIT_SUBCOMMAND_OPTIONS: &[GitOptionPattern] = &[
    GitOptionPattern::Exact("--output"),
    GitOptionPattern::Prefix("--output="),
    GitOptionPattern::Exact("--ext-diff"),
    GitOptionPattern::Exact("--textconv"),
    GitOptionPattern::Exact("--exec"),
    GitOptionPattern::Prefix("--exec="),
];

impl GitOptionPattern {
    fn matches(self, arg: &str) -> bool {
        match self {
            GitOptionPattern::Exact(option) => arg == option,
            GitOptionPattern::ShortWithInlineValue(option) => {
                arg.starts_with(option) && arg.len() > option.len()
            }
            GitOptionPattern::Prefix(prefix) => arg.starts_with(prefix),
        }
    }
}

fn git_matches_option_pattern(arg: &str, patterns: &[GitOptionPattern]) -> bool {
    patterns.iter().any(|pattern| pattern.matches(arg))
}

fn git_has_unsafe_global_option(global_args: &[String]) -> bool {
    global_args
        .iter()
        .map(String::as_str)
        .any(|arg| git_matches_option_pattern(arg, UNSAFE_GIT_GLOBAL_OPTIONS))
}

fn git_subcommand_args_are_read_only(args: &[String]) -> bool {
    !args
        .iter()
        .map(String::as_str)
        .any(|arg| git_matches_option_pattern(arg, UNSAFE_GIT_SUBCOMMAND_OPTIONS))
}

fn is_valid_sed_n_arg(arg: Option<&str>) -> bool {
    let s = match arg {
        Some(s) => s,
        None => return false,
    };
    let core = match s.strip_suffix('p') {
        Some(rest) => rest,
        None => return false,
    };
    let parts: Vec<&str> = core.split(',').collect();
    match parts.as_slice() {
        [num] => !num.is_empty() && num.chars().all(|c| c.is_ascii_digit()),
        [a, b] => {
            !a.is_empty()
                && !b.is_empty()
                && a.chars().all(|c| c.is_ascii_digit())
                && b.chars().all(|c| c.is_ascii_digit())
        }
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn vec_str(args: &[&str]) -> Vec<String> {
        args.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn known_safe_examples() {
        assert!(is_known_safe_command(&vec_str(&["ls"])));
        assert!(is_known_safe_command(&vec_str(&["git", "status"])));
        assert!(is_known_safe_command(&vec_str(&["git", "branch"])));
        assert!(is_known_safe_command(&vec_str(&[
            "git",
            "branch",
            "--show-current"
        ])));
        assert!(is_known_safe_command(&vec_str(&["base64"])));
        assert!(is_known_safe_command(&vec_str(&["sed", "-n", "1,5p", "file.txt"])));
        assert!(is_known_safe_command(&vec_str(&[
            "find", ".", "-name", "file.txt"
        ])));
    }

    #[test]
    fn git_branch_mutating_flags_are_not_safe() {
        assert!(!is_known_safe_command(&vec_str(&["git", "branch", "-d", "feature"])));
        assert!(!is_known_safe_command(&vec_str(&["git", "branch", "new-branch"])));
    }

    #[test]
    fn git_first_positional_is_the_subcommand() {
        assert!(!is_known_safe_command(&vec_str(&["git", "checkout", "status"])));
    }

    #[test]
    fn git_output_flags_are_not_safe() {
        assert!(!is_known_safe_command(&vec_str(&[
            "git",
            "log",
            "--output=/tmp/git-log-out-test",
            "-n",
            "1",
        ])));
    }

    #[test]
    fn git_global_pagination_flags_are_not_safe() {
        assert!(!is_known_safe_command(&vec_str(&["git", "--paginate", "log", "-1"])));
        assert!(!is_known_safe_command(&vec_str(&["git", "-p", "log", "-1"])));
    }

    #[test]
    fn git_subcommand_patch_flags_remain_safe() {
        assert!(is_known_safe_command(&vec_str(&["git", "log", "-p", "-1"])));
        assert!(is_known_safe_command(&vec_str(&["git", "diff", "-p"])));
    }

    #[test]
    fn git_global_override_flags_are_not_safe() {
        assert!(!is_known_safe_command(&vec_str(&["git", "-C", ".", "status"])));
        assert!(!is_known_safe_command(&vec_str(&["git", "-C.", "status"])));
        assert!(!is_known_safe_command(&vec_str(&[
            "git", "-c", "core.pager=cat", "log", "-n", "1"
        ])));
        assert!(!is_known_safe_command(&vec_str(&["git", "--git-dir", ".evil-git", "diff", "HEAD~1..HEAD"])));
        assert!(!is_known_safe_command(&vec_str(&["git", "--work-tree=.", "status"])));
    }

    #[test]
    fn cargo_check_is_not_safe() {
        assert!(!is_known_safe_command(&vec_str(&["cargo", "check"])));
    }

    #[test]
    fn zsh_lc_safe_command_sequence() {
        assert!(is_known_safe_command(&vec_str(&["zsh", "-lc", "ls"])));
    }

    #[test]
    fn unknown_or_partial() {
        assert!(!is_known_safe_command(&vec_str(&["foo"])));
        assert!(!is_known_safe_command(&vec_str(&["git", "fetch"])));
        assert!(!is_known_safe_command(&vec_str(&["sed", "-n", "xp", "file.txt"])));
        assert!(!is_known_safe_command(&vec_str(&[
            "find", ".", "-name", "file.txt", "-exec", "rm", "{}", ";"
        ])));
        assert!(!is_known_safe_command(&vec_str(&["find", ".", "-delete", "-name", "file.txt"])));
    }

    #[test]
    fn base64_output_options_are_unsafe() {
        assert!(!is_known_safe_command(&vec_str(&["base64", "-o", "out.bin"])));
        assert!(!is_known_safe_command(&vec_str(&["base64", "--output=out.bin"])));
        assert!(!is_known_safe_command(&vec_str(&["base64", "-ob64.txt"])));
    }

    #[test]
    fn ripgrep_rules() {
        assert!(is_known_safe_command(&vec_str(&["rg", "Cargo.toml", "-n"])));
        assert!(!is_known_safe_command(&vec_str(&["rg", "--pre", "pwned", "files"])));
        assert!(!is_known_safe_command(&vec_str(&["rg", "--search-zip", "files"])));
    }

    #[test]
    fn bash_lc_safe_examples() {
        assert!(is_known_safe_command(&vec_str(&["bash", "-lc", "ls"])));
        assert!(is_known_safe_command(&vec_str(&["bash", "-lc", "git status"])));
        assert!(is_known_safe_command(&vec_str(&["bash", "-lc", "sed -n 1,5p file.txt"])));
        assert!(is_known_safe_command(&vec_str(&["bash", "-lc", "find . -name file.txt"])));
    }

    #[test]
    fn bash_lc_safe_examples_with_operators() {
        assert!(is_known_safe_command(&vec_str(&[
            "bash", "-lc", "grep -R \"Cargo.toml\" -n || true"
        ])));
        assert!(is_known_safe_command(&vec_str(&["bash", "-lc", "ls && pwd"])));
        assert!(is_known_safe_command(&vec_str(&["bash", "-lc", "ls | wc -l"])));
    }

    #[test]
    fn bash_lc_unsafe_examples() {
        assert!(!is_known_safe_command(&vec_str(&["bash", "-lc", "git", "status"])));
        assert!(!is_known_safe_command(&vec_str(&["bash", "-lc", "'git status'"])));
        assert!(!is_known_safe_command(&vec_str(&[
            "bash", "-lc", "find . -name file.txt -delete"
        ])));
        assert!(!is_known_safe_command(&vec_str(&["bash", "-lc", "ls && rm -rf /"])));
        assert!(!is_known_safe_command(&vec_str(&["bash", "-lc", "(ls)"])));
        assert!(!is_known_safe_command(&vec_str(&["bash", "-lc", "ls > out.txt"])));
    }

    #[test]
    fn direct_powershell_words_use_windows_safelist() {
        let command = vec_str(&["Get-Content", "Cargo.toml"]);
        #[cfg(windows)]
        assert!(is_safe_powershell_words(&command));
        #[cfg(not(windows))]
        assert!(!is_safe_powershell_words(&command));
    }
}
