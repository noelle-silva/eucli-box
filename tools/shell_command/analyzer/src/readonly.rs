//! Read-only command verification ported from Claude Code:
//! - `utils/shell/readOnlyCommandValidation.ts`: flag tables (git/gh/docker/
//!   rg/pyright) + validateFlags generic walker + UNC path detection.
//! - `tools/BashTool/readOnlyValidation.ts`: COMMAND_ALLOWLIST entries for
//!   sed/xargs (and the flag-walker consumer semantics).
//! - `utils/shell/sedValidation.ts`: sed command allowlist.
//!
//! The verifier answers one question: "Is the whole command provably
//! read-only?" It is a per-command verifier: compounds are checked
//! command-by-command by the caller.

use once_cell::sync::Lazy;
use regex::Regex;
use std::collections::HashMap;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FlagArgType {
    None,
    Number,
    String,
    Char,
    Literal,
    Eof,
}

#[derive(Debug, Clone)]
pub struct CommandConfig {
    pub safe_flags: HashMap<String, FlagArgType>,
    pub respects_double_dash: bool,
    pub callback: Option<fn(raw_command: &str, args: &[String]) -> bool>,
}

impl CommandConfig {
    fn new(flags: &[(&'static str, FlagArgType)]) -> Self {
        CommandConfig {
            safe_flags: flags
                .iter()
                .map(|(k, v)| (k.to_string(), *v))
                .collect(),
            respects_double_dash: true,
            callback: None,
        }
    }

    fn with_callback(
        flags: &[(&'static str, FlagArgType)],
        callback: fn(&str, &[String]) -> bool,
    ) -> Self {
        CommandConfig {
            respects_double_dash: true,
            callback: Some(callback),
            ..Self::new(flags)
        }
    }

    fn no_double_dash(flags: &[(&'static str, FlagArgType)]) -> Self {
        CommandConfig {
            respects_double_dash: false,
            callback: None,
            ..Self::new(flags)
        }
    }
}

macro_rules! flags {
    ($($k:expr => $v:expr),* $(,)?) => {
        [$(($k, $v)),*]
    };
}

fn git_config_entries() -> HashMap<String, CommandConfig> {
    let mut m = HashMap::new();

    m.insert(
        "git diff".into(),
        CommandConfig::new(&flags![
            "--stat" => FlagArgType::None, "--numstat" => FlagArgType::None,
            "--shortstat" => FlagArgType::None, "--name-only" => FlagArgType::None,
            "--name-status" => FlagArgType::None, "--color" => FlagArgType::None,
            "--no-color" => FlagArgType::None, "--dirstat" => FlagArgType::None,
            "--summary" => FlagArgType::None, "--patch-with-stat" => FlagArgType::None,
            "--word-diff" => FlagArgType::None, "--word-diff-regex" => FlagArgType::String,
            "--color-words" => FlagArgType::None, "--no-renames" => FlagArgType::None,
            "--no-ext-diff" => FlagArgType::None, "--check" => FlagArgType::None,
            "--ws-error-highlight" => FlagArgType::String, "--full-index" => FlagArgType::None,
            "--binary" => FlagArgType::None, "--abbrev" => FlagArgType::Number,
            "--break-rewrites" => FlagArgType::None, "--find-renames" => FlagArgType::None,
            "--find-copies" => FlagArgType::None, "--find-copies-harder" => FlagArgType::None,
            "--irreversible-delete" => FlagArgType::None, "--diff-algorithm" => FlagArgType::String,
            "--histogram" => FlagArgType::None, "--patience" => FlagArgType::None,
            "--minimal" => FlagArgType::None, "--ignore-space-at-eol" => FlagArgType::None,
            "--ignore-space-change" => FlagArgType::None, "--ignore-all-space" => FlagArgType::None,
            "--ignore-blank-lines" => FlagArgType::None, "--inter-hunk-context" => FlagArgType::Number,
            "--function-context" => FlagArgType::None, "--exit-code" => FlagArgType::None,
            "--quiet" => FlagArgType::None, "--cached" => FlagArgType::None,
            "--staged" => FlagArgType::None, "--pickaxe-regex" => FlagArgType::None,
            "--pickaxe-all" => FlagArgType::None, "--no-index" => FlagArgType::None,
            "--relative" => FlagArgType::String, "--diff-filter" => FlagArgType::String,
            "-p" => FlagArgType::None, "-u" => FlagArgType::None, "-s" => FlagArgType::None,
            "-M" => FlagArgType::None, "-C" => FlagArgType::None, "-B" => FlagArgType::None,
            "-D" => FlagArgType::None, "-l" => FlagArgType::None, "-S" => FlagArgType::String,
            "-G" => FlagArgType::String, "-O" => FlagArgType::String, "-R" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git log".into(),
        CommandConfig::new(&flags![
            "--oneline" => FlagArgType::None, "--graph" => FlagArgType::None,
            "--decorate" => FlagArgType::None, "--no-decorate" => FlagArgType::None,
            "--date" => FlagArgType::String, "--relative-date" => FlagArgType::None,
            "--all" => FlagArgType::None, "--branches" => FlagArgType::None,
            "--tags" => FlagArgType::None, "--remotes" => FlagArgType::None,
            "--since" => FlagArgType::String, "--after" => FlagArgType::String,
            "--until" => FlagArgType::String, "--before" => FlagArgType::String,
            "--max-count" => FlagArgType::Number, "-n" => FlagArgType::Number,
            "--stat" => FlagArgType::None, "--numstat" => FlagArgType::None,
            "--shortstat" => FlagArgType::None, "--name-only" => FlagArgType::None,
            "--name-status" => FlagArgType::None, "--color" => FlagArgType::None,
            "--no-color" => FlagArgType::None, "--patch" => FlagArgType::None,
            "-p" => FlagArgType::None, "--no-patch" => FlagArgType::None,
            "--no-ext-diff" => FlagArgType::None, "-s" => FlagArgType::None,
            "--author" => FlagArgType::String, "--committer" => FlagArgType::String,
            "--grep" => FlagArgType::String, "--abbrev-commit" => FlagArgType::None,
            "--full-history" => FlagArgType::None, "--dense" => FlagArgType::None,
            "--sparse" => FlagArgType::None, "--simplify-merges" => FlagArgType::None,
            "--ancestry-path" => FlagArgType::None, "--source" => FlagArgType::None,
            "--first-parent" => FlagArgType::None, "--merges" => FlagArgType::None,
            "--no-merges" => FlagArgType::None, "--reverse" => FlagArgType::None,
            "--walk-reflogs" => FlagArgType::None, "--skip" => FlagArgType::Number,
            "--max-age" => FlagArgType::Number, "--min-age" => FlagArgType::Number,
            "--no-min-parents" => FlagArgType::None, "--no-max-parents" => FlagArgType::None,
            "--follow" => FlagArgType::None, "--no-walk" => FlagArgType::None,
            "--left-right" => FlagArgType::None, "--cherry-mark" => FlagArgType::None,
            "--cherry-pick" => FlagArgType::None, "--boundary" => FlagArgType::None,
            "--topo-order" => FlagArgType::None, "--date-order" => FlagArgType::None,
            "--author-date-order" => FlagArgType::None, "--pretty" => FlagArgType::String,
            "--format" => FlagArgType::String, "--diff-filter" => FlagArgType::String,
            "-S" => FlagArgType::String, "-G" => FlagArgType::String,
            "--pickaxe-regex" => FlagArgType::None, "--pickaxe-all" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git show".into(),
        CommandConfig::new(&flags![
            "--oneline" => FlagArgType::None, "--graph" => FlagArgType::None,
            "--decorate" => FlagArgType::None, "--date" => FlagArgType::String,
            "--stat" => FlagArgType::None, "--numstat" => FlagArgType::None,
            "--shortstat" => FlagArgType::None, "--name-only" => FlagArgType::None,
            "--name-status" => FlagArgType::None, "--color" => FlagArgType::None,
            "--no-color" => FlagArgType::None, "--patch" => FlagArgType::None,
            "-p" => FlagArgType::None, "--no-patch" => FlagArgType::None,
            "--no-ext-diff" => FlagArgType::None, "-s" => FlagArgType::None,
            "--abbrev-commit" => FlagArgType::None, "--word-diff" => FlagArgType::None,
            "--word-diff-regex" => FlagArgType::String, "--color-words" => FlagArgType::None,
            "--pretty" => FlagArgType::String, "--format" => FlagArgType::String,
            "--first-parent" => FlagArgType::None, "--raw" => FlagArgType::None,
            "--diff-filter" => FlagArgType::String, "-m" => FlagArgType::None,
            "--quiet" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git shortlog".into(),
        CommandConfig::new(&flags![
            "--all" => FlagArgType::None, "--since" => FlagArgType::String,
            "--until" => FlagArgType::String, "--before" => FlagArgType::String,
            "-s" => FlagArgType::None, "--summary" => FlagArgType::None,
            "-n" => FlagArgType::None, "--numbered" => FlagArgType::None,
            "-e" => FlagArgType::None, "--email" => FlagArgType::None,
            "-c" => FlagArgType::None, "--committer" => FlagArgType::None,
            "--group" => FlagArgType::String, "--format" => FlagArgType::String,
            "--no-merges" => FlagArgType::None, "--author" => FlagArgType::String,
        ]),
    );

    m.insert(
        "git reflog".into(),
        CommandConfig::with_callback(
            &flags![
                "--oneline" => FlagArgType::None, "--date" => FlagArgType::String,
                "--all" => FlagArgType::None, "--since" => FlagArgType::String,
                "--until" => FlagArgType::String, "--max-count" => FlagArgType::Number,
                "-n" => FlagArgType::Number, "--author" => FlagArgType::String,
                "--committer" => FlagArgType::String,
            ],
            |_, args: &[String]| {
                const DANGEROUS: &[&str] = &["expire", "delete", "exists"];
                for token in args {
                    if token.is_empty() || token.starts_with('-') {
                        continue;
                    }
                    if DANGEROUS.contains(&token.as_str()) {
                        return true;
                    }
                    return false;
                }
                false
            },
        ),
    );

    m.insert(
        "git stash list".into(),
        CommandConfig::new(&flags![
            "--oneline" => FlagArgType::None, "--date" => FlagArgType::String,
            "--all" => FlagArgType::None, "--since" => FlagArgType::String,
            "--until" => FlagArgType::String, "--max-count" => FlagArgType::Number,
            "-n" => FlagArgType::Number,
        ]),
    );

    m.insert(
        "git ls-remote".into(),
        CommandConfig::new(&flags![
            "--branches" => FlagArgType::None, "-b" => FlagArgType::None,
            "--tags" => FlagArgType::None, "-t" => FlagArgType::None,
            "--heads" => FlagArgType::None, "-h" => FlagArgType::None,
            "--refs" => FlagArgType::None, "--quiet" => FlagArgType::None,
            "-q" => FlagArgType::None, "--exit-code" => FlagArgType::None,
            "--get-url" => FlagArgType::None, "--symref" => FlagArgType::None,
            "--sort" => FlagArgType::String,
        ]),
    );

    m.insert(
        "git status".into(),
        CommandConfig::new(&flags![
            "--short" => FlagArgType::None, "-s" => FlagArgType::None,
            "--branch" => FlagArgType::None, "-b" => FlagArgType::None,
            "--porcelain" => FlagArgType::None, "--long" => FlagArgType::None,
            "--verbose" => FlagArgType::None, "-v" => FlagArgType::None,
            "--untracked-files" => FlagArgType::String, "-u" => FlagArgType::String,
            "--ignored" => FlagArgType::None, "--ignore-submodules" => FlagArgType::String,
            "--column" => FlagArgType::None, "--no-column" => FlagArgType::None,
            "--ahead-behind" => FlagArgType::None, "--no-ahead-behind" => FlagArgType::None,
            "--renames" => FlagArgType::None, "--no-renames" => FlagArgType::None,
            "--find-renames" => FlagArgType::String, "-M" => FlagArgType::String,
        ]),
    );

    m.insert(
        "git blame".into(),
        CommandConfig::new(&flags![
            "--color" => FlagArgType::None, "--no-color" => FlagArgType::None,
            "-L" => FlagArgType::String, "--porcelain" => FlagArgType::None,
            "-p" => FlagArgType::None, "--line-porcelain" => FlagArgType::None,
            "--incremental" => FlagArgType::None, "--root" => FlagArgType::None,
            "--show-stats" => FlagArgType::None, "--show-name" => FlagArgType::None,
            "--show-number" => FlagArgType::None, "-n" => FlagArgType::None,
            "--show-email" => FlagArgType::None, "-e" => FlagArgType::None,
            "-f" => FlagArgType::None, "--date" => FlagArgType::String,
            "-w" => FlagArgType::None, "--ignore-rev" => FlagArgType::String,
            "--ignore-revs-file" => FlagArgType::String, "-M" => FlagArgType::None,
            "-C" => FlagArgType::None, "--score-debug" => FlagArgType::None,
            "--abbrev" => FlagArgType::Number, "-s" => FlagArgType::None,
            "-l" => FlagArgType::None, "-t" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git ls-files".into(),
        CommandConfig::new(&flags![
            "--cached" => FlagArgType::None, "-c" => FlagArgType::None,
            "--deleted" => FlagArgType::None, "-d" => FlagArgType::None,
            "--modified" => FlagArgType::None, "-m" => FlagArgType::None,
            "--others" => FlagArgType::None, "-o" => FlagArgType::None,
            "--ignored" => FlagArgType::None, "-i" => FlagArgType::None,
            "--stage" => FlagArgType::None, "-s" => FlagArgType::None,
            "--killed" => FlagArgType::None, "-k" => FlagArgType::None,
            "--unmerged" => FlagArgType::None, "-u" => FlagArgType::None,
            "--directory" => FlagArgType::None, "--no-empty-directory" => FlagArgType::None,
            "--eol" => FlagArgType::None, "--full-name" => FlagArgType::None,
            "--abbrev" => FlagArgType::Number, "--debug" => FlagArgType::None,
            "-z" => FlagArgType::None, "-t" => FlagArgType::None,
            "-v" => FlagArgType::None, "-f" => FlagArgType::None,
            "--exclude" => FlagArgType::String, "-x" => FlagArgType::String,
            "--exclude-from" => FlagArgType::String, "-X" => FlagArgType::String,
            "--exclude-per-directory" => FlagArgType::String, "--exclude-standard" => FlagArgType::None,
            "--error-unmatch" => FlagArgType::None, "--recurse-submodules" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git config --get".into(),
        CommandConfig::new(&flags![
            "--local" => FlagArgType::None, "--global" => FlagArgType::None,
            "--system" => FlagArgType::None, "--worktree" => FlagArgType::None,
            "--default" => FlagArgType::String, "--type" => FlagArgType::String,
            "--bool" => FlagArgType::None, "--int" => FlagArgType::None,
            "--bool-or-int" => FlagArgType::None, "--path" => FlagArgType::None,
            "--expiry-date" => FlagArgType::None, "-z" => FlagArgType::None,
            "--null" => FlagArgType::None, "--name-only" => FlagArgType::None,
            "--show-origin" => FlagArgType::None, "--show-scope" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git remote show".into(),
        CommandConfig::with_callback(
            &flags!["-n" => FlagArgType::None],
            |_, args: &[String]| {
                let positional: Vec<&String> = args.iter().filter(|a| a.as_str() != "-n").collect();
                if positional.len() != 1 {
                    return true;
                }
                !Regex::new(r"^[a-zA-Z0-9_-]+$").unwrap().is_match(positional[0])
            },
        ),
    );

    m.insert(
        "git remote".into(),
        CommandConfig::with_callback(
            &flags!["-v" => FlagArgType::None, "--verbose" => FlagArgType::None],
            |_, args: &[String]| {
                args.iter()
                    .any(|a| a.as_str() != "-v" && a.as_str() != "--verbose")
            },
        ),
    );

    m.insert(
        "git merge-base".into(),
        CommandConfig::new(&flags![
            "--is-ancestor" => FlagArgType::None, "--fork-point" => FlagArgType::None,
            "--octopus" => FlagArgType::None, "--independent" => FlagArgType::None,
            "--all" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git rev-parse".into(),
        CommandConfig::new(&flags![
            "--verify" => FlagArgType::None, "--short" => FlagArgType::String,
            "--abbrev-ref" => FlagArgType::None, "--symbolic" => FlagArgType::None,
            "--symbolic-full-name" => FlagArgType::None, "--show-toplevel" => FlagArgType::None,
            "--show-cdup" => FlagArgType::None, "--show-prefix" => FlagArgType::None,
            "--git-dir" => FlagArgType::None, "--git-common-dir" => FlagArgType::None,
            "--absolute-git-dir" => FlagArgType::None, "--show-superproject-working-tree" => FlagArgType::None,
            "--is-inside-work-tree" => FlagArgType::None, "--is-inside-git-dir" => FlagArgType::None,
            "--is-bare-repository" => FlagArgType::None, "--is-shallow-repository" => FlagArgType::None,
            "--is-shallow-update" => FlagArgType::None, "--path-prefix" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git rev-list".into(),
        CommandConfig::new(&flags![
            "--all" => FlagArgType::None, "--tags" => FlagArgType::None,
            "--remotes" => FlagArgType::None, "--since" => FlagArgType::String,
            "--until" => FlagArgType::String, "--max-count" => FlagArgType::Number,
            "-n" => FlagArgType::Number, "--author" => FlagArgType::String,
            "--count" => FlagArgType::None, "--reverse" => FlagArgType::None,
            "--first-parent" => FlagArgType::None, "--ancestry-path" => FlagArgType::None,
            "--merges" => FlagArgType::None, "--no-merges" => FlagArgType::None,
            "--min-parents" => FlagArgType::Number, "--max-parents" => FlagArgType::Number,
            "--no-min-parents" => FlagArgType::None, "--no-max-parents" => FlagArgType::None,
            "--skip" => FlagArgType::Number, "--max-age" => FlagArgType::Number,
            "--min-age" => FlagArgType::Number, "--walk-reflogs" => FlagArgType::None,
            "--oneline" => FlagArgType::None, "--abbrev-commit" => FlagArgType::None,
            "--pretty" => FlagArgType::String, "--format" => FlagArgType::String,
            "--abbrev" => FlagArgType::Number, "--full-history" => FlagArgType::None,
            "--dense" => FlagArgType::None, "--sparse" => FlagArgType::None,
            "--source" => FlagArgType::None, "--graph" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git describe".into(),
        CommandConfig::new(&flags![
            "--tags" => FlagArgType::None, "--match" => FlagArgType::String,
            "--exclude" => FlagArgType::String, "--long" => FlagArgType::None,
            "--abbrev" => FlagArgType::Number, "--always" => FlagArgType::None,
            "--contains" => FlagArgType::None, "--first-match" => FlagArgType::None,
            "--exact-match" => FlagArgType::None, "--candidates" => FlagArgType::Number,
            "--dirty" => FlagArgType::None, "--broken" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git cat-file".into(),
        CommandConfig::new(&flags![
            "-t" => FlagArgType::None, "-s" => FlagArgType::None,
            "-p" => FlagArgType::None, "-e" => FlagArgType::None,
            "--batch-check" => FlagArgType::None, "--allow-undetermined-type" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git for-each-ref".into(),
        CommandConfig::new(&flags![
            "--format" => FlagArgType::String, "--sort" => FlagArgType::String,
            "--count" => FlagArgType::Number, "--contains" => FlagArgType::String,
            "--no-contains" => FlagArgType::String, "--merged" => FlagArgType::String,
            "--no-merged" => FlagArgType::String, "--points-at" => FlagArgType::String,
        ]),
    );

    m.insert(
        "git grep".into(),
        CommandConfig::new(&flags![
            "-e" => FlagArgType::String, "-E" => FlagArgType::None,
            "--extended-regexp" => FlagArgType::None, "-G" => FlagArgType::None,
            "--basic-regexp" => FlagArgType::None, "-F" => FlagArgType::None,
            "--fixed-strings" => FlagArgType::None, "-P" => FlagArgType::None,
            "--perl-regexp" => FlagArgType::None, "-i" => FlagArgType::None,
            "--ignore-case" => FlagArgType::None, "-v" => FlagArgType::None,
            "--invert-match" => FlagArgType::None, "-w" => FlagArgType::None,
            "--word-regexp" => FlagArgType::None, "-n" => FlagArgType::None,
            "--line-number" => FlagArgType::None, "-c" => FlagArgType::None,
            "--count" => FlagArgType::None, "-l" => FlagArgType::None,
            "--files-with-matches" => FlagArgType::None, "-L" => FlagArgType::None,
            "--files-without-match" => FlagArgType::None, "-h" => FlagArgType::None,
            "-H" => FlagArgType::None, "--heading" => FlagArgType::None,
            "--break" => FlagArgType::None, "--full-name" => FlagArgType::None,
            "--color" => FlagArgType::None, "--no-color" => FlagArgType::None,
            "-o" => FlagArgType::None, "--only-matching" => FlagArgType::None,
            "-A" => FlagArgType::Number, "--after-context" => FlagArgType::Number,
            "-B" => FlagArgType::Number, "--before-context" => FlagArgType::Number,
            "-C" => FlagArgType::Number, "--context" => FlagArgType::Number,
            "--and" => FlagArgType::None, "--or" => FlagArgType::None,
            "--not" => FlagArgType::None, "--max-depth" => FlagArgType::Number,
            "--untracked" => FlagArgType::None, "--no-index" => FlagArgType::None,
            "--recurse-submodules" => FlagArgType::None, "--cached" => FlagArgType::None,
            "--threads" => FlagArgType::Number, "-q" => FlagArgType::None,
            "--quiet" => FlagArgType::None,
        ]),
    );

    m.insert(
        "git stash show".into(),
        CommandConfig::new(&flags![
            "--stat" => FlagArgType::None, "--numstat" => FlagArgType::None,
            "--shortstat" => FlagArgType::None, "--name-only" => FlagArgType::None,
            "--name-status" => FlagArgType::None, "--color" => FlagArgType::None,
            "--no-color" => FlagArgType::None, "--patch" => FlagArgType::None,
            "-p" => FlagArgType::None, "--no-patch" => FlagArgType::None,
            "--no-ext-diff" => FlagArgType::None, "-s" => FlagArgType::None,
            "--word-diff" => FlagArgType::None, "--word-diff-regex" => FlagArgType::String,
            "--diff-filter" => FlagArgType::String, "--abbrev" => FlagArgType::Number,
        ]),
    );

    m.insert(
        "git worktree list".into(),
        CommandConfig::new(&flags![
            "--porcelain" => FlagArgType::None, "-v" => FlagArgType::None,
            "--verbose" => FlagArgType::None, "--expire" => FlagArgType::String,
        ]),
    );

    m.insert(
        "git tag".into(),
        CommandConfig::with_callback(
            &flags![
                "-l" => FlagArgType::None, "--list" => FlagArgType::None,
                "-n" => FlagArgType::Number, "--contains" => FlagArgType::String,
                "--no-contains" => FlagArgType::String, "--merged" => FlagArgType::String,
                "--no-merged" => FlagArgType::String, "--sort" => FlagArgType::String,
                "--format" => FlagArgType::String, "--points-at" => FlagArgType::String,
                "--column" => FlagArgType::None, "--no-column" => FlagArgType::None,
                "-i" => FlagArgType::None, "--ignore-case" => FlagArgType::None,
            ],
            |_, args: &[String]| {
                const FLAGS_WITH_ARGS: &[&str] = &[
                    "--contains", "--no-contains", "--merged", "--no-merged", "--points-at",
                    "--sort", "--format", "-n",
                ];
                let mut i = 0;
                let mut seen_list = false;
                let mut seen_dashdash = false;
                while i < args.len() {
                    let token = args[i].clone();
                    if token.is_empty() {
                        i += 1;
                        continue;
                    }
                    if token == "--" && !seen_dashdash {
                        seen_dashdash = true;
                        i += 1;
                        continue;
                    }
                    if !seen_dashdash && token.starts_with('-') {
                        if token == "--list" || token == "-l" {
                            seen_list = true;
                        } else if token.starts_with('-')
                            && !token.starts_with("--")
                            && token.len() > 2
                            && !token.contains('=')
                            && token[1..].contains('l')
                        {
                            seen_list = true;
                        }
                        if token.contains('=') {
                            i += 1;
                        } else if FLAGS_WITH_ARGS.contains(&token.as_str()) {
                            i += 2;
                        } else {
                            i += 1;
                        }
                    } else {
                        if !seen_list {
                            return true;
                        }
                        i += 1;
                    }
                }
                false
            },
        ),
    );

    m.insert(
        "git branch".into(),
        CommandConfig::with_callback(
            &flags![
                "-l" => FlagArgType::None, "--list" => FlagArgType::None,
                "-a" => FlagArgType::None, "--all" => FlagArgType::None,
                "-r" => FlagArgType::None, "--remotes" => FlagArgType::None,
                "-v" => FlagArgType::None, "-vv" => FlagArgType::None,
                "--verbose" => FlagArgType::None, "--color" => FlagArgType::None,
                "--no-color" => FlagArgType::None, "--column" => FlagArgType::None,
                "--no-column" => FlagArgType::None, "--abbrev" => FlagArgType::Number,
                "--no-abbrev" => FlagArgType::None, "--contains" => FlagArgType::String,
                "--no-contains" => FlagArgType::String, "--merged" => FlagArgType::None,
                "--no-merged" => FlagArgType::None, "--points-at" => FlagArgType::String,
                "--sort" => FlagArgType::String, "--show-current" => FlagArgType::None,
                "-i" => FlagArgType::None, "--ignore-case" => FlagArgType::None,
            ],
            |_, args: &[String]| {
                const FLAGS_WITH_ARGS: &[&str] =
                    &["--contains", "--no-contains", "--points-at", "--sort"];
                const FLAGS_WITH_OPTIONAL: &[&str] = &["--merged", "--no-merged"];
                let mut i = 0;
                let mut last_flag = String::new();
                let mut seen_list = false;
                let mut seen_dashdash = false;
                while i < args.len() {
                    let token = args[i].clone();
                    if token.is_empty() {
                        i += 1;
                        continue;
                    }
                    if token == "--" && !seen_dashdash {
                        seen_dashdash = true;
                        last_flag.clear();
                        i += 1;
                        continue;
                    }
                    if !seen_dashdash && token.starts_with('-') {
                        if token == "--list" || token == "-l" {
                            seen_list = true;
                        } else if token.starts_with('-')
                            && !token.starts_with("--")
                            && token.len() > 2
                            && !token.contains('=')
                            && token[1..].contains('l')
                        {
                            seen_list = true;
                        }
                        if token.contains('=') {
                            last_flag = token.split('=').next().unwrap_or("").to_string();
                            i += 1;
                        } else if FLAGS_WITH_ARGS.contains(&token.as_str()) {
                            last_flag = token.clone();
                            i += 2;
                        } else {
                            last_flag = token.clone();
                            i += 1;
                        }
                    } else {
                        let has_optional = FLAGS_WITH_OPTIONAL.contains(&last_flag.as_str());
                        if !seen_list && !has_optional {
                            return true;
                        }
                        i += 1;
                    }
                }
                false
            },
        ),
    );

    // git ls-remote URL guard: reject URL/ssh-style/2+ slash repos.
    m.entry("git ls-remote".into()).and_modify(|cfg| {
        cfg.safe_flags.insert("--".into(), FlagArgType::None);
        cfg.callback = Some(|_, args: &[String]| {
            for token in args {
                if !token.is_empty() && !token.starts_with('-') {
                    if token.contains("://") || token.contains('@') || token.contains('$') || token.matches('/').count() >= 2 {
                        return true;
                    }
                }
            }
            false
        });
    });

    m
}

fn gh_is_dangerous(_raw: &str, args: &[String]) -> bool {
    for token in args {
        if token.is_empty() {
            continue;
        }
        let mut value = token.as_str();
        if let Some(stripped) = token.strip_prefix('-') {
            let Some(eq_idx) = stripped.find('=') else {
                continue;
            };
            value = &stripped[eq_idx + 1..];
            if value.is_empty() {
                continue;
            }
        }
        if !value.contains('/') && !value.contains("://") && !value.contains('@') {
            continue;
        }
        if value.contains("://") || value.contains('@') {
            return true;
        }
        if value.matches('/').count() >= 2 {
            return true;
        }
    }
    false
}

fn gh_entries() -> HashMap<String, CommandConfig> {
    let mut m = HashMap::new();
    let entries: Vec<(
        &'static str,
        &'static [(&'static str, FlagArgType)],
    )> = vec![
        ("gh pr view", &[
            ("--json", FlagArgType::String), ("--comments", FlagArgType::None),
            ("--repo", FlagArgType::String), ("-R", FlagArgType::String),
        ]),
        ("gh pr list", &[
            ("--state", FlagArgType::String), ("-s", FlagArgType::String),
            ("--author", FlagArgType::String), ("--assignee", FlagArgType::String),
            ("--label", FlagArgType::String), ("--limit", FlagArgType::Number),
            ("-L", FlagArgType::Number), ("--base", FlagArgType::String),
            ("--head", FlagArgType::String), ("--search", FlagArgType::String),
            ("--json", FlagArgType::String), ("--draft", FlagArgType::None),
            ("--app", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh pr diff", &[
            ("--color", FlagArgType::String), ("--name-only", FlagArgType::None),
            ("--patch", FlagArgType::None), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh pr checks", &[
            ("--watch", FlagArgType::None), ("--required", FlagArgType::None),
            ("--fail-fast", FlagArgType::None), ("--json", FlagArgType::String),
            ("--interval", FlagArgType::Number), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh issue view", &[
            ("--json", FlagArgType::String), ("--comments", FlagArgType::None),
            ("--repo", FlagArgType::String), ("-R", FlagArgType::String),
        ]),
        ("gh issue list", &[
            ("--state", FlagArgType::String), ("-s", FlagArgType::String),
            ("--assignee", FlagArgType::String), ("--author", FlagArgType::String),
            ("--label", FlagArgType::String), ("--limit", FlagArgType::Number),
            ("-L", FlagArgType::Number), ("--milestone", FlagArgType::String),
            ("--search", FlagArgType::String), ("--json", FlagArgType::String),
            ("--app", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh repo view", &[("--json", FlagArgType::String)]),
        ("gh run list", &[
            ("--branch", FlagArgType::String), ("-b", FlagArgType::String),
            ("--status", FlagArgType::String), ("-s", FlagArgType::String),
            ("--workflow", FlagArgType::String), ("-w", FlagArgType::String),
            ("--limit", FlagArgType::Number), ("-L", FlagArgType::Number),
            ("--json", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String), ("--event", FlagArgType::String),
            ("-e", FlagArgType::String), ("--user", FlagArgType::String),
            ("-u", FlagArgType::String), ("--created", FlagArgType::String),
            ("--commit", FlagArgType::String), ("-c", FlagArgType::String),
        ]),
        ("gh run view", &[
            ("--log", FlagArgType::None), ("--log-failed", FlagArgType::None),
            ("--exit-status", FlagArgType::None), ("--verbose", FlagArgType::None),
            ("-v", FlagArgType::None), ("--json", FlagArgType::String),
            ("--repo", FlagArgType::String), ("-R", FlagArgType::String),
            ("--job", FlagArgType::String), ("-j", FlagArgType::String),
            ("--attempt", FlagArgType::Number), ("-a", FlagArgType::Number),
        ]),
        ("gh auth status", &[
            ("--active", FlagArgType::None), ("-a", FlagArgType::None),
            ("--hostname", FlagArgType::String), ("-h", FlagArgType::String),
            ("--json", FlagArgType::String),
        ]),
        ("gh pr status", &[
            ("--conflict-status", FlagArgType::None), ("-c", FlagArgType::None),
            ("--json", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh issue status", &[
            ("--json", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh release list", &[
            ("--exclude-drafts", FlagArgType::None), ("--exclude-pre-releases", FlagArgType::None),
            ("--json", FlagArgType::String), ("--limit", FlagArgType::Number),
            ("-L", FlagArgType::Number), ("--order", FlagArgType::String),
            ("-O", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh release view", &[
            ("--json", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh workflow list", &[
            ("--all", FlagArgType::None), ("-a", FlagArgType::None),
            ("--json", FlagArgType::String), ("--limit", FlagArgType::Number),
            ("-L", FlagArgType::Number), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh workflow view", &[
            ("--ref", FlagArgType::String), ("-r", FlagArgType::String),
            ("--yaml", FlagArgType::None), ("-y", FlagArgType::None),
            ("--repo", FlagArgType::String), ("-R", FlagArgType::String),
        ]),
        ("gh label list", &[
            ("--json", FlagArgType::String), ("--limit", FlagArgType::Number),
            ("-L", FlagArgType::Number), ("--order", FlagArgType::String),
            ("--search", FlagArgType::String), ("-S", FlagArgType::String),
            ("--sort", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String),
        ]),
        ("gh search repos", &[
            ("--archived", FlagArgType::None), ("--created", FlagArgType::String),
            ("--followers", FlagArgType::String), ("--forks", FlagArgType::String),
            ("--good-first-issues", FlagArgType::String), ("--help-wanted-issues", FlagArgType::String),
            ("--include-forks", FlagArgType::String), ("--json", FlagArgType::String),
            ("--language", FlagArgType::String), ("--license", FlagArgType::String),
            ("--limit", FlagArgType::Number), ("-L", FlagArgType::Number),
            ("--match", FlagArgType::String), ("--number-topics", FlagArgType::String),
            ("--order", FlagArgType::String), ("--owner", FlagArgType::String),
            ("--size", FlagArgType::String), ("--sort", FlagArgType::String),
            ("--stars", FlagArgType::String), ("--topic", FlagArgType::String),
            ("--updated", FlagArgType::String), ("--visibility", FlagArgType::String),
        ]),
        ("gh search issues", &[
            ("--app", FlagArgType::String), ("--assignee", FlagArgType::String),
            ("--author", FlagArgType::String), ("--closed", FlagArgType::String),
            ("--commenter", FlagArgType::String), ("--comments", FlagArgType::String),
            ("--created", FlagArgType::String), ("--include-prs", FlagArgType::None),
            ("--interactions", FlagArgType::String), ("--involves", FlagArgType::String),
            ("--json", FlagArgType::String), ("--label", FlagArgType::String),
            ("--language", FlagArgType::String), ("--limit", FlagArgType::Number),
            ("-L", FlagArgType::Number), ("--locked", FlagArgType::None),
            ("--match", FlagArgType::String), ("--mentions", FlagArgType::String),
            ("--milestone", FlagArgType::String), ("--no-assignee", FlagArgType::None),
            ("--no-label", FlagArgType::None), ("--no-milestone", FlagArgType::None),
            ("--no-project", FlagArgType::None), ("--order", FlagArgType::String),
            ("--owner", FlagArgType::String), ("--project", FlagArgType::String),
            ("--reactions", FlagArgType::String), ("--repo", FlagArgType::String),
            ("-R", FlagArgType::String), ("--sort", FlagArgType::String),
            ("--state", FlagArgType::String), ("--team-mentions", FlagArgType::String),
            ("--updated", FlagArgType::String), ("--visibility", FlagArgType::String),
        ]),
        ("gh search prs", &[
            ("--app", FlagArgType::String), ("--assignee", FlagArgType::String),
            ("--author", FlagArgType::String), ("--base", FlagArgType::String),
            ("-B", FlagArgType::String), ("--checks", FlagArgType::String),
            ("--closed", FlagArgType::String), ("--commenter", FlagArgType::String),
            ("--comments", FlagArgType::String), ("--created", FlagArgType::String),
            ("--draft", FlagArgType::None), ("--head", FlagArgType::String),
            ("-H", FlagArgType::String), ("--interactions", FlagArgType::String),
            ("--involves", FlagArgType::String), ("--json", FlagArgType::String),
            ("--label", FlagArgType::String), ("--language", FlagArgType::String),
            ("--limit", FlagArgType::Number), ("-L", FlagArgType::Number),
            ("--locked", FlagArgType::None), ("--match", FlagArgType::String),
            ("--mentions", FlagArgType::String), ("--merged", FlagArgType::None),
            ("--merged-at", FlagArgType::String), ("--milestone", FlagArgType::String),
            ("--no-assignee", FlagArgType::None), ("--no-label", FlagArgType::None),
            ("--no-milestone", FlagArgType::None), ("--no-project", FlagArgType::None),
            ("--order", FlagArgType::String), ("--owner", FlagArgType::String),
            ("--project", FlagArgType::String), ("--reactions", FlagArgType::String),
            ("--repo", FlagArgType::String), ("-R", FlagArgType::String),
            ("--review", FlagArgType::String), ("--review-requested", FlagArgType::String),
            ("--reviewed-by", FlagArgType::String), ("--sort", FlagArgType::String),
            ("--state", FlagArgType::String), ("--team-mentions", FlagArgType::String),
            ("--updated", FlagArgType::String), ("--visibility", FlagArgType::String),
        ]),
        ("gh search commits", &[
            ("--author", FlagArgType::String), ("--author-date", FlagArgType::String),
            ("--author-email", FlagArgType::String), ("--author-name", FlagArgType::String),
            ("--committer", FlagArgType::String), ("--committer-date", FlagArgType::String),
            ("--committer-email", FlagArgType::String), ("--committer-name", FlagArgType::String),
            ("--hash", FlagArgType::String), ("--json", FlagArgType::String),
            ("--limit", FlagArgType::Number), ("-L", FlagArgType::Number),
            ("--merge", FlagArgType::None), ("--order", FlagArgType::String),
            ("--owner", FlagArgType::String), ("--parent", FlagArgType::String),
            ("--repo", FlagArgType::String), ("-R", FlagArgType::String),
            ("--sort", FlagArgType::String), ("--tree", FlagArgType::String),
            ("--visibility", FlagArgType::String),
        ]),
        ("gh search code", &[
            ("--extension", FlagArgType::String), ("--filename", FlagArgType::String),
            ("--json", FlagArgType::String), ("--language", FlagArgType::String),
            ("--limit", FlagArgType::Number), ("-L", FlagArgType::Number),
            ("--match", FlagArgType::String), ("--owner", FlagArgType::String),
            ("--repo", FlagArgType::String), ("-R", FlagArgType::String),
            ("--size", FlagArgType::String),
        ]),
    ];

    for (key, flags) in entries {
        let mut cfg = CommandConfig::new(flags as &[(&'static str, FlagArgType)]);
        cfg.callback = Some(gh_is_dangerous as fn(&str, &[String]) -> bool);
        m.insert(key.to_string(), cfg);
    }
    m
}

fn docker_entries() -> HashMap<String, CommandConfig> {
    let mut m = HashMap::new();
    m.insert(
        "docker logs".into(),
        CommandConfig::new(&flags![
            "--follow" => FlagArgType::None, "-f" => FlagArgType::None,
            "--tail" => FlagArgType::String, "-n" => FlagArgType::String,
            "--timestamps" => FlagArgType::None, "-t" => FlagArgType::None,
            "--since" => FlagArgType::String, "--until" => FlagArgType::String,
            "--details" => FlagArgType::None,
        ]),
    );
    m.insert(
        "docker inspect".into(),
        CommandConfig::new(&flags![
            "--format" => FlagArgType::String, "-f" => FlagArgType::String,
            "--type" => FlagArgType::String, "--size" => FlagArgType::None,
            "-s" => FlagArgType::None,
        ]),
    );
    m
}

fn rg_entries() -> HashMap<String, CommandConfig> {
    let mut m = HashMap::new();
    m.insert(
        "rg".into(),
        CommandConfig::new(&flags![
            "-e" => FlagArgType::String, "--regexp" => FlagArgType::String,
            "-f" => FlagArgType::String, "-i" => FlagArgType::None,
            "--ignore-case" => FlagArgType::None, "-S" => FlagArgType::None,
            "--smart-case" => FlagArgType::None, "-F" => FlagArgType::None,
            "--fixed-strings" => FlagArgType::None, "-w" => FlagArgType::None,
            "--word-regexp" => FlagArgType::None, "-v" => FlagArgType::None,
            "--invert-match" => FlagArgType::None, "-c" => FlagArgType::None,
            "--count" => FlagArgType::None, "-l" => FlagArgType::None,
            "--files-with-matches" => FlagArgType::None, "--files-without-match" => FlagArgType::None,
            "-n" => FlagArgType::None, "--line-number" => FlagArgType::None,
            "-o" => FlagArgType::None, "--only-matching" => FlagArgType::None,
            "-A" => FlagArgType::Number, "--after-context" => FlagArgType::Number,
            "-B" => FlagArgType::Number, "--before-context" => FlagArgType::Number,
            "-C" => FlagArgType::Number, "--context" => FlagArgType::Number,
            "-H" => FlagArgType::None, "-h" => FlagArgType::None,
            "--heading" => FlagArgType::None, "--no-heading" => FlagArgType::None,
            "-q" => FlagArgType::None, "--quiet" => FlagArgType::None,
            "--column" => FlagArgType::None, "-g" => FlagArgType::String,
            "--glob" => FlagArgType::String, "-t" => FlagArgType::String,
            "--type" => FlagArgType::String, "-T" => FlagArgType::String,
            "--type-not" => FlagArgType::String, "--type-list" => FlagArgType::None,
            "--hidden" => FlagArgType::None, "--no-ignore" => FlagArgType::None,
            "-u" => FlagArgType::None, "-m" => FlagArgType::Number,
            "--max-count" => FlagArgType::Number, "-d" => FlagArgType::Number,
            "--max-depth" => FlagArgType::Number, "-a" => FlagArgType::None,
            "--text" => FlagArgType::None, "-z" => FlagArgType::None,
            "-L" => FlagArgType::None, "--follow" => FlagArgType::None,
            "--color" => FlagArgType::String, "--json" => FlagArgType::None,
            "--stats" => FlagArgType::None, "--help" => FlagArgType::None,
            "--version" => FlagArgType::None, "--debug" => FlagArgType::None,
            "--" => FlagArgType::None,
        ]),
    );
    m
}

fn pyright_entries() -> HashMap<String, CommandConfig> {
    let mut m = HashMap::new();
    let mut cfg = CommandConfig::no_double_dash(&flags![
        "--outputjson" => FlagArgType::None, "--project" => FlagArgType::String,
        "-p" => FlagArgType::String, "--pythonversion" => FlagArgType::String,
        "--pythonplatform" => FlagArgType::String, "--typeshedpath" => FlagArgType::String,
        "--venvpath" => FlagArgType::String, "--level" => FlagArgType::String,
        "--stats" => FlagArgType::None, "--verbose" => FlagArgType::None,
        "--version" => FlagArgType::None, "--dependencies" => FlagArgType::None,
        "--warnings" => FlagArgType::None,
    ]);
    cfg.callback = Some(|_, args: &[String]| {
        args.iter().any(|t| t == "--watch" || t == "-w")
    });
    m.insert("pyright".into(), cfg);
    m
}

fn fd_safe_flags() -> Vec<(&'static str, FlagArgType)> {
    vec![
        ("-h", FlagArgType::None), ("--help", FlagArgType::None),
        ("-V", FlagArgType::None), ("--version", FlagArgType::None),
        ("-H", FlagArgType::None), ("--hidden", FlagArgType::None),
        ("-I", FlagArgType::None), ("--no-ignore", FlagArgType::None),
        ("--no-ignore-vcs", FlagArgType::None), ("--no-ignore-parent", FlagArgType::None),
        ("-s", FlagArgType::None), ("--case-sensitive", FlagArgType::None),
        ("-i", FlagArgType::None), ("--ignore-case", FlagArgType::None),
        ("-g", FlagArgType::None), ("--glob", FlagArgType::None),
        ("--regex", FlagArgType::None), ("-F", FlagArgType::None),
        ("--fixed-strings", FlagArgType::None), ("-a", FlagArgType::None),
        ("--absolute-path", FlagArgType::None), ("-L", FlagArgType::None),
        ("--follow", FlagArgType::None), ("-p", FlagArgType::None),
        ("--full-path", FlagArgType::None), ("-0", FlagArgType::None),
        ("--print0", FlagArgType::None), ("-d", FlagArgType::Number),
        ("--max-depth", FlagArgType::Number), ("--min-depth", FlagArgType::Number),
        ("--exact-depth", FlagArgType::Number), ("-t", FlagArgType::String),
        ("--type", FlagArgType::String), ("-e", FlagArgType::String),
        ("--extension", FlagArgType::String), ("-S", FlagArgType::String),
        ("--size", FlagArgType::String), ("--changed-within", FlagArgType::String),
        ("--changed-before", FlagArgType::String), ("-o", FlagArgType::String),
        ("--owner", FlagArgType::String), ("-E", FlagArgType::String),
        ("--exclude", FlagArgType::String), ("--ignore-file", FlagArgType::String),
        ("-c", FlagArgType::String), ("--color", FlagArgType::String),
        ("-j", FlagArgType::Number), ("--threads", FlagArgType::Number),
        ("--max-buffer-time", FlagArgType::String), ("--max-results", FlagArgType::Number),
        ("-1", FlagArgType::None), ("-q", FlagArgType::None),
        ("--quiet", FlagArgType::None), ("--show-errors", FlagArgType::None),
        ("--strip-cwd-prefix", FlagArgType::None), ("--one-file-system", FlagArgType::None),
        ("--prune", FlagArgType::None), ("--search-path", FlagArgType::String),
        ("--base-directory", FlagArgType::String), ("--path-separator", FlagArgType::String),
        ("--batch-size", FlagArgType::Number), ("--no-require-git", FlagArgType::None),
        ("--hyperlink", FlagArgType::String), ("--and", FlagArgType::String),
        ("--format", FlagArgType::String),
    ]
}

fn sed_is_dangerous(raw: &str, _args: &[String]) -> bool {
    !sed_command_allowed(raw)
}

pub fn is_command_safe_via_flag_parsing(command: &str) -> bool {
    // Only single-command strings reach here (callers split compounds).
    let tokens = match shlex::split(command) {
        Some(t) if !t.is_empty() => t,
        _ => return false,
    };

    let mut allowlist: HashMap<String, CommandConfig> = git_config_entries();
    allowlist.extend(gh_entries());
    allowlist.extend(docker_entries());
    allowlist.extend(rg_entries());
    allowlist.extend(pyright_entries());
    allowlist.insert(
        "xargs".into(),
        CommandConfig::new(&flags![
            "-I" => FlagArgType::Literal, "-n" => FlagArgType::Number,
            "-P" => FlagArgType::Number, "-L" => FlagArgType::Number,
            "-s" => FlagArgType::Number, "-E" => FlagArgType::Eof,
            "-0" => FlagArgType::None, "-t" => FlagArgType::None,
            "-r" => FlagArgType::None, "-x" => FlagArgType::None,
            "-d" => FlagArgType::Char,
        ]),
    );
    allowlist.insert(
        "sed".into(),
        CommandConfig::with_callback(
            &flags![
                "--expression" => FlagArgType::String, "-e" => FlagArgType::String,
                "--quiet" => FlagArgType::None, "--silent" => FlagArgType::None,
                "-n" => FlagArgType::None, "--regexp-extended" => FlagArgType::None,
                "-r" => FlagArgType::None, "--posix" => FlagArgType::None,
                "-E" => FlagArgType::None, "--line-length" => FlagArgType::Number,
                "-l" => FlagArgType::Number, "--zero-terminated" => FlagArgType::None,
                "-z" => FlagArgType::None, "--separate" => FlagArgType::None,
                "-s" => FlagArgType::None, "--unbuffered" => FlagArgType::None,
                "-u" => FlagArgType::None, "--debug" => FlagArgType::None,
                "--help" => FlagArgType::None, "--version" => FlagArgType::None,
            ],
            sed_is_dangerous,
        ),
    );
    allowlist.insert(
        "sort".into(),
        CommandConfig::new(&flags![
            "--ignore-leading-blanks" => FlagArgType::None, "-b" => FlagArgType::None,
            "--dictionary-order" => FlagArgType::None, "-d" => FlagArgType::None,
            "--ignore-case" => FlagArgType::None, "-f" => FlagArgType::None,
            "--general-numeric-sort" => FlagArgType::None, "-g" => FlagArgType::None,
            "--human-numeric-sort" => FlagArgType::None, "-h" => FlagArgType::None,
            "--ignore-nonprinting" => FlagArgType::None, "-i" => FlagArgType::None,
            "--month-sort" => FlagArgType::None, "-M" => FlagArgType::None,
            "--numeric-sort" => FlagArgType::None, "-n" => FlagArgType::None,
            "--random-sort" => FlagArgType::None, "-R" => FlagArgType::None,
            "--reverse" => FlagArgType::None, "-r" => FlagArgType::None,
            "--sort" => FlagArgType::String, "--stable" => FlagArgType::None,
            "-s" => FlagArgType::None, "--unique" => FlagArgType::None,
            "-u" => FlagArgType::None, "--version-sort" => FlagArgType::None,
            "-V" => FlagArgType::None, "--zero-terminated" => FlagArgType::None,
            "-z" => FlagArgType::None, "--key" => FlagArgType::String,
            "-k" => FlagArgType::String, "--field-separator" => FlagArgType::String,
            "-t" => FlagArgType::String, "--check" => FlagArgType::None,
            "-c" => FlagArgType::None, "--check-char-order" => FlagArgType::None,
            "-C" => FlagArgType::None, "--merge" => FlagArgType::None,
            "-m" => FlagArgType::None, "--buffer-size" => FlagArgType::String,
            "-S" => FlagArgType::String, "--parallel" => FlagArgType::Number,
            "--batch-size" => FlagArgType::Number, "--help" => FlagArgType::None,
            "--version" => FlagArgType::None,
        ]),
    );
    allowlist.insert(
        "date".into(),
        CommandConfig::with_callback(
            &flags![
                "-d" => FlagArgType::String, "--date" => FlagArgType::String,
                "-r" => FlagArgType::String, "--reference" => FlagArgType::String,
                "-u" => FlagArgType::None, "--utc" => FlagArgType::None,
                "--universal" => FlagArgType::None, "-I" => FlagArgType::None,
                "--iso-8601" => FlagArgType::String, "-R" => FlagArgType::None,
                "--rfc-email" => FlagArgType::None, "--rfc-3339" => FlagArgType::String,
                "--debug" => FlagArgType::None, "--help" => FlagArgType::None,
                "--version" => FlagArgType::None,
            ],
            |_, args: &[String]| {
                let flags_with_args = [
                    "-d", "--date", "-r", "--reference", "--iso-8601", "--rfc-3339",
                ];
                let mut i = 0;
                while i < args.len() {
                    let token = args[i].clone();
                    if token.starts_with("--") && token.contains('=') {
                        i += 1;
                    } else if token.starts_with('-') {
                        if flags_with_args.contains(&token.as_str()) {
                            i += 2;
                        } else {
                            i += 1;
                        }
                    } else {
                        if !token.starts_with('+') {
                            return true;
                        }
                        i += 1;
                    }
                }
                false
            },
        ),
    );
    allowlist.insert(
        "hostname".into(),
        CommandConfig::with_callback(
            &flags![
                "-f" => FlagArgType::None, "--fqdn" => FlagArgType::None,
                "--long" => FlagArgType::None, "-s" => FlagArgType::None,
                "--short" => FlagArgType::None, "-i" => FlagArgType::None,
                "--ip-address" => FlagArgType::None, "-I" => FlagArgType::None,
                "--all-ip-addresses" => FlagArgType::None, "-a" => FlagArgType::None,
                "--alias" => FlagArgType::None, "-d" => FlagArgType::None,
                "--domain" => FlagArgType::None, "-A" => FlagArgType::None,
                "--all-fqdns" => FlagArgType::None, "-v" => FlagArgType::None,
                "--verbose" => FlagArgType::None, "-h" => FlagArgType::None,
                "--help" => FlagArgType::None, "-V" => FlagArgType::None,
                "--version" => FlagArgType::None,
            ],
            |_, args: &[String]| {
                // Claude semantics: any positional argument sets the hostname.
                args.iter().any(|a| !a.starts_with('-'))
            },
        ),
    );
    allowlist.insert(
        "file".into(),
        CommandConfig::new(&flags![
            "--brief" => FlagArgType::None, "-b" => FlagArgType::None,
            "--mime" => FlagArgType::None, "-i" => FlagArgType::None,
            "--mime-type" => FlagArgType::None, "--mime-encoding" => FlagArgType::None,
            "--apple" => FlagArgType::None, "--check-encoding" => FlagArgType::None,
            "-c" => FlagArgType::None, "--exclude" => FlagArgType::String,
            "--exclude-quiet" => FlagArgType::String, "--print0" => FlagArgType::None,
            "-0" => FlagArgType::None, "-f" => FlagArgType::String,
            "-F" => FlagArgType::String, "--separator" => FlagArgType::String,
            "--help" => FlagArgType::None, "--version" => FlagArgType::None,
            "-v" => FlagArgType::None, "--no-dereference" => FlagArgType::None,
            "-h" => FlagArgType::None, "--dereference" => FlagArgType::None,
            "-L" => FlagArgType::None, "--magic-file" => FlagArgType::String,
            "-m" => FlagArgType::String, "--keep-going" => FlagArgType::None,
            "-k" => FlagArgType::None, "--list" => FlagArgType::None,
            "-l" => FlagArgType::None, "--no-buffer" => FlagArgType::None,
            "-n" => FlagArgType::None, "--preserve-date" => FlagArgType::None,
            "-p" => FlagArgType::None, "--raw" => FlagArgType::None,
            "-r" => FlagArgType::None, "-s" => FlagArgType::None,
            "--special-files" => FlagArgType::None, "--uncompress" => FlagArgType::None,
            "-z" => FlagArgType::None,
        ]),
    );
    allowlist.insert(
        "fd".into(),
        CommandConfig::new(&fd_safe_flags()),
    );
    allowlist.insert(
        "fdfind".into(),
        CommandConfig::new(&fd_safe_flags()),
    );

    const SAFE_TARGET_COMMANDS_FOR_XARGS: &[&str] = &["echo", "printf", "wc", "grep", "head", "tail"];

    let tokens = tokens;
    let mut command_config: Option<(CommandConfig, usize)> = None;
    for (cmd_pattern, cfg) in &allowlist {
        let cmd_tokens: Vec<&str> = cmd_pattern.split(' ').collect();
        if tokens.len() >= cmd_tokens.len()
            && tokens
                .iter()
                .zip(cmd_tokens.iter())
                .all(|(t, c)| t == c)
        {
            command_config = Some((cfg.clone(), cmd_tokens.len()));
            break;
        }
    }
    let Some((config, command_tokens)) = command_config else {
        return false;
    };

    if tokens[0] == "git" && tokens.get(1) == Some(&"ls-remote".to_string()) {
        for token in tokens.iter().skip(2) {
            if token.is_empty() || token.starts_with('-') {
                continue;
            }
            if token.contains("://") || token.contains("$") {
                return false;
            }
        }
    }

    for token in tokens.iter().skip(command_tokens) {
        if token.contains('$') {
            return false;
        }
        if token.contains('{') && (token.contains(',') || token.contains("..")) {
            return false;
        }
    }

    if !validate_flags(
        &tokens,
        command_tokens,
        &config,
        tokens.first().map(String::as_str).unwrap_or(""),
        tokens[0] == "xargs",
        &SAFE_TARGET_COMMANDS_FOR_XARGS,
    ) {
        return false;
    }

    if let Some(callback) = config.callback {
        if callback(command, &tokens[command_tokens..]) {
            return false;
        }
    }

    true
}

fn validate_flag_argument(value: &str, arg_type: FlagArgType) -> bool {
    match arg_type {
        FlagArgType::None => false,
        FlagArgType::Number => !value.is_empty() && value.chars().all(|c| c.is_ascii_digit()),
        FlagArgType::String => true,
        FlagArgType::Char => value.chars().count() == 1,
        FlagArgType::Literal => value == "{}",
        FlagArgType::Eof => value == "EOF",
    }
}

fn validate_flags(
    tokens: &[String],
    start_index: usize,
    config: &CommandConfig,
    command_name: &str,
    xargs_mode: bool,
    xargs_target_commands: &[&str],
) -> bool {
    static FLAG_PATTERN: Lazy<Regex> = Lazy::new(|| Regex::new(r"^-[a-zA-Z0-9_-]").unwrap());

    let mut i = start_index;
    while i < tokens.len() {
        let token = tokens[i].clone();
        if token.is_empty() {
            i += 1;
            continue;
        }

        if xargs_mode && command_name == "xargs" && (!token.starts_with('-') || token == "--") {
            let mut cursor = i;
            if token == "--" && cursor + 1 < tokens.len() {
                cursor += 1;
            }
            if tokens
                .get(cursor)
                .is_some_and(|t| xargs_target_commands.contains(&t.as_str()))
            {
                break;
            }
            return false;
        }

        if token == "--" {
            if config.respects_double_dash {
                i += 1;
                break;
            }
            i += 1;
            continue;
        }

        if token.starts_with('-') && token.len() > 1 && FLAG_PATTERN.is_match(&token) {
            let has_equals = token.contains('=');
            let (flag, inline_value) = match token.split_once('=') {
                Some((f, v)) => (f.to_string(), v.to_string()),
                None => (token.clone(), String::new()),
            };
            if flag.is_empty() {
                return false;
            }
            let flag_arg_type = config.safe_flags.get(&flag);

            let Some(flag_value) = flag_arg_type else {
                if command_name == "git" && Regex::new(r"^-\d+$").unwrap().is_match(&flag) {
                    i += 1;
                    continue;
                }
                if (command_name == "grep" || command_name == "rg")
                    && flag.starts_with('-')
                    && !flag.starts_with("--")
                    && flag.len() > 2
                {
                    let potential_flag = &flag[..2];
                    let potential_value = &flag[2..];
                    if let Some(cfg_type) = config.safe_flags.get(potential_flag) {
                        if (matches!(cfg_type, FlagArgType::Number | FlagArgType::String))
                            && potential_value.chars().all(|c| c.is_ascii_digit())
                        {
                            i += 1;
                            continue;
                        }
                    }
                }
                if flag.starts_with('-') && !flag.starts_with("--") && flag.len() > 2 {
                    let mut valid = true;
                    for j in 1..flag.len() {
                        let single = format!("-{}", &flag[j..=j]);
                        match config.safe_flags.get(&single) {
                            Some(FlagArgType::None) => {}
                            _ => {
                                valid = false;
                                break;
                            }
                        }
                    }
                    if valid {
                        i += 1;
                        continue;
                    }
                    return false;
                }
                return false;
            };

            if matches!(flag_value, FlagArgType::None) {
                if has_equals {
                    return false;
                }
                i += 1;
            } else {
                let arg_value: String;
                if has_equals {
                    arg_value = inline_value;
                    i += 1;
                } else {
                    if i + 1 >= tokens.len()
                        || (tokens[i + 1].starts_with('-')
                            && tokens[i + 1].len() > 1
                            && FLAG_PATTERN.is_match(&tokens[i + 1]))
                    {
                        return false;
                    }
                    arg_value = tokens[i + 1].clone();
                    i += 2;
                }
                if matches!(flag_value, FlagArgType::String)
                    && arg_value.starts_with('-')
                {
                    if flag == "--sort"
                        && command_name == "git"
                        && Regex::new(r"^-[a-zA-Z]").unwrap().is_match(&arg_value)
                    {
                        // reverse sort allowed
                    } else {
                        return false;
                    }
                }
                if !validate_flag_argument(&arg_value, *flag_value) {
                    return false;
                }
            }
        } else {
            i += 1;
        }
    }
    true
}

fn sed_command_allowed(command: &str) -> bool {
    // Port of Claude's sedCommandIsAllowedByAllowlist (read-only mode).
    let Some((_cmd, rest)) = command.trim().split_once(" ") else {
        return false;
    };
    // Reject dangerous flag combinations -ew/-eW/-ee/-we.
    if Regex::new(r"-e[wWe]").unwrap().is_match(rest)
        || Regex::new(r"-w[eE]").unwrap().is_match(rest)
    {
        return false;
    }
    let tokens = match shlex::split(rest) {
        Some(t) => t,
        None => return false,
    };
    let expressions = match extract_sed_expressions(&tokens) {
        Some(e) => e,
        None => return false,
    };
    let has_file_args = sed_has_file_args(&tokens);

    let allowed_flags = [
        "-n", "--quiet", "--silent", "-E", "--regexp-extended", "-r", "-z",
        "--zero-terminated", "--posix",
    ];
    let flags_ok = tokens.iter().all(|t| {
        if t.starts_with('-') && t != "--" {
            if t.starts_with("--") {
                allowed_flags.contains(&t.as_str())
            } else {
                t[1..].chars().all(|c| allowed_flags.contains(&format!("-{c}").as_str()))
            }
        } else {
            true
        }
    });
    if !flags_ok {
        return false;
    }

    // Pattern 1: line printing with -n.
    let has_n_flag = tokens
        .iter()
        .any(|t| matches!(t.as_str(), "-n" | "--quiet" | "--silent") || (t.starts_with('-') && !t.starts_with("--") && t.contains('n')));
    let pattern1 = if has_n_flag && !expressions.is_empty() {
        expressions.iter().all(|expr| {
            expr.split(';').all(|c| {
                let c = c.trim();
                Regex::new(r"^(?:\d+|\d+,\d+)?p$").unwrap().is_match(c)
            })
        })
    } else {
        false
    };

    // Pattern 2: single substitution s/a/b/flags.
    let mut pattern2 = false;
    if expressions.len() == 1 && !has_file_args {
        let expr = expressions[0].trim();
        if expr.starts_with('s') {
            // Claude strips the `s/` prefix (`/^s\/(.*?)$/`), so the delimiter
            // scan runs on the pattern/replacement body only.
            let rest = if expr.as_bytes().get(1) == Some(&b'/') {
                &expr[2..]
            } else {
                &expr[1..]
            };
            let mut delimiter_count = 0;
            let mut last_delimiter = None;
            let mut i = 0;
            let bytes: Vec<char> = rest.chars().collect();
            while i < bytes.len() {
                if bytes[i] == '\\' {
                    i += 2;
                    continue;
                }
                if bytes[i] == '/' {
                    delimiter_count += 1;
                    last_delimiter = Some(i);
                }
                i += 1;
            }
        
            if delimiter_count == 2 {
                let expr_flags: String = bytes[last_delimiter.unwrap() + 1..].iter().collect();
                let allowed_flag_chars = Regex::new(r"^[gpimIM]*[1-9]?[gpimIM]*$").unwrap();
                pattern2 = allowed_flag_chars.is_match(&expr_flags)
                    && expr.chars().next().unwrap() == 's'
                    && expr.as_bytes().get(1) == Some(&b'/');
            }
        }
    }


    if !pattern1 && !pattern2 {
        return false;
    }
    if pattern2 && expressions.iter().any(|e| e.contains(';')) {
        return false;
    }
    if expressions.iter().any(|e| sed_contains_dangerous_operations(e)) {
        return false;
    }
    true
}

fn extract_sed_expressions(tokens: &[String]) -> Option<Vec<String>> {
    let mut expressions: Vec<String> = Vec::new();
    let mut found_e_flag = false;
    let mut found_expression = false;
    let mut i = 0;
    while i < tokens.len() {
        let arg = tokens[i].clone();
        if arg == "-e" || arg == "--expression" {
            if i + 1 >= tokens.len() {
                return None;
            }
            found_e_flag = true;
            expressions.push(tokens[i + 1].clone());
            i += 2;
            continue;
        }
        if let Some(v) = arg.strip_prefix("--expression=") {
            found_e_flag = true;
            expressions.push(v.to_string());
            i += 1;
            continue;
        }
        if let Some(v) = arg.strip_prefix("-e=") {
            found_e_flag = true;
            expressions.push(v.to_string());
            i += 1;
            continue;
        }
        if arg.starts_with('-') {
            i += 1;
            continue;
        }
        if !found_e_flag && !found_expression {
            expressions.push(arg);
            found_expression = true;
            i += 1;
            continue;
        }
        break;
    }
    Some(expressions)
}

fn sed_has_file_args(tokens: &[String]) -> bool {
    // Port of Claude's hasFileArgs: returns true when filenames are present.
    let mut arg_count = 0;
    let mut has_e_flag = false;
    let mut i = 0;
    while i < tokens.len() {
        let arg = tokens[i].clone();
        if arg == "-e" || arg == "--expression" {
            has_e_flag = true;
            i += 2;
            continue;
        }
        if arg.starts_with("--expression=") || arg.starts_with("-e=") {
            has_e_flag = true;
            i += 1;
            continue;
        }
        if arg.starts_with('-') {
            i += 1;
            continue;
        }
        arg_count += 1;
        if has_e_flag {
            return true;
        }
        if arg_count > 1 {
            return true;
        }
        i += 1;
    }
    false
}

fn sed_contains_dangerous_operations(expression: &str) -> bool {
    let cmd = expression.trim();
    if cmd.is_empty() {
        return false;
    }
    if !cmd.is_ascii() {
        return true;
    }
    if cmd.contains('{') || cmd.contains('}') || cmd.contains('\n') {
        return true;
    }
    let hash_index = cmd.find('#');
    if let Some(hash_index) = hash_index {
        if !(hash_index > 0 && cmd.as_bytes()[hash_index - 1] == b's') {
            return true;
        }
    }
    if Regex::new(r"^!").unwrap().is_match(cmd)
        || Regex::new(r"[/\d$]!").unwrap().is_match(cmd)
    {
        return true;
    }
    if Regex::new(r"\d\s*~\s*\d|,\s*~\s*\d|\$\s*~\s*\d").unwrap().is_match(cmd) {
        return true;
    }
    if cmd.starts_with(',') {
        return true;
    }
    if Regex::new(r",\s*[+-]").unwrap().is_match(cmd) {
        return true;
    }
    if Regex::new(r"s\\").unwrap().is_match(cmd)
        || Regex::new(r"\\[|#%@]").unwrap().is_match(cmd)
    {
        return true;
    }
    if Regex::new(r"\\\/.*[wW]").unwrap().is_match(cmd) {
        return true;
    }
    false
}

// ---------------------------------------------------------------------------
// UNC path detection (Claude's containsVulnerableUncPath)
// ---------------------------------------------------------------------------

pub fn contains_vulnerable_unc_path(path: &str) -> bool {
    if !cfg!(windows) {
        return false;
    }
    if Regex::new(r"\\\\[^\s\\/]+(?:@(?:\d+|ssl))?(?:[\\/]|$|\s)").unwrap().is_match(path) {
        return true;
    }
    if Regex::new(r"(?:^|[^:])\/\/[^\s\\/]+(?:@(?:\d+|ssl))?(?:[\\/]|$|\s)")
        .unwrap()
        .is_match(path)
    {
        return true;
    }
    if Regex::new(r"/\\{2,}[^\s\\/]").unwrap().is_match(path) {
        return true;
    }
    if Regex::new(r"\\{2,}/[^\s\\/]").unwrap().is_match(path) {
        return true;
    }
    if Regex::new(r"@SSL@\d+").unwrap().is_match(path)
        || Regex::new(r"@\d+@SSL").unwrap().is_match(path)
    {
        return true;
    }
    if Regex::new(r"DavWWWRoot").unwrap().is_match(path) {
        return true;
    }
    false
}
