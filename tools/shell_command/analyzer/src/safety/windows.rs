//! Windows-specific safety classifiers ported from Codex
//! `command_safety/windows_safe_commands.rs`,
//! `windows_dangerous_commands.rs` and `powershell_parser.rs` (+ the PS1
//! AST-parse harness). Compiled only on Windows.

use std::path::Path;

pub fn is_safe_command_windows(command: &[String]) -> bool {
    if let Some(commands) = try_parse_powershell_command_sequence(command) {
        commands
            .iter()
            .all(|cmd| is_safe_powershell_words(cmd.as_slice()))
    } else {
        false
    }
}

fn try_parse_powershell_command_sequence(command: &[String]) -> Option<Vec<Vec<String>>> {
    let (exe, rest) = command.split_first()?;
    if is_powershell_executable(exe) {
        parse_powershell_invocation(exe, rest)
    } else {
        None
    }
}

fn parse_powershell_invocation(executable: &str, args: &[String]) -> Option<Vec<Vec<String>>> {
    if args.is_empty() {
        return None;
    }
    let mut idx = 0;
    while idx < args.len() {
        let arg = &args[idx];
        let lower = arg.to_ascii_lowercase();
        match lower.as_str() {
            "-command" | "/command" | "-c" => {
                let script = args.get(idx + 1)?;
                if idx + 2 != args.len() {
                    return None;
                }
                return parse_powershell_script(executable, script);
            }
            _ if lower.starts_with("-command:") || lower.starts_with("/command:") => {
                if idx + 1 != args.len() {
                    return None;
                }
                let script = arg.split_once(':')?.1;
                return parse_powershell_script(executable, script);
            }
            "-nologo" | "-noprofile" | "-noninteractive" | "-mta" | "-sta" => {
                idx += 1;
                continue;
            }
            "-encodedcommand" | "-ec" | "-file" | "/file" | "-windowstyle" | "-executionpolicy"
            | "-workingdirectory" => {
                return None;
            }
            _ if lower.starts_with('-') => {
                return None;
            }
            _ => {
                let script = join_arguments_as_script(&args[idx..]);
                return parse_powershell_script(executable, &script);
            }
        }
    }
    None
}

fn parse_powershell_script(executable: &str, script: &str) -> Option<Vec<Vec<String>>> {
    if let PowershellParseOutcome::Commands(commands) = parse_with_powershell_ast(executable, script)
    {
        Some(commands)
    } else {
        None
    }
}

fn is_powershell_executable(exe: &str) -> bool {
    let executable_name = Path::new(exe)
        .file_name()
        .and_then(|osstr| osstr.to_str())
        .unwrap_or(exe)
        .to_ascii_lowercase();
    matches!(
        executable_name.as_str(),
        "powershell" | "powershell.exe" | "pwsh" | "pwsh.exe"
    )
}

fn join_arguments_as_script(args: &[String]) -> String {
    let mut words = Vec::with_capacity(args.len());
    if let Some((first, rest)) = args.split_first() {
        words.push(first.clone());
        for arg in rest {
            words.push(quote_argument(arg));
        }
    }
    words.join(" ")
}

fn quote_argument(arg: &str) -> String {
    if arg.is_empty() {
        return "''".to_string();
    }
    if arg.chars().all(|ch| !ch.is_whitespace()) {
        return arg.to_string();
    }
    format!("'{}'", arg.replace('\'', "''"))
}

pub(crate) fn is_safe_powershell_words(words: &[String]) -> bool {
    if words.is_empty() {
        return false;
    }
    for w in words.iter() {
        let inner = w
            .trim_matches(|c| c == '(' || c == ')')
            .trim_start_matches('-')
            .to_ascii_lowercase();
        if matches!(
            inner.as_str(),
            "set-content"
                | "add-content"
                | "out-file"
                | "new-item"
                | "remove-item"
                | "move-item"
                | "copy-item"
                | "rename-item"
                | "start-process"
                | "stop-process"
        ) {
            return false;
        }
    }
    let command = words[0]
        .trim_matches(|c| c == '(' || c == ')')
        .trim_start_matches('-')
        .to_ascii_lowercase();
    match command.as_str() {
        "echo" | "write-output" | "write-host" => true,
        "dir" | "ls" | "get-childitem" | "gci" => true,
        "cat" | "type" | "gc" | "get-content" => true,
        "select-string" | "sls" | "findstr" => true,
        "measure-object" | "measure" => true,
        "get-location" | "gl" | "pwd" => true,
        "test-path" | "tp" => true,
        "resolve-path" | "rvpa" => true,
        "select-object" | "select" => true,
        "get-item" => true,
        "git" => crate::safety::safe::is_safe_git_command(words),
        "rg" => is_safe_ripgrep(words),
        "set-content" | "add-content" | "out-file" | "new-item" | "remove-item" | "move-item"
        | "copy-item" | "rename-item" | "start-process" | "stop-process" => false,
        _ => false,
    }
}

fn is_safe_ripgrep(words: &[String]) -> bool {
    const UNSAFE_WITH_ARGS: &[&str] = &["--pre", "--hostname-bin"];
    const UNSAFE_WITHOUT_ARGS: &[&str] = &["--search-zip", "-z"];
    !words.iter().skip(1).any(|arg| {
        let arg_lc = arg.to_ascii_lowercase();
        UNSAFE_WITHOUT_ARGS.contains(&arg_lc.as_str())
            || UNSAFE_WITH_ARGS
                .iter()
                .any(|opt| arg_lc == *opt || arg_lc.starts_with(&format!("{opt}=")))
    })
}

pub fn is_dangerous_command_windows(command: &[String]) -> bool {
    if is_dangerous_powershell(command) {
        return true;
    }
    if is_dangerous_cmd(command) {
        return true;
    }
    is_direct_gui_launch(command)
}

fn is_dangerous_powershell(command: &[String]) -> bool {
    let Some((exe, rest)) = command.split_first() else {
        return false;
    };
    if !is_powershell_executable(exe) {
        return false;
    }
    let Some(parsed) = parse_powershell_invocation_dangerous(rest) else {
        return false;
    };
    is_dangerous_powershell_words(&parsed)
}

pub(crate) fn is_dangerous_powershell_words(words: &[String]) -> bool {
    let tokens_lc: Vec<String> = words
        .iter()
        .map(|t| t.trim_matches(['\'', '"']).to_ascii_lowercase())
        .collect();
    let has_url = args_have_url(words);

    if has_url
        && tokens_lc.iter().any(|t| {
            matches!(
                t.as_str(),
                "start-process" | "start" | "saps" | "invoke-item" | "ii"
            ) || t.contains("start-process")
                || t.contains("invoke-item")
        })
    {
        return true;
    }
    if has_url
        && tokens_lc
            .iter()
            .any(|t| t.contains("shellexecute") || t.contains("shell.application"))
    {
        return true;
    }
    if let Some(first) = tokens_lc.first() {
        if first == "rundll32"
            && tokens_lc
                .iter()
                .any(|t| t.contains("url.dll,fileprotocolhandler"))
            && has_url
        {
            return true;
        }
        if first == "mshta" && has_url {
            return true;
        }
        if is_browser_executable(first) && has_url {
            return true;
        }
        if matches!(first.as_str(), "explorer" | "explorer.exe") && has_url {
            return true;
        }
    }
    has_force_delete_cmdlet(&tokens_lc)
}

fn parse_powershell_invocation_dangerous(args: &[String]) -> Option<Vec<String>> {
    if args.is_empty() {
        return None;
    }
    let mut idx = 0;
    while idx < args.len() {
        let arg = &args[idx];
        let lower = arg.to_ascii_lowercase();
        match lower.as_str() {
            "-command" | "/command" | "-c" => {
                let script = args.get(idx + 1)?;
                if idx + 2 != args.len() {
                    return None;
                }
                let tokens = shlex::split(script)?;
                return Some(tokens);
            }
            _ if lower.starts_with("-command:") || lower.starts_with("/command:") => {
                if idx + 1 != args.len() {
                    return None;
                }
                let (_, script) = arg.split_once(':')?;
                let tokens = shlex::split(script)?;
                return Some(tokens);
            }
            "-nologo" | "-noprofile" | "-noninteractive" | "-mta" | "-sta" => {
                idx += 1;
            }
            _ if lower.starts_with('-') => {
                idx += 1;
            }
            _ => {
                return Some(args[idx..].to_vec());
            }
        }
    }
    None
}

fn is_dangerous_cmd(command: &[String]) -> bool {
    let Some((exe, rest)) = command.split_first() else {
        return false;
    };
    let Some(base) = executable_basename(exe) else {
        return false;
    };
    if base != "cmd" && base != "cmd.exe" {
        return false;
    }
    let mut iter = rest.iter();
    let mut reached_command = false;
    for arg in iter.by_ref() {
        let lower = arg.to_ascii_lowercase();
        match lower.as_str() {
            "/c" | "/r" | "-c" => {
                reached_command = true;
                break;
            }
            _ if lower.starts_with('/') => continue,
            _ => return false,
        }
    }
    if !reached_command {
        return false;
    }
    let remaining: Vec<String> = iter.cloned().collect();
    if remaining.is_empty() {
        return false;
    }
    let cmd_tokens: Vec<String> = match remaining.as_slice() {
        [only] => shlex::split(only).unwrap_or_else(|| vec![only.clone()]),
        _ => remaining,
    };
    let tokens: Vec<String> = cmd_tokens
        .into_iter()
        .flat_map(|t| split_embedded_cmd_operators(&t))
        .collect();

    const CMD_SEPARATORS: &[&str] = &["&", "&&", "|", "||"];
    tokens
        .split(|t| CMD_SEPARATORS.contains(&t.as_str()))
        .any(|segment| {
            let Some(cmd) = segment.first() else {
                return false;
            };
            if cmd.eq_ignore_ascii_case("start") && args_have_url(segment) {
                return true;
            }
            if (cmd.eq_ignore_ascii_case("del") || cmd.eq_ignore_ascii_case("erase"))
                && has_force_flag_cmd(segment)
            {
                return true;
            }
            if (cmd.eq_ignore_ascii_case("rd") || cmd.eq_ignore_ascii_case("rmdir"))
                && has_recursive_flag_cmd(segment)
                && has_quiet_flag_cmd(segment)
            {
                return true;
            }
            false
        })
}

fn is_direct_gui_launch(command: &[String]) -> bool {
    let Some((exe, rest)) = command.split_first() else {
        return false;
    };
    let Some(base) = executable_basename(exe) else {
        return false;
    };
    if matches!(base.as_str(), "explorer" | "explorer.exe") && args_have_url(rest) {
        return true;
    }
    if matches!(base.as_str(), "mshta" | "mshta.exe") && args_have_url(rest) {
        return true;
    }
    if (base == "rundll32" || base == "rundll32.exe")
        && rest.iter().any(|t| {
            t.to_ascii_lowercase()
                .contains("url.dll,fileprotocolhandler")
        })
        && args_have_url(rest)
    {
        return true;
    }
    if is_browser_executable(&base) && args_have_url(rest) {
        return true;
    }
    false
}

fn split_embedded_cmd_operators(token: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0;
    let mut it = token.char_indices().peekable();
    while let Some((i, ch)) = it.next() {
        if ch == '&' || ch == '|' {
            if i > start {
                parts.push(token[start..i].to_string());
            }
            let op_len = match it.peek() {
                Some(&(j, next)) if next == ch => {
                    it.next();
                    (j + next.len_utf8()) - i
                }
                _ => ch.len_utf8(),
            };
            parts.push(token[i..i + op_len].to_string());
            start = i + op_len;
        }
    }
    if start < token.len() {
        parts.push(token[start..].to_string());
    }
    parts.retain(|s| !s.trim().is_empty());
    parts
}

fn has_force_delete_cmdlet(tokens: &[String]) -> bool {
    const DELETE_CMDLETS: &[&str] = &["remove-item", "ri", "rm", "del", "erase", "rd", "rmdir"];
    const SEG_SEPS: &[char] = &[';', '|', '&', '\n', '\r', '\t'];
    const SOFT_SEPS: &[char] = &['{', '}', '(', ')', '[', ']', ',', ';'];

    let mut segments: Vec<Vec<String>> = vec![Vec::new()];
    for tok in tokens {
        let mut cur = String::new();
        for ch in tok.chars() {
            if SEG_SEPS.contains(&ch) {
                let s = cur.trim();
                if !s.is_empty() {
                    if let Some(last) = segments.last_mut() {
                        last.push(s.to_string());
                    }
                }
                cur.clear();
                if let Some(last) = segments.last() {
                    if !last.is_empty() {
                        segments.push(Vec::new());
                    }
                }
            } else {
                cur.push(ch);
            }
        }
        let s = cur.trim();
        if !s.is_empty() {
            if let Some(segment) = segments.last_mut() {
                segment.push(s.to_string());
            }
        }
    }

    segments.into_iter().any(|seg| {
        let atoms: Vec<String> = seg
            .iter()
            .flat_map(|t| t.split(|c| SOFT_SEPS.contains(&c)))
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(|s| s.to_string())
            .collect();

        let mut has_delete = false;
        let mut has_force = false;
        for a in atoms {
            if DELETE_CMDLETS.iter().any(|cmd| a.eq_ignore_ascii_case(cmd)) {
                has_delete = true;
            }
            if a.eq_ignore_ascii_case("-force")
                || a.get(..7).is_some_and(|p| p.eq_ignore_ascii_case("-force:"))
            {
                has_force = true;
            }
        }
        has_delete && has_force
    })
}

fn has_force_flag_cmd(args: &[String]) -> bool {
    args.iter().any(|a| a.eq_ignore_ascii_case("/f"))
}

fn has_recursive_flag_cmd(args: &[String]) -> bool {
    args.iter().any(|a| a.eq_ignore_ascii_case("/s"))
}

fn has_quiet_flag_cmd(args: &[String]) -> bool {
    args.iter().any(|a| a.eq_ignore_ascii_case("/q"))
}

fn args_have_url(args: &[String]) -> bool {
    args.iter().any(|arg| looks_like_url(arg))
}

fn looks_like_url(token: &str) -> bool {
    static RE: once_cell::sync::Lazy<Option<regex::Regex>> = once_cell::sync::Lazy::new(|| {
        regex::Regex::new(r#"^[ "'\(\s]*([^\s"'\);]+)[\s;\)]*$"#).ok()
    });
    let lowercase_token = token.to_ascii_lowercase();
    let urlish = lowercase_token
        .find("https://")
        .or_else(|| lowercase_token.find("http://"))
        .map(|idx| &token[idx..])
        .unwrap_or(token);
    let candidate = RE
        .as_ref()
        .and_then(|re| re.captures(urlish))
        .and_then(|caps| caps.get(1))
        .map(|m| m.as_str())
        .unwrap_or(urlish);
    let Ok(url) = url::Url::parse(candidate) else {
        return false;
    };
    matches!(url.scheme(), "http" | "https")
}

fn executable_basename(exe: &str) -> Option<String> {
    Path::new(exe)
        .file_name()
        .and_then(|osstr| osstr.to_str())
        .map(str::to_ascii_lowercase)
}

fn is_browser_executable(name: &str) -> bool {
    matches!(
        name,
        "chrome"
            | "chrome.exe"
            | "msedge"
            | "msedge.exe"
            | "firefox"
            | "firefox.exe"
            | "iexplore"
            | "iexplore.exe"
    )
}

// ---------------------------------------------------------------------------
// PowerShell AST parser subprocess (Codex powershell_parser.rs port)
// ---------------------------------------------------------------------------

use base64::engine::general_purpose::STANDARD as BASE64_STANDARD;
use base64::Engine;
use serde::Deserialize;
use serde::Serialize;
use std::collections::HashMap;
use std::io::{BufRead, BufReader, ErrorKind, Write};
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};
use std::sync::{LazyLock, Mutex, PoisonError};

const POWERSHELL_PARSER_SCRIPT: &str = include_str!("powershell_parser.ps1");

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum PowershellParseOutcome {
    Commands(Vec<Vec<String>>),
    Unsupported,
    Failed,
}

fn parse_with_powershell_ast(executable: &str, script: &str) -> PowershellParseOutcome {
    static PARSER_PROCESSES: LazyLock<Mutex<HashMap<String, PowershellParserProcess>>> =
        LazyLock::new(|| Mutex::new(HashMap::new()));
    let mut parser_processes = PARSER_PROCESSES
        .lock()
        .unwrap_or_else(PoisonError::into_inner);
    parse_with_cached_process(&mut parser_processes, executable, script)
}

fn encode_powershell_base64(script: &str) -> String {
    let mut utf16 = Vec::with_capacity(script.len() * 2);
    for unit in script.encode_utf16() {
        utf16.extend_from_slice(&unit.to_le_bytes());
    }
    BASE64_STANDARD.encode(utf16)
}

fn encoded_parser_script() -> &'static str {
    static ENCODED: LazyLock<String> =
        LazyLock::new(|| encode_powershell_base64(POWERSHELL_PARSER_SCRIPT));
    &ENCODED
}

struct PowershellParserProcess {
    child: Child,
    stdin: ChildStdin,
    stdout: BufReader<ChildStdout>,
    next_request_id: u64,
}

impl PowershellParserProcess {
    fn spawn(executable: &str) -> std::io::Result<Self> {
        let mut command = Command::new(executable);
        command
            .args([
                "-NoLogo",
                "-NoProfile",
                "-NonInteractive",
                "-EncodedCommand",
                encoded_parser_script(),
            ])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null());
        let mut child = command.spawn()?;
        let stdin = match child.stdin.take() {
            Some(stdin) => stdin,
            None => {
                let _ = child.kill();
                let _ = child.wait();
                return Err(std::io::Error::new(
                    ErrorKind::BrokenPipe,
                    "PowerShell parser child did not expose stdin",
                ));
            }
        };
        let stdout = match child.stdout.take() {
            Some(stdout) => BufReader::new(stdout),
            None => {
                let _ = child.kill();
                let _ = child.wait();
                return Err(std::io::Error::new(
                    ErrorKind::BrokenPipe,
                    "PowerShell parser child did not expose stdout",
                ));
            }
        };
        Ok(Self {
            child,
            stdin,
            stdout,
            next_request_id: 0,
        })
    }

    fn parse(&mut self, script: &str) -> std::io::Result<PowershellParseOutcome> {
        let request = PowershellParserRequest {
            id: self.next_request_id,
            payload: encode_powershell_base64(script),
        };
        self.next_request_id = self.next_request_id.wrapping_add(1);
        let mut request_json = serde_json::to_string(&request).map_err(|error| {
            std::io::Error::new(
                ErrorKind::InvalidData,
                format!("failed to serialize PowerShell parser request: {error}"),
            )
        })?;
        request_json.push('\n');
        self.stdin.write_all(request_json.as_bytes())?;
        self.stdin.flush()?;

        let mut response_line = String::new();
        if self.stdout.read_line(&mut response_line)? == 0 {
            return Err(std::io::Error::new(
                ErrorKind::UnexpectedEof,
                "PowerShell parser closed stdout",
            ));
        }
        let response: PowershellParserResponse =
            serde_json::from_str(&response_line).map_err(|error| {
                std::io::Error::new(
                    ErrorKind::InvalidData,
                    format!("failed to parse PowerShell parser response: {error}"),
                )
            })?;
        if response.id != request.id {
            return Err(std::io::Error::new(
                ErrorKind::InvalidData,
                format!(
                    "PowerShell parser returned response id {} for request {}",
                    response.id, request.id
                ),
            ));
        }
        Ok(response.into_outcome())
    }
}

impl Drop for PowershellParserProcess {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

fn parse_with_cached_process(
    parser_processes: &mut HashMap<String, PowershellParserProcess>,
    executable: &str,
    script: &str,
) -> PowershellParseOutcome {
    let parser_key = executable.to_string();
    for attempt in 0..=1 {
        if !parser_processes.contains_key(&parser_key) {
            match PowershellParserProcess::spawn(executable) {
                Ok(process) => {
                    parser_processes.insert(parser_key.clone(), process);
                }
                Err(_) => return PowershellParseOutcome::Failed,
            }
        }
        let Some(parser_process) = parser_processes.get_mut(&parser_key) else {
            return PowershellParseOutcome::Failed;
        };
        match parser_process.parse(script) {
            Ok(outcome) => return outcome,
            Err(_) if attempt == 0 => {
                parser_processes.remove(&parser_key);
            }
            Err(_) => return PowershellParseOutcome::Failed,
        }
    }
    PowershellParseOutcome::Failed
}

#[derive(Serialize)]
struct PowershellParserRequest {
    id: u64,
    payload: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct PowershellParserResponse {
    id: u64,
    status: String,
    commands: Option<Vec<Vec<String>>>,
}

impl PowershellParserResponse {
    fn into_outcome(self) -> PowershellParseOutcome {
        match self.status.as_str() {
            "ok" => self
                .commands
                .filter(|commands| {
                    !commands.is_empty()
                        && commands
                            .iter()
                            .all(|cmd| !cmd.is_empty() && cmd.iter().all(|word| !word.is_empty()))
                })
                .map(PowershellParseOutcome::Commands)
                .unwrap_or(PowershellParseOutcome::Unsupported),
            "unsupported" => PowershellParseOutcome::Unsupported,
            _ => PowershellParseOutcome::Failed,
        }
    }
}
