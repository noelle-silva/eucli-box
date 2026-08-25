#![allow(dead_code)] // Ported-from-reference interfaces, some not yet wired into the protocol; kept for later stages.
//! Dangerous-command detection ported from Codex
//! `command_safety/is_dangerous_command.rs`.

use crate::bash::parse_shell_lc_literal_commands;
use crate::safety::common::executable_name_lookup_key;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DangerousCommandMatch {
    ForcedRm,
    Other,
}

const MAX_DANGEROUS_COMMAND_WRAPPER_DEPTH: usize = 8;

pub fn dangerous_command_match(command: &[String]) -> Option<DangerousCommandMatch> {
    dangerous_command_match_with_depth(command, 0)
}

fn dangerous_command_match_with_depth(
    command: &[String],
    wrapper_depth: usize,
) -> Option<DangerousCommandMatch> {
    if wrapper_depth > MAX_DANGEROUS_COMMAND_WRAPPER_DEPTH {
        return None;
    }

    if let Some(dangerous_match) = dangerous_command_match_for_exec(command, wrapper_depth) {
        return Some(dangerous_match);
    }

    if let Some(dangerous_match) = parse_shell_lc_literal_commands(command).and_then(|commands| {
        commands
            .iter()
            .find_map(|command| dangerous_command_match_with_depth(command, wrapper_depth + 1))
    }) {
        return Some(dangerous_match);
    }

    #[cfg(windows)]
    {
        if crate::safety::windows::is_dangerous_command_windows(command) {
            return Some(DangerousCommandMatch::Other);
        }
    }

    None
}

pub fn dangerous_powershell_words_match(command: &[String]) -> Option<DangerousCommandMatch> {
    #[cfg(windows)]
    {
        crate::safety::windows::is_dangerous_powershell_words(command)
            .then_some(DangerousCommandMatch::Other)
    }
    #[cfg(not(windows))]
    {
        let _ = command;
        None
    }
}

fn dangerous_command_match_for_exec(
    command: &[String],
    wrapper_depth: usize,
) -> Option<DangerousCommandMatch> {
    let cmd0 = command
        .first()
        .and_then(|command| executable_name_lookup_key(command));

    match cmd0.as_deref() {
        Some("rm") if rm_args_include_force_option(&command[1..]) => {
            Some(DangerousCommandMatch::ForcedRm)
        }
        Some("sudo") => dangerous_command_match_with_depth(&command[1..], wrapper_depth + 1),
        Some("env") => dangerous_command_match_for_env(command, wrapper_depth),
        Some("trap") => dangerous_command_match_for_trap(command, wrapper_depth),
        _ => None,
    }
}

fn dangerous_command_match_for_env(
    command: &[String],
    wrapper_depth: usize,
) -> Option<DangerousCommandMatch> {
    let mut command_index = 1;
    while let Some(argument) = command.get(command_index) {
        if argument == "--" {
            command_index += 1;
            break;
        }
        if matches!(argument.as_str(), "-i" | "--ignore-environment")
            || argument
                .split_once('=')
                .is_some_and(|(name, _)| !name.is_empty() && !name.starts_with('-'))
        {
            command_index += 1;
            continue;
        }
        break;
    }
    dangerous_command_match_with_depth(&command[command_index..], wrapper_depth + 1)
}

fn dangerous_command_match_for_trap(
    command: &[String],
    wrapper_depth: usize,
) -> Option<DangerousCommandMatch> {
    let mut action_index = 1;
    if command
        .get(action_index)
        .is_some_and(|argument| argument == "--")
    {
        action_index += 1;
    }
    let action = command
        .get(action_index)
        .filter(|action| !action.starts_with('-'))?;
    let shell_command = vec!["sh".to_string(), "-c".to_string(), action.clone()];
    dangerous_command_match_with_depth(&shell_command, wrapper_depth + 1)
}

fn rm_args_include_force_option(args: &[String]) -> bool {
    args.iter()
        .take_while(|arg| arg.as_str() != "--")
        .any(|arg| {
            arg == "--force"
                || arg
                    .strip_prefix('-')
                    .is_some_and(|flags| !flags.starts_with('-') && flags.contains('f'))
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn vec_str(items: &[&str]) -> Vec<String> {
        items.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn rm_rf_is_dangerous() {
        assert_eq!(
            dangerous_command_match(&vec_str(&["rm", "-rf", "/"])),
            Some(DangerousCommandMatch::ForcedRm)
        );
    }

    #[test]
    fn rm_f_is_dangerous() {
        assert_eq!(
            dangerous_command_match(&vec_str(&["rm", "-f", "/"])),
            Some(DangerousCommandMatch::ForcedRm)
        );
    }

    #[test]
    fn forced_rm_variants_are_dangerous() {
        for command in [
            vec_str(&["/bin/rm", "-fr", "/tmp/example"]),
            vec_str(&["rm", "-r", "-f", "/tmp/example"]),
            vec_str(&["rm", "--force", "/tmp/example"]),
            vec_str(&["rm", "/tmp/example", "-f"]),
            vec_str(&["sudo", "rm", "-rf", "/tmp/example"]),
            vec_str(&["env", "TARGET=/tmp/example", "rm", "-rf", "/tmp/example"]),
        ] {
            assert_eq!(
                dangerous_command_match(&command),
                Some(DangerousCommandMatch::ForcedRm),
                "{command:?}"
            );
        }
    }

    #[test]
    fn forced_rm_in_complex_shell_syntax_is_dangerous() {
        for script in [
            "printf x | rm -rf /tmp/example",
            "if test -d /tmp/example; then rm --force /tmp/example; fi",
            "rm -rf \"$TARGET\" >/dev/null",
            "for target in /tmp/a /tmp/b; do rm -r -f \"$target\"; done",
            "echo \"$(rm -rf /tmp/example)\"",
            "bash -c 'rm -rf /tmp/example'",
            "trap 'rm -rf /tmp/example' EXIT",
        ] {
            let command = vec_str(&["bash", "-lc", script]);
            assert_eq!(
                dangerous_command_match(&command),
                Some(DangerousCommandMatch::ForcedRm),
                "{script}"
            );
        }
    }

    #[test]
    fn non_forced_or_non_literal_rm_is_not_dangerous() {
        for command in [
            vec_str(&["rm", "-r", "/tmp/example"]),
            vec_str(&["rm", "--", "-f"]),
            vec_str(&["bash", "-lc", "echo 'rm -rf /tmp/example'"]),
            vec_str(&["bash", "-lc", "cmd=rm; $cmd -rf /tmp/example"]),
            vec_str(&["bash", "-lc", "trap 'echo rm -rf /tmp/example' EXIT"]),
        ] {
            assert_eq!(dangerous_command_match(&command), None, "{command:?}");
        }
    }
}
