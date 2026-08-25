//! Read-only plain command verification ported from Claude Code
//! `tools/BashTool/readOnlyValidation.ts` (isCommandReadOnly +
//! READONLY_COMMAND_REGEXES + containsUnquotedExpansion). This covers the
//! simple commands allowed by the plain regex layer; flag tables for
//! git/gh/docker/rg/pyright/sed/xargs live in `readonly.rs`.


use regex::Regex;

use crate::readonly::contains_vulnerable_unc_path;

fn match_simple_command(command: &str, name: &str) -> bool {
    // makeRegexForSafeCommand: /^name(?:\s|$)[^<>()$`|{}&;\n\r]*$/
    let pattern = format!(
        "^{}(?:\\s|$)[^<>()$`|{{}}&;\\n\\r]*$",
        regex::escape(name)
    );
    Regex::new(&pattern).unwrap().is_match(command)
}

pub fn contains_unquoted_expansion(command: &str) -> bool {
    let mut in_single_quote = false;
    let mut in_double_quote = false;
    let mut escaped = false;
    let chars: Vec<char> = command.chars().collect();
    let mut i = 0;
    while i < chars.len() {
        let c = chars[i];
        if escaped {
            escaped = false;
            i += 1;
            continue;
        }
        if c == '\\' && !in_single_quote {
            escaped = true;
            i += 1;
            continue;
        }
        if c == '\'' && !in_double_quote {
            in_single_quote = !in_single_quote;
            i += 1;
            continue;
        }
        if c == '"' && !in_single_quote {
            in_double_quote = !in_double_quote;
            i += 1;
            continue;
        }
        if in_single_quote {
            i += 1;
            continue;
        }
        if c == '$' {
            let next = chars.get(i + 1).copied();
            if next.is_some_and(|n| n.is_ascii_alphanumeric() || "_@*#?!$-".contains(n)) {
                return true;
            }
        }
        if in_double_quote {
            i += 1;
            continue;
        }
        if matches!(c, '?' | '*' | '[' | ']') {
            return true;
        }
        i += 1;
    }
    false
}

const READONLY_COMMANDS: &[&str] = &[
    "cal", "uptime", "cat", "head", "tail", "wc", "stat", "strings", "hexdump", "od", "nl", "id",
    "uname", "free", "df", "du", "locale", "groups", "nproc", "basename", "dirname", "realpath",
    "cut", "paste", "tr", "column", "tac", "rev", "fold", "expand", "unexpand", "fmt", "comm",
    "cmp", "numfmt", "readlink", "diff", "true", "false", "sleep", "which", "type", "expr",
    "test", "getconf", "seq", "tsort", "pr",
];

fn custom_regexes() -> Vec<&'static str> {
    vec![
        r#"^echo(?:\s+(?:'[^']*'|"[^"$<>\n\r]*"|[^|;&`$(){}><#\\!"'\s]+))*(?:\s+2>&1)?\s*$"#,
        r"^uniq(?:\s+(?:-[a-zA-Z]+|--[a-zA-Z-]+(?:=\S+)?|-[fsw]\s+\d+))*(?:\s|$)\s*$",
        r"^pwd$",
        r"^whoami$",
        r"^node -v$",
        r"^node --version$",
        r"^python --version$",
        r"^python3 --version$",
        r"^history(?:\s+\d+)?\s*$",
        r"^alias$",
        r"^arch(?:\s+(?:--help|-h))?\s*$",
        r"^ip addr$",
        r"^ifconfig(?:\s+[a-zA-Z][a-zA-Z0-9_-]*)?\s*$",
        r#"^jq(?:\s+(?:-[a-zA-Z]+|--[a-zA-Z-]+(?:=\S+)?))*(?:\s+'[^'`]*'|\s+"[^"`]*"|\s+[^-\s'"][^\s]*)+\s*$"#,
        r#"^cd(?:\s+(?:'[^']*'|"[^"]*"|[^\s;|&`$(){}><#\\]+))?$"#,
        r"^ls(?:\s+[^<>()$`|{}&;\n\r]*)?$",
        r"^find(?:\s+[^<>$`|{}&;\n\r]*)?$",
    ]
}

/// Claude's isCommandReadOnly for a single command string.
pub fn is_command_read_only(command: &str) -> bool {
    let mut test_command = command.trim().to_string();
    if test_command.ends_with(" 2>&1") {
        test_command = test_command[..test_command.len() - 5].trim().to_string();
    }
    if contains_vulnerable_unc_path(&test_command) {
        return false;
    }
    if contains_unquoted_expansion(&test_command) {
        return false;
    }
    if crate::readonly::is_command_safe_via_flag_parsing(&test_command) {
        return true;
    }
    for pattern in custom_regexes() {
        if Regex::new(pattern).unwrap().is_match(&test_command) {
            if test_command.contains("git")
                && (Regex::new(r"\s-c[\s=]").unwrap().is_match(&test_command)
                    || Regex::new(r"\s--exec-path[\s=]").unwrap().is_match(&test_command)
                    || Regex::new(r"\s--config-env[\s=]").unwrap().is_match(&test_command))
            {
                return false;
            }
            if test_command.starts_with("jq")
                && Regex::new(r"(?:-f\b|--from-file|--rawfile|--slurpfile|--run-tests|-L\b|--library-path|\benv\b|\$ENV\b)")
                    .unwrap()
                    .is_match(&test_command)
            {
                return false;
            }
            if test_command.starts_with("find")
                && Regex::new(r"(?:^|\s)-(?:delete|execdir|exec|okdir|ok|fprint0?|fls|fprintf)\b")
                    .unwrap()
                    .is_match(&test_command)
            {
                return false;
            }
            return true;
        }
    }
    for name in READONLY_COMMANDS {
        if match_simple_command(&test_command, name) {
            return true;
        }
    }
    false
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sed_substitution_isolated_steps() {
        assert!(crate::readonly::is_command_safe_via_flag_parsing("sed s/a/b/g"));
        assert!(is_command_read_only("sed s/a/b/g"));
    }

    #[test]
    fn simple_commands_are_read_only() {
        assert!(is_command_read_only("cat foo.txt"));
        assert!(is_command_read_only("ls -la"));
        assert!(is_command_read_only("pwd"));
        assert!(is_command_read_only("echo hello world"));
        assert!(is_command_read_only("git status"));
        assert!(is_command_read_only("git diff -p"));
        assert!(is_command_read_only("rg foo"));
        assert!(is_command_read_only("sed -n '1,5p' file.txt"));
        assert!(is_command_read_only("sed s/a/b/g"));
        assert!(is_command_read_only("sort -n foo"));
        assert!(is_command_read_only("cd /tmp"));
        assert!(is_command_read_only("find . -name file.txt"));
        assert!(is_command_read_only("wc -l"));
    }

    #[test]
    fn mutating_commands_are_not_read_only() {
        assert!(!is_command_read_only("rm -rf /tmp/x"));
        assert!(!is_command_read_only("touch foo"));
        assert!(!is_command_read_only("git branch new-branch"));
        assert!(!is_command_read_only("sed -i 's/a/b/' file.txt"));
        assert!(!is_command_read_only("git diff --output file.txt"));
        assert!(!is_command_read_only("cat foo.txt > out.txt"));
    }

    #[test]
    fn expansions_block_read_only() {
        assert!(!is_command_read_only("ls $HOME"));
        assert!(!is_command_read_only("ls *.txt"));
        assert!(!is_command_read_only("echo \"$HOME\""));
    }

    #[test]
    fn unc_blocked_on_windows() {
        #[cfg(windows)]
        {
            assert!(!is_command_read_only("cat \\\\server\\share\\file.txt"));
        }
        #[cfg(not(windows))]
        {
            let _ = "skip";
        }
    }
}
