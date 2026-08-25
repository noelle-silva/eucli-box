//! Command safety classification ported from Codex `shell-command`.
//! - `is_known_safe_command`: known-safe whitelist (exec direct + bash -lc
//!   compound + Windows PowerShell safelist).
//! - `dangerous_command_match`: dangerous blacklist with wrapper unwrapping.
//! Rules are shared by submodules to avoid git-global-option bypasses.

pub mod common;
pub mod dangerous;
pub mod safe;
#[cfg(windows)]
pub(crate) mod windows;

pub use dangerous::dangerous_command_match;
pub use dangerous::DangerousCommandMatch;
pub use safe::is_known_safe_command;
