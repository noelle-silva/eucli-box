//! PowerShell invocation extraction ported from Codex
//! `shell-command/src/powershell.rs` (the parts used by the analyzer).

/// Tokenize an inline PowerShell invocation string into argv words.
/// PowerShell scripts are single-quoted strings for shlex purposes.
pub fn tokenize_powershell_invocation(command: &str) -> Option<Vec<String>> {
    shlex::split(command)
}

/// Resolve the PowerShell executable named in the first token against PATH
/// when the token is a bare name (Codex powershell.rs behavior: the caller
/// resolves the real executable before safelist judgment).
pub fn resolve_powershell_executable(tokens: &mut [String]) {
    let Some(first) = tokens.first() else {
        return;
    };
    let lower = first.to_ascii_lowercase();
    let name = if lower == "pwsh" || lower == "pwsh.exe" {
        "pwsh.exe"
    } else if lower == "powershell" || lower == "powershell.exe" {
        "powershell.exe"
    } else {
        return;
    };
    if first.contains('/') || first.contains('\\') || first.contains(':') {
        return;
    }
    if let Ok(found) = which::which(name) {
        tokens[0] = found.to_string_lossy().into_owned();
    }
}

const POWERSHELL_FLAGS: &[&str] = &["-nologo", "-noprofile", "-command", "-c"];

/// Extract the PowerShell script body from an invocation like
/// `["pwsh", "-NoProfile", "-Command", "...script..."]`.
pub fn extract_powershell_command(command: &[String]) -> Option<(&str, &str)> {
    if command.len() < 3 {
        return None;
    }
    let shell = &command[0];
    let shell_kind = crate::shell_type::detect_shell_type(std::path::PathBuf::from(shell));
    if !matches!(shell_kind, Some(crate::shell_type::ShellType::PowerShell)) {
        return None;
    }
    let mut i = 1usize;
    while i + 1 < command.len() {
        let flag = &command[i];
        let lower = flag.to_ascii_lowercase();
        if !POWERSHELL_FLAGS.contains(&lower.as_str()) {
            return None;
        }
        if lower == "-command" || lower == "-c" {
            let script = &command[i + 1];
            return Some((shell, script));
        }
        i += 1;
    }
    None
}
