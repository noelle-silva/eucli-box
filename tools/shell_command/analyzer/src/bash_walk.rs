#![allow(dead_code)] // Ported-from-reference interfaces, some not yet wired into the protocol; kept for later stages.
//! AST-based walker ported from Claude Code `utils/bash/ast.ts`
//! (parseForSecurityFromAst / walkProgram / collectCommands / walkCommand /
//! walkArgument / walkString / redirect walkers / variable tracking).
//!
//! Design property: FAIL-CLOSED. Any tree-sitter node type not explicitly
//! handled makes the whole command "too complex". This module answers one
//! question: "Can we produce a trustworthy argv[] for each simple command?"
//!
//! Port notes:
//! - tree-sitter node `text` spans are taken via utf8_text on the source.
//! - varScope is `HashMap<String,String>`; placeholders are the same strings
//!   as Claude (`__CMDSUB_OUTPUT__`, `__TRACKED_VAR__`).
//! - The walker works on u8 byte offsets directly (Rust &str slicing is safe
//!   because tree-sitter offsets are byte offsets on the same UTF-8 string).

use std::collections::{HashMap, HashSet};

use once_cell::sync::Lazy;
use regex::Regex;
use tree_sitter::Node;

use crate::protocol::{InternalCommand, InternalRedirect};

pub const CMDSUB_PLACEHOLDER: &str = "__CMDSUB_OUTPUT__";
pub const VAR_PLACEHOLDER: &str = "__TRACKED_VAR__";

fn contains_any_placeholder(value: &str) -> bool {
    value.contains(CMDSUB_PLACEHOLDER) || value.contains(VAR_PLACEHOLDER)
}

fn strip_raw_string(text: &str) -> &str {
    &text[1..text.len().saturating_sub(1)]
}

static BRACE_EXPANSION_RE: Lazy<Regex> =
    Lazy::new(|| Regex::new(r"\{[^{}\s]*(,|\.\.)[^{}\s]*\}").unwrap());
static CONTROL_CHAR_RE: Lazy<Regex> =
    Lazy::new(|| Regex::new(r"[\x00-\x08\x0B-\x1F\x7F]").unwrap());
static UNICODE_WHITESPACE_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"[\u00A0\u1680\u2000-\u200B\u2028\u2029\u202F\u205F\u3000\uFEFF]").unwrap()
});
static BACKSLASH_WHITESPACE_RE: Lazy<Regex> =
    Lazy::new(|| Regex::new(r"\\[ \t]|[^ \t\n\\]\\\n").unwrap());
static ZSH_TILDE_BRACKET_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"~\[").unwrap());
static ZSH_EQUALS_EXPANSION_RE: Lazy<Regex> =
    Lazy::new(|| Regex::new(r"(?:^|[\s;&|])=[a-zA-Z_]").unwrap());
static BRACE_WITH_QUOTE_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r#"\{[^}]*['\"]"#).unwrap());
static PROC_ENVIRON_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"/proc/.*/environ").unwrap());
static NEWLINE_HASH_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"\n[ \t]*#").unwrap());
static PS4_SAFE_CHARSET_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"^[A-Za-z0-9 _+:./=\[\]-]*$").unwrap());
static PS4_REF_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"\$\{[A-Za-z_][A-Za-z0-9_]*\}").unwrap());
static ARITH_LEAF_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(
        r"^(?:[0-9]+|0[xX][0-9a-fA-F]+|[0-9]+#[0-9a-zA-Z]+|[-+*/%^&|~!<>=?:(),]+|<<|>>|\*\*|&&|\|\||[<>=!]=|\$\(\(|\)\))$",
    )
    .unwrap()
});

fn safe_env_vars() -> &'static HashSet<&'static str> {
    static SET: Lazy<HashSet<&'static str>> = Lazy::new(|| {
        [
            "HOME", "PWD", "OLDPWD", "USER", "LOGNAME", "SHELL", "PATH", "HOSTNAME", "UID",
            "EUID", "PPID", "RANDOM", "SECONDS", "LINENO", "TMPDIR", "BASH_VERSION", "BASHPID",
            "SHLVL", "HISTFILE", "IFS",
        ]
        .into_iter()
        .collect()
    });
    &SET
}

fn special_var_names() -> &'static HashSet<&'static str> {
    static SET: Lazy<HashSet<&'static str>> =
        Lazy::new(|| ["?", "$", "!", "#", "0", "-"].into_iter().collect());
    &SET
}

#[derive(Debug)]
pub enum WalkError {
    TooComplex { reason: String },
}

impl WalkError {
    fn too_complex(node: Node) -> Self {
        WalkError::TooComplex {
            reason: format!("Unhandled node type: {}", node.kind()),
        }
    }

    fn with_reason(reason: impl Into<String>) -> Self {
        WalkError::TooComplex {
            reason: reason.into(),
        }
    }
}

pub type WalkResult<T> = Result<T, WalkError>;

fn too_complex(node: Node, src: &str) -> WalkError {
    let _ = src;
    WalkError::too_complex(node)
}

struct VarScope(HashMap<String, String>);

impl VarScope {
    fn new() -> Self {
        VarScope(HashMap::new())
    }

    fn clone_scope(&self) -> VarScope {
        VarScope(self.0.clone())
    }
}

/// Pre-checks from parseForSecurityFromAst. Returns Err(reason) when the
/// command contains a tree-sitter/bash differential.
pub fn precheck(command: &str) -> Result<(), String> {
    if CONTROL_CHAR_RE.is_match(command) {
        return Err("Contains control characters".into());
    }
    if UNICODE_WHITESPACE_RE.is_match(command) {
        return Err("Contains Unicode whitespace".into());
    }
    if BACKSLASH_WHITESPACE_RE.is_match(command) {
        return Err("Contains backslash-escaped whitespace".into());
    }
    if ZSH_TILDE_BRACKET_RE.is_match(command) {
        return Err("Contains zsh ~[ dynamic directory syntax".into());
    }
    if ZSH_EQUALS_EXPANSION_RE.is_match(command) {
        return Err("Contains zsh =cmd equals expansion".into());
    }
    if BRACE_WITH_QUOTE_RE.is_match(&mask_braces_in_quoted_contexts(command)) {
        return Err("Contains brace with quote character (expansion obfuscation)".into());
    }
    Ok(())
}

fn mask_braces_in_quoted_contexts(cmd: &str) -> String {
    if !cmd.contains('{') {
        return cmd.to_string();
    }
    let mut out: Vec<char> = Vec::with_capacity(cmd.len());
    let mut in_single = false;
    let mut in_double = false;
    let chars: Vec<char> = cmd.chars().collect();
    let mut i = 0;
    while i < chars.len() {
        let c = chars[i];
        if in_single {
            if c == '\'' {
                in_single = false;
            }
            out.push(if c == '{' { ' ' } else { c });
            i += 1;
        } else if in_double {
            if c == '\\' && i + 1 < chars.len() && (chars[i + 1] == '"' || chars[i + 1] == '\\') {
                out.push(c);
                out.push(chars[i + 1]);
                i += 2;
            } else {
                if c == '"' {
                    in_double = false;
                }
                out.push(if c == '{' { ' ' } else { c });
                i += 1;
            }
        } else {
            if c == '\\' && i + 1 < chars.len() {
                out.push(c);
                out.push(chars[i + 1]);
                i += 2;
            } else {
                if c == '\'' {
                    in_single = true;
                } else if c == '"' {
                    in_double = true;
                }
                out.push(c);
                i += 1;
            }
        }
    }
    out.into_iter().collect()
}

/// Walk a parsed tree and return a flat list of simple commands
/// (fail-closed). Returns Err(reason) on first too-complex hit.
pub fn parse_for_security(tree: &tree_sitter::Tree, src: &str) -> WalkResult<Vec<InternalCommand>> {
    let mut commands = Vec::new();
    let mut var_scope = VarScope::new();
    collect_commands(tree.root_node(), &mut commands, &mut var_scope, src)?;
    Ok(commands)
}

fn collect_commands(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    if node.has_error() {
        return Err(WalkError::with_reason("Parse error"));
    }

    match node.kind() {
        "command" => {
            let result = walk_command(node, &mut Vec::new(), commands, var_scope, src)?;
            commands.extend(result);
            Ok(())
        }
        "redirected_statement" => walk_redirected_statement(node, commands, var_scope, src),
        "comment" => Ok(()),
        kind if is_structural(kind) => {
            // Claude's scope discipline: `&&`/`;` chains share state; `||`,
            // `|`, `|&`, `&` reset to the entry snapshot; pipeline stages all
            // start from an entry copy so vars set in a stage never leak to
            // the caller or another stage.
            let is_pipeline = node.kind() == "pipeline";
            let mut needs_snapshot = false;
            if !is_pipeline {
                let mut cursor = node.walk();
                for c in node.children(&mut cursor) {
                    if c.kind() == "||" || c.kind() == "&" {
                        needs_snapshot = true;
                        break;
                    }
                }
            }
            let snapshot = if needs_snapshot {
                Some(var_scope.clone_scope())
            } else {
                None
            };
            let mut working = var_scope.clone_scope();
            let mut cursor = node.walk();
            let child_nodes: Vec<Node> = node.children(&mut cursor).collect();
            for child in child_nodes {
                if is_separator(child.kind()) {
                    if matches!(child.kind(), "||" | "|" | "|&" | "&") {
                        working = snapshot
                            .as_ref()
                            .map(|s| s.clone_scope())
                            .unwrap_or_else(|| var_scope.clone_scope());
                    }
                    continue;
                }
                collect_commands(child, commands, &mut working, src)?;
            }
            if !is_pipeline {
                *var_scope = working;
            }
            Ok(())
        }
        "negated_command" => {
            let mut cursor = node.walk();
            for child in node.children(&mut cursor) {
                if child.kind() == "!" {
                    continue;
                }
                return collect_commands(child, commands, var_scope, src);
            }
            Ok(())
        }
        "declaration_command" => {
            let argv: Vec<String> = Vec::new();
            walk_declaration_command(node, argv, commands, var_scope, src)
        }
        "variable_assignment" => {
            let ev = walk_variable_assignment(node, commands, var_scope, src)?;
            apply_var_to_scope(var_scope, &ev.name, &ev.value, ev.is_append);
            Ok(())
        }
        "for_statement" => walk_for_statement(node, commands, var_scope, src),
        "if_statement" | "while_statement" | "until_statement" => {
            walk_if_while(node, commands, var_scope, src)
        }
        "subshell" => {
            let mut inner_scope = var_scope.clone_scope();
            let mut cursor = node.walk();
            for child in node.children(&mut cursor) {
                if child.kind() == "(" || child.kind() == ")" {
                    continue;
                }
                collect_commands(child, commands, &mut inner_scope, src)?;
            }
            Ok(())
        }
        "test_command" => walk_test_command(node, commands, var_scope, src),
        "unset_command" => walk_unset_command(node, commands, var_scope, src),
        _ => Err(too_complex(node, src)),
    }
}

fn is_structural(kind: &str) -> bool {
    matches!(kind, "program" | "list" | "pipeline" | "redirected_statement")
}

fn is_separator(kind: &str) -> bool {
    matches!(kind, "&&" | "||" | "|" | ";" | "&" | "|&" | "\n")
}

fn walk_declaration_command(
    node: Node,
    mut argv: Vec<String>,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut cursor = node.walk();
    for child in node.named_children(&mut cursor) {
        match child.kind() {
            "export" | "local" | "readonly" | "declare" | "typeset" => {
                argv.push(child_text(child, src));
            }
            "word" | "number" | "raw_string" | "string" | "concatenation" => {
                let arg = walk_argument(child, commands, var_scope, src)?;
                if (argv.first().map(String::as_str) == Some("declare")
                    || argv.first().map(String::as_str) == Some("typeset")
                    || argv.first().map(String::as_str) == Some("local"))
                    && Regex::new(r"^-[a-zA-Z]*[niaA]").unwrap().is_match(&arg)
                {
                    return Err(WalkError::with_reason(
                        "declare flag changes assignment semantics (nameref/integer/array)",
                    ));
                }
                if (argv.first().map(String::as_str) == Some("declare")
                    || argv.first().map(String::as_str) == Some("typeset")
                    || argv.first().map(String::as_str) == Some("local"))
                    && !arg.starts_with('-')
                    && Regex::new(r"^[^=]*\[").unwrap().is_match(&arg)
                {
                    return Err(WalkError::with_reason(
                        "declare positional contains array subscript — bash evaluates $(cmd) in subscripts",
                    ));
                }
                argv.push(arg);
            }
            "variable_assignment" => {
                let ev = walk_variable_assignment(child, commands, var_scope, src)?;
                apply_var_to_scope(var_scope, &ev.name, &ev.value, ev.is_append);
                argv.push(format!("{}={}", ev.name, ev.value));
            }
            "variable_name" => {
                argv.push(child_text(child, src));
            }
            _ => return Err(too_complex(child, src)),
        }
    }
    commands.push(InternalCommand {
        argv,
        text: node_text(node, src),
        env_vars: Vec::new(),
        redirects: Vec::new(),
        dynamic: false,
    });
    Ok(())
}

fn walk_for_statement(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut loop_var: Option<String> = None;
    let mut do_group: Option<Node> = None;
    let mut cursor = node.walk();
    for child in node.named_children(&mut cursor) {
        match child.kind() {
            "variable_name" => loop_var = Some(child_text(child, src)),
            "do_group" => do_group = Some(child),
            "for" | "in" | "select" | ";" => continue,
            "command_substitution" => {
                collect_command_substitution(child, commands, var_scope, src)?;
            }
            _ => {
                walk_argument(child, commands, var_scope, src)?;
            }
        }
    }
    let (Some(loop_var), Some(do_group)) = (loop_var, do_group) else {
        return Err(too_complex(node, src));
    };
    if loop_var == "PS4" || loop_var == "IFS" {
        return Err(WalkError::with_reason(
            "variable as loop variable bypasses assignment validation",
        ));
    }
    var_scope.0.insert(loop_var.clone(), VAR_PLACEHOLDER.to_string());
    let mut body_scope = var_scope.clone_scope();
    let mut cursor = do_group.walk();
    for c in do_group.children(&mut cursor) {
        if matches!(c.kind(), "do" | "done" | ";") {
            continue;
        }
        collect_commands(c, commands, &mut body_scope, src)?;
    }
    Ok(())
}

fn walk_if_while(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut seen_then = false;
    let mut cursor = node.walk();
    for child in node.named_children(&mut cursor) {
        match child.kind() {
            "if" | "fi" | "else" | "elif" | "while" | "until" | ";" => continue,
            "then" => {
                seen_then = true;
                continue;
            }
            "do_group" => {
                let mut body_scope = var_scope.clone_scope();
                let mut c = child.walk();
                for cc in child.children(&mut c) {
                    if matches!(cc.kind(), "do" | "done" | ";") {
                        continue;
                    }
                    collect_commands(cc, commands, &mut body_scope, src)?;
                }
                continue;
            }
            "elif_clause" | "else_clause" => {
                let mut branch_scope = var_scope.clone_scope();
                let mut c = child.walk();
                for cc in child.children(&mut c) {
                    if matches!(cc.kind(), "elif" | "else" | "then" | ";") {
                        continue;
                    }
                    collect_commands(cc, commands, &mut branch_scope, src)?;
                }
                continue;
            }
            _ => {
                let mut target_scope;
                let scope: &mut VarScope = if seen_then {
                    target_scope = var_scope.clone_scope();
                    &mut target_scope
                } else {
                    var_scope
                };
                let before = commands.len();
                collect_commands(child, commands, scope, src)?;
                if !seen_then {
                    for i in before..commands.len() {
                        let c = &commands[i];
                        if c.argv.first().map(String::as_str) == Some("read") {
                            let mut read_vars: Vec<String> = Vec::new();
                            for a in c.argv.iter().skip(1) {
                                if !a.starts_with('-')
                                    && Regex::new(r"^[A-Za-z_][A-Za-z0-9_]*$")
                                        .unwrap()
                                        .is_match(a)
                                {
                                    read_vars.push(a.clone());
                                }
                            }
                            for a in read_vars {
                                if let Some(existing) = var_scope.0.get(&a.clone()) {
                                    if !contains_any_placeholder(existing) {
                                        return Err(WalkError::with_reason(
                                            "'read' in condition may not execute; cannot prove it overwrites tracked literal",
                                        ));
                                    }
                                }
                                var_scope.0.insert(a, VAR_PLACEHOLDER.to_string());
                            }
                        }
                    }
                }
            }
        }
    }
    Ok(())
}

fn walk_test_command(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut argv = vec!["[[".to_string()];
    let mut cursor = node.walk();
    for child in node.children(&mut cursor) {
        match child.kind() {
            "[[" | "]]" | "[" | "]" => continue,
            _ => walk_test_expr(child, &mut argv, commands, var_scope, src)?,
        }
    }
    commands.push(InternalCommand {
        argv,
        text: node_text(node, src),
        env_vars: Vec::new(),
        redirects: Vec::new(),
        dynamic: false,
    });
    Ok(())
}

fn walk_test_expr(
    node: Node,
    argv: &mut Vec<String>,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    match node.kind() {
        "unary_expression" | "binary_expression" | "negated_expression"
        | "parenthesized_expression" => {
            let children: Vec<Node> = {
                let mut cursor = node.walk();
                node.children(&mut cursor).collect()
            };
            for c in children {
                walk_test_expr(c, argv, commands, var_scope, src)?;
            }
            Ok(())
        }
        "test_operator" | "!" | "(" | ")" | "&&" | "||" | "==" | "=" | "!=" | "<" | ">"
        | "=~" | "regex" | "extglob_pattern" => {
            argv.push(child_text(node, src));
            Ok(())
        }
        _ => {
            let arg = walk_argument(node, commands, var_scope, src)?;
            argv.push(arg);
            Ok(())
        }
    }
}

fn walk_unset_command(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut argv: Vec<String> = Vec::new();
    let mut cursor = node.walk();
    for child in node.named_children(&mut cursor) {
        match child.kind() {
            "unset" => argv.push(child_text(child, src)),
            "variable_name" => {
                let name = child_text(child, src);
                argv.push(name.clone());
                var_scope.0.remove(&name);
            }
            "word" => {
                let arg = walk_argument(child, commands, var_scope, src)?;
                argv.push(arg);
            }
            _ => return Err(too_complex(child, src)),
        }
    }
    commands.push(InternalCommand {
        argv,
        text: node_text(node, src),
        env_vars: Vec::new(),
        redirects: Vec::new(),
        dynamic: false,
    });
    Ok(())
}

fn walk_redirected_statement(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut redirects: Vec<InternalRedirect> = Vec::new();
    let mut inner_command: Option<Node> = None;
    let mut cursor = node.walk();
    for child in node.children(&mut cursor) {
        match child.kind() {
            "file_redirect" => {
                redirects.push(walk_file_redirect(child, commands, var_scope, src)?);
            }
            "heredoc_redirect" => {
                walk_heredoc_redirect(child, src)?;
            }
            "command" | "pipeline" | "list" | "negated_command" | "declaration_command"
            | "unset_command" => {
                inner_command = Some(child);
            }
            _ => return Err(too_complex(child, src)),
        }
    }

    let Some(inner_command) = inner_command else {
        commands.push(InternalCommand {
            argv: Vec::new(),
            text: node_text(node, src),
            env_vars: Vec::new(),
            redirects,
            dynamic: false,
        });
        return Ok(());
    };

    let before = commands.len();
    collect_commands(inner_command, commands, var_scope, src)?;
    if commands.len() > before && !redirects.is_empty() {
        if let Some(last) = commands.last_mut() {
            last.redirects.extend(redirects);
        }
    }
    Ok(())
}

fn walk_file_redirect(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<InternalRedirect> {
    let mut op: Option<String> = None;
    let mut target: Option<String> = None;
    let mut fd: Option<u32> = None;

    let mut cursor = node.walk();
    for child in node.children(&mut cursor) {
        match child.kind() {
            "file_descriptor" => {
                fd = child_text(child, src).parse().ok();
            }
            ">" | ">>" | "<" | ">&" | "<&" | ">|" | "&>" | "&>>" | "<<<" => {
                op = Some(child.kind().to_string());
            }
            "word" | "number" => {
                if child.named_child_count() > 0 {
                    return Err(WalkError::with_reason(
                        "Number node contains expansion (NN# arithmetic base syntax)",
                    ));
                }
                let text = child_text(child, src);
                if BRACE_EXPANSION_RE.is_match(&text) {
                    return Err(WalkError::with_reason(
                        "Redirect word contains brace expansion syntax",
                    ));
                }
                target = Some(Regex::new(r"\\(.)").unwrap().replace_all(&text, "$1").into_owned());
            }
            "raw_string" => {
                target = Some(strip_raw_string(&child_text(child, src)).to_string());
            }
            "string" => {
                let s = walk_string(child, commands, var_scope, src)?;
                target = Some(s);
            }
            "concatenation" => {
                let s = walk_argument(child, commands, var_scope, src)?;
                target = Some(s);
            }
            _ => return Err(too_complex(child, src)),
        }
    }

    let (Some(op), Some(target)) = (op, target) else {
        return Err(WalkError::with_reason("Unrecognized redirect shape"));
    };
    Ok(InternalRedirect {
        op,
        target,
        fd,
        dynamic: false,
    })
}

fn walk_heredoc_redirect(node: Node, src: &str) -> WalkResult<()> {
    let mut start_text: Option<String> = None;
    let mut body: Option<Node> = None;
    let mut cursor = node.walk();
    for child in node.children(&mut cursor) {
        match child.kind() {
            "heredoc_start" => start_text = Some(child_text(child, src)),
            "heredoc_body" => body = Some(child),
            "<<" | "<<-" | "heredoc_end" | "file_descriptor" => continue,
            _ => {
                return Err(WalkError::with_reason(
                    "Unsupported construct next to heredoc redirect",
                ))
            }
        }
    }

    let is_quoted = start_text
        .as_ref()
        .is_some_and(|t| {
            (t.starts_with('\'') && t.ends_with('\''))
                || (t.starts_with('"') && t.ends_with('"'))
                || t.starts_with('\\')
        });
    if !is_quoted {
        return Err(WalkError::with_reason(
            "Heredoc with unquoted delimiter undergoes shell expansion",
        ));
    }

    if let Some(body) = body {
        // Verify the body is pure heredoc_content by checking children of the
        // program child inside heredoc_body (Claude's check via children).
        let mut cursor = body.walk();
        for c in body.named_children(&mut cursor) {
            if c.kind() != "heredoc_content" {
                return Err(WalkError::with_reason(
                    "Heredoc body contains non-content node",
                ));
            }
        }
    }
    Ok(())
}

fn walk_herestring_redirect(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut cursor = node.walk();
    for child in node.children(&mut cursor) {
        if child.kind() == "<<<" {
            continue;
        }
        let content = walk_argument(child, commands, var_scope, src)?;
        if NEWLINE_HASH_RE.is_match(&content) {
            return Err(WalkError::with_reason(
                "Newline followed by # inside a herestring",
            ));
        }
    }
    Ok(())
}

fn walk_command(
    node: Node,
    extra_redirects: &mut Vec<InternalRedirect>,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<Vec<InternalCommand>> {
    let mut argv: Vec<String> = Vec::new();
    let mut env_vars: Vec<(String, String)> = Vec::new();
    let mut redirects: Vec<InternalRedirect> = extra_redirects.to_vec();

    let mut cursor = node.walk();
    for child in node.named_children(&mut cursor) {
        match child.kind() {
            "variable_assignment" => {
                let ev = walk_variable_assignment(child, commands, var_scope, src)?;
                env_vars.push((ev.name.clone(), ev.value.clone()));
                if ev.is_append {
                    return Err(WalkError::with_reason(
                        "Append assignment as env prefix cannot be statically modeled",
                    ));
                }
            }
            "command_name" => {
                let arg = walk_argument(
                    child.named_child(0).unwrap_or(child),
                    commands,
                    var_scope,
                    src,
                )?;
                argv.push(arg);
            }
            "word" | "number" | "raw_string" | "string" | "concatenation"
            | "arithmetic_expansion" => {
                let arg = walk_argument(child, commands, var_scope, src)?;
                argv.push(arg);
            }
            "simple_expansion" => {
                let v = resolve_simple_expansion(child, var_scope, false, src)?;
                argv.push(v);
            }
            "file_redirect" => {
                let r = walk_file_redirect(child, commands, var_scope, src)?;
                redirects.push(r);
            }
            "herestring_redirect" => {
                walk_herestring_redirect(child, commands, var_scope, src)?;
            }
            // NOTE: bare command_substitution as an argument is NOT handled
            // (Claude keeps it too-complex so paths can't be hidden).
            _ => return Err(too_complex(child, src)),
        }
    }

    let mut text = node_text(node, src);
    if Regex::new(r"\$[A-Za-z_]").unwrap().is_match(&text) || text.contains('\n') {
        text = argv
            .iter()
            .map(|a| {
                if a.is_empty()
                    || Regex::new(r#"["'\\ \t\n$`;|&<>(){}*?\[\]~#]"#)
                        .unwrap()
                        .is_match(a)
                {
                    format!("'{}'", a.replace('\'', "'\\''"))
                } else {
                    a.clone()
                }
            })
            .collect::<Vec<_>>()
            .join(" ");
    }

    let dynamic = argv
        .iter()
        .any(|a| contains_any_placeholder(a) || a.contains("$"));

    Ok(vec![InternalCommand {
        argv,
        text,
        env_vars,
        redirects,
        dynamic,
    }])
}

fn walk_argument(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<String> {
    match node.kind() {
        "word" => {
            let text = child_text(node, src);
            if BRACE_EXPANSION_RE.is_match(&text) {
                return Err(WalkError::with_reason("Word contains brace expansion syntax"));
            }
            Ok(Regex::new(r"\\(.)").unwrap().replace_all(&text, "$1").into_owned())
        }
        "number" => {
            if node.named_child_count() > 0 {
                return Err(WalkError::with_reason(
                    "Number node contains expansion (NN# arithmetic base syntax)",
                ));
            }
            Ok(child_text(node, src))
        }
        "raw_string" => Ok(strip_raw_string(&child_text(node, src)).to_string()),
        "string" => walk_string(node, commands, var_scope, src),
        "concatenation" => {
            let text = node_text(node, src);
            if BRACE_EXPANSION_RE.is_match(&text) {
                return Err(WalkError::with_reason("Brace expansion"));
            }
            let mut result = String::new();
            let mut cursor = node.walk();
            for child in node.named_children(&mut cursor) {
                result.push_str(&walk_argument(child, commands, var_scope, src)?);
            }
            Ok(result)
        }
        "arithmetic_expansion" => {
            walk_arithmetic(node, src)?;
            Ok(node_text(node, src))
        }
        "simple_expansion" | "expansion" if node.kind() == "simple_expansion" => {
            resolve_simple_expansion(node, var_scope, false, src)
        }
        _ => Err(too_complex(node, src)),
    }
}

fn walk_string(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<String> {
    let mut result = String::new();
    let mut cursor_byte: i64 = -1;
    let mut saw_dynamic = false;
    let mut saw_literal = false;

    let mut cursor = node.walk();
    for child in node.children(&mut cursor) {
        let start = child.start_byte() as i64;
        let end = child.end_byte() as i64;
        if cursor_byte != -1 && start > cursor_byte && child.kind() != "\"" {
            result.push_str(&"\n".repeat((start - cursor_byte) as usize));
            saw_literal = true;
        }
        cursor_byte = end;
        match child.kind() {
            "\"" => {
                cursor_byte = end;
            }
            "string_content" => {
                let text = child_text(child, src);
                result.push_str(
                    &Regex::new(r#"\\([$`"\\])"#).unwrap().replace_all(&text, "$1"),
                );
                saw_literal = true;
            }
            "$" => {
                result.push('$');
                saw_literal = true;
            }
            "command_substitution" => {
                let heredoc_body = extract_safe_cat_heredoc(child, src);
                if heredoc_body.as_deref() == Some("DANGEROUS") {
                    return Err(WalkError::with_reason(
                        "Command substitution heredoc contains dangerous content",
                    ));
                }
                if let Some(body) = heredoc_body {
                    let trimmed = body.trim_end_matches('\n').to_string();
                    if trimmed.contains('\n') {
                        saw_literal = true;
                    } else {
                        result.push_str(&trimmed);
                        saw_literal = true;
                    }
                    continue;
                }
                collect_command_substitution(child, commands, var_scope, src)?;
                result.push_str(CMDSUB_PLACEHOLDER);
                saw_dynamic = true;
            }
            "simple_expansion" => {
                let v = resolve_simple_expansion(child, var_scope, true, src)?;
                if v == VAR_PLACEHOLDER {
                    saw_dynamic = true;
                } else {
                    saw_literal = true;
                }
                result.push_str(&v);
            }
            "arithmetic_expansion" => {
                walk_arithmetic(child, src)?;
                result.push_str(&node_text(child, src));
                saw_literal = true;
            }
            _ => {
                return Err(WalkError::with_reason(format!(
                    "expansion inside double quotes: {}",
                    child.kind()
                )));
            }
        }
    }

    if saw_dynamic && !saw_literal {
        return Err(WalkError::with_reason(
            "String consists solely of a dynamic placeholder",
        ));
    }
    let raw = node_text(node, src);
    if !saw_literal && !saw_dynamic && raw.len() > 2 {
        return Err(WalkError::with_reason(
            "Quoted whitespace-only string cannot be statically modeled",
        ));
    }
    Ok(result)
}

fn walk_arithmetic(node: Node, src: &str) -> WalkResult<()> {
    let mut cursor = node.walk();
    for child in node.named_children(&mut cursor) {
        if child.named_child_count() == 0 {
            let text = child_text(child, src);
            if !ARITH_LEAF_RE.is_match(&text) {
                return Err(WalkError::with_reason(
                    "Arithmetic expansion references variable or non-literal",
                ));
            }
            continue;
        }
        match child.kind() {
            "binary_expression" | "unary_expression" | "ternary_expression"
            | "parenthesized_expression" => walk_arithmetic(child, src)?,
            _ => return Err(WalkError::too_complex(child)),
        }
    }
    Ok(())
}

fn walk_variable_assignment(
    node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<VarAssign> {
    let mut name: Option<String> = None;
    let mut value = String::new();
    let mut is_append = false;

    let mut cursor = node.walk();
    for child in node.named_children(&mut cursor) {
        match child.kind() {
            "variable_name" => name = Some(child_text(child, src)),
            "=" | "+=" => {
                is_append = child.kind() == "+=";
                continue;
            }
            "command_substitution" => {
                collect_command_substitution(child, commands, var_scope, src)?;
                value = CMDSUB_PLACEHOLDER.to_string();
            }
            "simple_expansion" => {
                let v = resolve_simple_expansion(child, var_scope, true, src)?;
                value = v;
            }
            _ => {
                value = walk_argument(child, commands, var_scope, src)?;
            }
        }
    }

    let Some(name) = name else {
        return Err(WalkError::with_reason("Variable assignment without name"));
    };
    if !Regex::new(r"^[A-Za-z_][A-Za-z0-9_]*$").unwrap().is_match(&name) {
        return Err(WalkError::with_reason(
            "Invalid variable name (bash treats as command)",
        ));
    }
    if name == "IFS" {
        return Err(WalkError::with_reason(
            "IFS assignment changes word-splitting — cannot model statically",
        ));
    }
    if name == "PS4" {
        if is_append {
            return Err(WalkError::with_reason(
                "PS4 += cannot be statically verified — combine into a single PS4= assignment",
            ));
        }
        if contains_any_placeholder(&value) {
            return Err(WalkError::with_reason(
                "PS4 value derived from cmdsub/variable — runtime unknowable",
            ));
        }
        let cleaned = PS4_REF_RE.replace_all(&value, "");
        if !PS4_SAFE_CHARSET_RE.is_match(&cleaned) {
            return Err(WalkError::with_reason(
                "PS4 value outside safe charset",
            ));
        }
    }
    if value.contains('~') {
        return Err(WalkError::with_reason(
            "Tilde in assignment value — bash may expand at assignment time",
        ));
    }
    Ok(VarAssign { name, value, is_append })
}

struct VarAssign {
    name: String,
    value: String,
    is_append: bool,
}

fn apply_var_to_scope(scope: &mut VarScope, name: &str, value: &str, is_append: bool) {
    let existing = scope.0.get(name).cloned().unwrap_or_default();
    let combined = if is_append { existing + value } else { value.to_string() };
    let stored = if contains_any_placeholder(&combined) {
        VAR_PLACEHOLDER.to_string()
    } else {
        combined
    };
    scope.0.insert(name.to_string(), stored);
}

fn resolve_simple_expansion(
    node: Node,
    var_scope: &mut VarScope,
    inside_string: bool,
    src: &str,
) -> WalkResult<String> {
    let mut var_name: Option<String> = None;
    let mut is_special = false;
    let mut cursor = node.walk();
    for c in node.named_children(&mut cursor) {
        match c.kind() {
            "variable_name" => {
                var_name = Some(child_text(c, src));
                break;
            }
            "special_variable_name" => {
                var_name = Some(child_text(c, src));
                is_special = true;
                break;
            }
            _ => {}
        }
    }
    let Some(var_name) = var_name else {
        return Err(WalkError::with_reason("Expansion without variable name"));
    };

    if let Some(tracked_value) = var_scope.0.get(&var_name).cloned() {
        if contains_any_placeholder(&tracked_value) {
            if !inside_string {
                return Err(WalkError::with_reason(
                    "Bare argument resolves to runtime-unknown variable",
                ));
            }
            return Ok(VAR_PLACEHOLDER.to_string());
        }
        if !inside_string {
            if tracked_value.is_empty() {
                return Err(WalkError::with_reason(
                    "Bare argument is empty expansion (word-splitting differential)",
                ));
            }
            if bare_var_unsafe(&tracked_value) {
                return Err(WalkError::with_reason(
                    "Bare argument value contains IFS/glob characters",
                ));
            }
        }
        return Ok(tracked_value);
    }

    if inside_string {
        if safe_env_vars().contains(var_name.as_str()) {
            return Ok(VAR_PLACEHOLDER.to_string());
        }
        if is_special && (special_var_names().contains(var_name.as_str()) || var_name.chars().all(|c| c.is_ascii_digit())) {
            return Ok(VAR_PLACEHOLDER.to_string());
        }
    }
    Err(WalkError::with_reason(
        "Untracked variable expansion in bare argument position",
    ))
}

fn collect_command_substitution(
    cs_node: Node,
    commands: &mut Vec<InternalCommand>,
    var_scope: &mut VarScope,
    src: &str,
) -> WalkResult<()> {
    let mut inner_scope = var_scope.clone_scope();
    let mut cursor = cs_node.walk();
    for child in cs_node.children(&mut cursor) {
        if matches!(child.kind(), "$(" | "`" | ")") {
            continue;
        }
        collect_commands(child, commands, &mut inner_scope, src)?;
    }
    Ok(())
}

fn extract_safe_cat_heredoc(sub_node: Node, src: &str) -> Option<String> {
    let mut stmt: Option<Node> = None;
    for idx in 0..sub_node.named_child_count() {
        let child = sub_node.named_child(idx)?;
        match child.kind() {
            "$(" | ")" => continue,
            "redirected_statement" if stmt.is_none() => stmt = Some(child),
            _ => return None,
        }
    }
    let stmt = stmt?;
    let mut saw_cat = false;
    let mut body: Option<String> = None;
    let mut cursor = stmt.walk();
    for child in stmt.named_children(&mut cursor) {
        match child.kind() {
            "command" => {
                if child.named_child_count() != 1 {
                    return None;
                }
                let name_node = child.named_child(0)?;
                if name_node.kind() != "command_name" || node_text(name_node, src) != "cat" {
                    return None;
                }
                saw_cat = true;
            }
            "heredoc_redirect" => {
                if walk_heredoc_redirect(child, src).is_err() {
                    return None;
                }
                let mut c = child.walk();
                for hc in child.children(&mut c) {
                    if hc.kind() == "heredoc_body" {
                        body = Some(node_text(hc, src));
                    }
                }
            }
            _ => return None,
        }
    }
    if !saw_cat {
        return None;
    }
    let body = body?;
    if PROC_ENVIRON_RE.is_match(&body) {
        return Some("DANGEROUS".to_string());
    }
    if Regex::new(r"\bsystem\s*\(").unwrap().is_match(&body) {
        return Some("DANGEROUS".to_string());
    }
    Some(body)
}

fn bare_var_unsafe(value: &str) -> bool {
    value
        .chars()
        .any(|c| matches!(c, ' ' | '\t' | '\n' | '*' | '?' | '['))
}

fn child_text(node: Node, src: &str) -> String {
    node.utf8_text(src.as_bytes())
        .unwrap_or_default()
        .to_string()
}

fn node_text(node: Node, src: &str) -> String {
    child_text(node, src)
}
