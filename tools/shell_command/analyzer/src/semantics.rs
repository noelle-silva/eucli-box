//! Post-argv semantic checks ported from Claude Code `utils/bash/ast.ts`
//! (checkSemantics). Runs after parsing to catch commands that tokenize fine
//! but are dangerous by name or argument content. Returns the first failure.

use once_cell::sync::Lazy;
use regex::Regex;

use crate::protocol::InternalCommand;

fn eval_like_builtins() -> &'static [&'static str] {
    &[
        "eval", "source", ".", "exec", "command", "builtin", "fc", "coproc", "noglob",
        "nocorrect", "trap", "enable", "mapfile", "readarray", "hash", "bind", "complete",
        "compgen", "alias", "let",
    ]
}

fn zsh_dangerous_builtins() -> &'static [&'static str] {
    &[
        "zmodload", "emulate", "sysopen", "sysread", "syswrite", "sysseek", "zpty", "ztcp",
        "zsocket", "zf_rm", "zf_mv", "zf_ln", "zf_chmod", "zf_chown", "zf_mkdir", "zf_rmdir",
        "zf_chgrp",
    ]
}

fn shell_keywords() -> &'static [&'static str] {
    &[
        "if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case",
        "esac", "in", "function", "select", "time", "coproc", "!", "{", "}", "[[", "]]",
    ]
}

static SUBSCRIPT_EVAL_FLAGS: Lazy<std::collections::HashMap<&'static str, &'static [&'static str]>> =
    Lazy::new(|| {
        let mut m = std::collections::HashMap::new();
        m.insert("test", &["-v", "-R"][..]);
        m.insert("[", &["-v", "-R"][..]);
        m.insert("[[", &["-v", "-R"][..]);
        m.insert("printf", &["-v"][..]);
        m.insert("read", &["-a"][..]);
        m.insert("unset", &["-v"][..]);
        m.insert("wait", &["-p"][..]);
        m
    });

const TEST_ARITH_CMP_OPS: &[&str] = &["-eq", "-ne", "-lt", "-le", "-gt", "-ge"];
const BARE_SUBSCRIPT_NAME_BUILTINS: &[&str] = &["read", "unset"];
const READ_DATA_FLAGS: &[&str] = &["-p", "-d", "-n", "-N", "-t", "-u", "-i"];

static PROC_ENVIRON_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"/proc/.*/environ").unwrap());
static NEWLINE_HASH_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"\n[ \t]*#").unwrap());
static DURATION_RE: Lazy<Regex> =
    Lazy::new(|| Regex::new(r"^\d+(?:\.\d+)?[smhd]?$").unwrap());
static NICE_DURATION_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"^-?\d+$").unwrap());
static VALUE_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"^[A-Za-z0-9_.+-]+$").unwrap());

/// Claude-style post-argv verdict split:
/// - `Dangerous`  - the command is provably dangerous by name/content
///                  (eval-like builtins, subscript injection, procfs access,
///                  jq system(), newline-hash hiding).
/// - `Untrusted`  - argv cannot be fully trusted (fail-closed per Claude),
///                  the caller must ask instead of auto-approving.
#[derive(Debug, PartialEq, Eq)]
pub enum SemanticVerdict {
    Safe,
    Dangerous(String),
    Untrusted(String),
}

impl SemanticVerdict {
    pub fn dangerous_msg(&self) -> Option<&str> {
        match self {
            SemanticVerdict::Dangerous(m) => Some(m),
            _ => None,
        }
    }
}

pub fn check_semantics(commands: &[InternalCommand]) -> SemanticVerdict {
    for cmd in commands {
        // Strip safe wrapper commands (time, nohup, timeout N, nice -n N,
        // env VAR=..., stdbuf) so the wrapped command is the one checked.
        let mut a: Vec<String> = cmd.argv.clone();
        loop {
            match a.first().map(String::as_str) {
                Some("time") | Some("nohup") => {
                    a = a[1..].to_vec();
                }
                Some("timeout") => {
                    let mut i = 1;
                    let mut failed = false;
                    while i < a.len() {
                        let arg = a[i].clone();
                        if matches!(arg.as_str(), "--foreground" | "--preserve-status" | "--verbose") {
                            i += 1;
                        } else if Regex::new(r"^--(?:kill-after|signal)=[A-Za-z0-9_.+-]+$")
                            .unwrap()
                            .is_match(&arg)
                        {
                            i += 1;
                        } else if (arg == "--kill-after" || arg == "--signal")
                            && i + 1 < a.len()
                            && VALUE_RE.is_match(&a[i + 1])
                        {
                            i += 2;
                        } else if arg.starts_with("--") {
                            failed = true;
                            break;
                        } else if arg == "-v" {
                            i += 1;
                        } else if (arg == "-k" || arg == "-s")
                            && i + 1 < a.len()
                            && VALUE_RE.is_match(&a[i + 1])
                        {
                            i += 2;
                        } else if Regex::new(r"^-[ks][A-Za-z0-9_.+-]+$").unwrap().is_match(&arg) {
                            i += 1;
                        } else if arg.starts_with('-') {
                            failed = true;
                            break;
                        } else {
                            break;
                        }
                    }
                    if failed {
                        return SemanticVerdict::Untrusted("timeout with unknown flag cannot be statically analyzed".into());
                    }
                    if let Some(dur) = a.get(i) {
                        if !DURATION_RE.is_match(dur) {
                            return SemanticVerdict::Untrusted(format!(
                                "timeout duration '{dur}' cannot be statically analyzed"
                            ));
                        }
                        a = a[i + 1..].to_vec();
                    } else {
                        break;
                    }
                }
                Some("nice") => {
                    if a.get(1).map(String::as_str) == Some("-n")
                        && a.get(2).map(String::as_str).is_some_and(|v| NICE_DURATION_RE.is_match(v))
                    {
                        a = a[3..].to_vec();
                    } else if a
                        .get(1)
                        .is_some_and(|v| NICE_DURATION_RE.is_match(v))
                    {
                        a = a[2..].to_vec();
                    } else if a
                        .get(1)
                        .is_some_and(|v| v.contains('$') || v.contains('(') || v.contains('`'))
                    {
                        return SemanticVerdict::Untrusted(
                            "nice argument contains expansion — cannot statically determine wrapped command"
                                .into(),
                        );
                    } else {
                        a = a[1..].to_vec();
                    }
                }
                Some("env") => {
                    let mut i = 1;
                    let mut failed = false;
                    while i < a.len() {
                        let arg = a[i].clone();
                        if arg.contains('=') && !arg.starts_with('-') {
                            i += 1;
                        } else if matches!(arg.as_str(), "-i" | "-0" | "-v") {
                            i += 1;
                        } else if arg == "-u" && i + 1 < a.len() {
                            i += 2;
                        } else if arg.starts_with('-') {
                            failed = true;
                            break;
                        } else {
                            break;
                        }
                    }
                    if failed {
                        let flag = a.get(i).cloned().unwrap_or_default();
                        return SemanticVerdict::Untrusted(format!(
                            "env with '{flag}' flag cannot be statically analyzed"
                        ));
                    }
                    if i < a.len() {
                        a = a[i..].to_vec();
                    } else {
                        break;
                    }
                }
                Some("stdbuf") => {
                    let mut i = 1;
                    let mut failed = false;
                    while i < a.len() {
                        let arg = a[i].clone();
                        if Regex::new(r"^-[ioe]$").unwrap().is_match(&arg) && i + 1 < a.len() {
                            i += 2;
                        } else if Regex::new(r"^-[ioe].").unwrap().is_match(&arg) {
                            i += 1;
                        } else if Regex::new(r"^--(input|output|error)=").unwrap().is_match(&arg) {
                            i += 1;
                        } else if arg.starts_with('-') {
                            failed = true;
                            break;
                        } else {
                            break;
                        }
                    }
                    if failed {
                        return SemanticVerdict::Untrusted("stdbuf with unknown flag cannot be statically analyzed".into());
                    }
                    if i > 1 && i < a.len() {
                        a = a[i..].to_vec();
                    } else {
                        break;
                    }
                }
                _ => break,
            }
        }

        let Some(name) = a.first().cloned() else {
            continue;
        };

        // Empty command name.
        if name.is_empty() {
            return SemanticVerdict::Untrusted("Empty command name — argv[0] may not reflect what bash runs".into());
        }
        if name.contains("__CMDSUB_OUTPUT__") || name.contains("__TRACKED_VAR__") {
            return SemanticVerdict::Untrusted("Command name is runtime-determined (placeholder argv[0])".into());
        }
        if name.starts_with('-') || name.starts_with('|') || name.starts_with('&') {
            return SemanticVerdict::Untrusted("Command appears to be an incomplete fragment".into());
        }

        // Subscript-eval flags (test/[/[[/printf/read/unset/wait + array subscripts).
        if let Some(danger_flags) = SUBSCRIPT_EVAL_FLAGS.get(name.as_str()) {
            for i in 1..a.len() {
                let arg = a[i].clone();
                if danger_flags.contains(&arg.as_str())
                    && a.get(i + 1).map(String::as_str).is_some_and(|v| v.contains('['))
                {
                    return SemanticVerdict::Dangerous(
                        "'X OPERAND contains array subscript — bash evaluates $(cmd) in subscripts"
                            .into(),
                    );
                }
                if arg.len() > 2 && arg.starts_with('-') && !arg.starts_with("--") && !arg.contains('[') {
                    for &flag in *danger_flags {
                        if flag.len() == 2 && arg.contains(&flag[1..2]) {
                            if a.get(i + 1).map(String::as_str).is_some_and(|v| v.contains('[')) {
                                return SemanticVerdict::Dangerous(
                                    "combined flag operand contains array subscript — bash evaluates $(cmd) in subscripts"
                                        .into(),
                                );
                            }
                        }
                    }
                }
                for &flag in *danger_flags {
                    if flag.len() == 2
                        && arg.starts_with(flag)
                        && arg.len() > 2
                        && arg.contains('[')
                    {
                        return SemanticVerdict::Dangerous(
                            "fused flag operand contains array subscript — bash evaluates $(cmd) in subscripts"
                                .into(),
                        );
                    }
                }
            }
        }

        // `[[ ARG OP ARG ]]` arithmetic comparison.
        if name == "[[" {
            for i in 2..a.len() {
                if !TEST_ARITH_CMP_OPS.contains(&a[i].as_str()) {
                    continue;
                }
                if a.get(i - 1).is_some_and(|v| v.contains('['))
                    || a.get(i + 1).is_some_and(|v| v.contains('['))
                {
                    return SemanticVerdict::Dangerous(
                        "[[ ... operand contains array subscript — bash arithmetically evaluates $(cmd) in subscripts"
                            .into(),
                    );
                }
            }
        }

        // read/unset treat every bare positional as a NAME.
        if BARE_SUBSCRIPT_NAME_BUILTINS.contains(&name.as_str()) {
            let mut skip_next = false;
            for i in 1..a.len() {
                let arg = a[i].clone();
                if skip_next {
                    skip_next = false;
                    continue;
                }
                if arg.starts_with('-') {
                    if name == "read" {
                        if READ_DATA_FLAGS.contains(&arg.as_str()) {
                            skip_next = true;
                        } else if arg.len() > 2 && !arg.starts_with("--") {
                            for (j, ch) in arg[1..].chars().enumerate() {
                                if READ_DATA_FLAGS.contains(&format!("-{ch}").as_str().to_owned().as_str()) {
                                    if j == arg.len() - 2 {
                                        skip_next = true;
                                    }
                                    break;
                                }
                            }
                        }
                    }
                    continue;
                }
                if arg.contains('[') {
                    return SemanticVerdict::Dangerous(
                        "'read'/'unset' positional NAME contains array subscript — bash evaluates $(cmd) in subscripts"
                            .into(),
                    );
                }
            }
        }

        // Shell reserved keywords as argv[0] indicate a tree-sitter mis-parse.
        if shell_keywords().contains(&name.as_str()) {
            return SemanticVerdict::Untrusted(format!(
                "Shell keyword '{name}' as command name — tree-sitter mis-parse"
            ));
        }

        // Newline followed by # inside argv, env values, redirect targets.
        for arg in &cmd.argv {
            if arg.contains('\n') && NEWLINE_HASH_RE.is_match(arg) {
                return SemanticVerdict::Untrusted(
                    "Newline followed by # inside a quoted argument can hide arguments from path validation"
                        .into(),
                );
            }
        }
        for (_, value) in &cmd.env_vars {
            if value.contains('\n') && NEWLINE_HASH_RE.is_match(value) {
                return SemanticVerdict::Untrusted(
                    "Newline followed by # inside an env var value can hide arguments from path validation"
                        .into(),
                );
            }
        }
        for r in &cmd.redirects {
            if r.target.contains('\n') && NEWLINE_HASH_RE.is_match(&r.target) {
                return SemanticVerdict::Untrusted(
                    "Newline followed by # inside a redirect target can hide arguments from path validation"
                        .into(),
                );
            }
        }

        // jq system() and dangerous flags.
        if name == "jq" {
            for arg in &a {
                if Regex::new(r"\bsystem\s*\(").unwrap().is_match(arg) {
                    return SemanticVerdict::Dangerous(
                        "jq command contains system() function which executes arbitrary commands"
                            .into(),
                    );
                }
            }
            if a.iter().any(|arg| {
                Regex::new(r"^(?:-[fL](?:$|[^A-Za-z])|--(?:from-file|rawfile|slurpfile|library-path)(?:$|=))")
                    .unwrap()
                    .is_match(arg)
            }) {
                return SemanticVerdict::Dangerous(
                    "jq command contains dangerous flags that could execute code or read arbitrary files"
                        .into(),
                );
            }
        }

        if zsh_dangerous_builtins().contains(&name.as_str()) {
            return SemanticVerdict::Dangerous(format!(
                "Zsh builtin '{name}' can bypass security checks"
            ));
        }

        if eval_like_builtins().contains(&name.as_str()) {
            // command -v/-V are POSIX existence checks that only print paths.
            if name == "command" && matches!(a.get(1).map(String::as_str), Some("-v") | Some("-V")) {
                // fall through
            } else if name == "fc"
                && !a[1..].iter().any(|arg| Regex::new(r"^-[^-]*[es]").unwrap().is_match(arg))
            {
                // fc -l / -ln only list history — safe.
            } else if name == "compgen"
                && !a[1..].iter().any(|arg| Regex::new(r"^-[^-]*[CFW]").unwrap().is_match(arg))
            {
                // compgen -c/-f/-v only list completions — safe.
            } else {
                return SemanticVerdict::Dangerous(format!(
                    "'{name}' evaluates arguments as shell code"
                ));
            }
        }

        // /proc/*/environ exposes env vars of other processes.
        for arg in &cmd.argv {
            if arg.contains("/proc/") && PROC_ENVIRON_RE.is_match(arg) {
                return SemanticVerdict::Dangerous("Accesses /proc/*/environ which may expose secrets".into());
            }
        }
        for r in &cmd.redirects {
            if r.target.contains("/proc/") && PROC_ENVIRON_RE.is_match(&r.target) {
                return SemanticVerdict::Dangerous("Accesses /proc/*/environ which may expose secrets".into());
            }
        }
    }
    SemanticVerdict::Safe
}
