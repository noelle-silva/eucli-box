#![allow(dead_code)] // Ported-from-reference interfaces, some not yet wired into the protocol; kept for later stages.
//! Shell type detection ported from Codex `shell-command/src/shell_detect.rs`
//! (the type-mapping subset; the executable-lookup parts are unused by the
//! analyzer so they are not ported).

use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

#[derive(Debug, PartialEq, Eq, Clone, Copy, Serialize, Deserialize)]
pub enum ShellType {
    Zsh,
    Bash,
    PowerShell,
    Sh,
    Cmd,
}

impl ShellType {
    pub fn name(self) -> &'static str {
        match self {
            Self::Zsh => "zsh",
            Self::Bash => "bash",
            Self::PowerShell => "powershell",
            Self::Sh => "sh",
            Self::Cmd => "cmd",
        }
    }
}

pub fn detect_shell_type(shell_path: impl AsRef<Path>) -> Option<ShellType> {
    let shell_path = shell_path.as_ref();
    match shell_path.as_os_str().to_str() {
        Some("zsh") => Some(ShellType::Zsh),
        Some("sh") => Some(ShellType::Sh),
        Some("cmd") => Some(ShellType::Cmd),
        Some("bash") => Some(ShellType::Bash),
        Some("pwsh") => Some(ShellType::PowerShell),
        Some("powershell") => Some(ShellType::PowerShell),
        _ => {
            let shell_name = shell_path.file_stem();
            if let Some(shell_name) = shell_name {
                let shell_name_path = PathBuf::from(shell_name);
                if shell_name_path != shell_path {
                    return detect_shell_type(shell_name_path);
                }
            }
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_detect_shell_type() {
        assert_eq!(detect_shell_type(PathBuf::from("zsh")), Some(ShellType::Zsh));
        assert_eq!(detect_shell_type(PathBuf::from("bash")), Some(ShellType::Bash));
        assert_eq!(detect_shell_type(PathBuf::from("pwsh")), Some(ShellType::PowerShell));
        assert_eq!(detect_shell_type(PathBuf::from("powershell")), Some(ShellType::PowerShell));
        assert_eq!(detect_shell_type(PathBuf::from("fish")), None);
        assert_eq!(detect_shell_type(PathBuf::from("/bin/zsh")), Some(ShellType::Zsh));
        assert_eq!(detect_shell_type(PathBuf::from("/usr/bin/bash")), Some(ShellType::Bash));
        assert_eq!(detect_shell_type(PathBuf::from("powershell.exe")), Some(ShellType::PowerShell));
        assert_eq!(detect_shell_type(PathBuf::from("cmd")), Some(ShellType::Cmd));
        assert_eq!(detect_shell_type(PathBuf::from("cmd.exe")), Some(ShellType::Cmd));
    }
}
