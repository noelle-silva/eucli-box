//! command-analyzer: unified shell command analyzer for the shell_command tool.
//!
//! Reads one AnalyzeRequest (JSON) from stdin and writes one AnalyzeResponse
//! (JSON) to stdout. The analysis merges the reference implementations:
//! - Codex shell-command safety core (safe/dangerous classification)
//! - Claude Code AST walk + semantics + read-only flag tables + path extractors
//! - OpenCode impact scan (file commands, workdir boundaries, BashArity)

mod analyzer;
mod bash;
mod bash_walk;
mod impact;
mod powershell;
mod protocol;
mod readonly;
mod readonly_simple;
mod safety;
mod semantics;
mod shell_type;

use std::io::Read;

fn main() {
    let mut payload = String::new();
    if std::io::stdin().read_to_string(&mut payload).is_err() {
        write_error("failed to read request");
        std::process::exit(1);
    }
    let request: protocol::AnalyzeRequest = match serde_json::from_str(payload.trim()) {
        Ok(req) => req,
        Err(err) => {
            write_error(&format!("invalid request: {err}"));
            std::process::exit(1);
        }
    };
    let response = analyzer::analyze_full(&request);
    match serde_json::to_string(&response) {
        Ok(out) => {
            println!("{out}");
        }
        Err(err) => {
            write_error(&format!("failed to serialize response: {err}"));
            std::process::exit(1);
        }
    }
}

fn write_error(message: &str) {
    eprintln!("command-analyzer: {message}");
}
