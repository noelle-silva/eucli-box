//! Bash parsing helpers ported from Codex `shell-command/src/bash.rs`.
//! Uses tree-sitter-bash for parsing. Two extraction modes:
//! - [`parse_shell_script_into_commands`]: word-only plain-command sequence
//!   (used for *safe* classification proof).
//! - [`parse_shell_lc_literal_commands`]: literal command extraction from complex
//!   scripts (used for *dangerous* detection; never for safe proofs).

use std::path::PathBuf;

use tree_sitter::{Node, Parser, Tree};
use tree_sitter_bash::LANGUAGE as BASH;

use crate::shell_type::detect_shell_type;

pub fn try_parse_shell(source: &str) -> Option<Tree> {
    let lang = BASH.into();
    let mut parser = Parser::new();
    parser.set_language(&lang).ok()?;
    parser.parse(source, None)
}

fn parse_plain_command_from_node(cmd: Node, src: &str) -> Option<Vec<String>> {
    if cmd.kind() != "command" {
        return None;
    }
    let mut words = Vec::new();
    let mut cursor = cmd.walk();
    for child in cmd.named_children(&mut cursor) {
        match child.kind() {
            "command_name" => {
                let word_node = child.named_child(0)?;
                if word_node.kind() != "word" {
                    return None;
                }
                words.push(word_node.utf8_text(src.as_bytes()).ok()?.to_owned());
            }
            "word" | "number" => {
                words.push(child.utf8_text(src.as_bytes()).ok()?.to_owned());
            }
            "string" => {
                let parsed = parse_double_quoted_string(child, src)?;
                words.push(parsed);
            }
            "raw_string" => {
                let parsed = parse_raw_string(child, src)?;
                words.push(parsed);
            }
            "concatenation" => {
                let mut concatenated = String::new();
                let mut concat_cursor = child.walk();
                for part in child.named_children(&mut concat_cursor) {
                    match part.kind() {
                        "word" | "number" => {
                            concatenated.push_str(part.utf8_text(src.as_bytes()).ok()?);
                        }
                        "string" => {
                            let parsed = parse_double_quoted_string(part, src)?;
                            concatenated.push_str(&parsed);
                        }
                        "raw_string" => {
                            let parsed = parse_raw_string(part, src)?;
                            concatenated.push_str(&parsed);
                        }
                        _ => return None,
                    }
                }
                if concatenated.is_empty() {
                    return None;
                }
                words.push(concatenated);
            }
            _ => return None,
        }
    }
    Some(words)
}

fn parse_literal_command_from_node(cmd: Node, src: &str) -> Option<Vec<String>> {
    if cmd.kind() != "command" {
        return None;
    }
    let mut words = Vec::new();
    let mut found_command_name = false;
    let mut cursor = cmd.walk();
    for child in cmd.named_children(&mut cursor) {
        if child.kind() == "command_name" {
            let command_name = parse_literal_shell_word(child.named_child(0)?, src)?;
            words.push(command_name);
            found_command_name = true;
        } else if found_command_name {
            if let Some(word) = parse_literal_shell_word(child, src) {
                words.push(word);
            }
        }
    }
    found_command_name.then_some(words)
}

fn parse_literal_shell_word(node: Node, src: &str) -> Option<String> {
    match node.kind() {
        "word" | "number" if is_literal_word_or_number(node) => {
            Some(node.utf8_text(src.as_bytes()).ok()?.to_owned())
        }
        "string" => parse_double_quoted_string(node, src),
        "raw_string" => parse_raw_string(node, src),
        "concatenation" => {
            let mut concatenated = String::new();
            let mut cursor = node.walk();
            for part in node.named_children(&mut cursor) {
                concatenated.push_str(&parse_literal_shell_word(part, src)?);
            }
            (!concatenated.is_empty()).then_some(concatenated)
        }
        _ => None,
    }
}

fn parse_heredoc_command_words(cmd: Node, src: &str) -> Option<Vec<String>> {
    if cmd.kind() != "command" {
        return None;
    }
    let mut words = Vec::new();
    let mut cursor = cmd.walk();
    for child in cmd.named_children(&mut cursor) {
        match child.kind() {
            "command_name" => {
                let word_node = child.named_child(0)?;
                if !matches!(word_node.kind(), "word" | "number")
                    || !is_literal_word_or_number(word_node)
                {
                    return None;
                }
                words.push(word_node.utf8_text(src.as_bytes()).ok()?.to_owned());
            }
            "word" | "number" => {
                if !is_literal_word_or_number(child) {
                    return None;
                }
                words.push(child.utf8_text(src.as_bytes()).ok()?.to_owned());
            }
            "comment" => {}
            kind if is_allowed_heredoc_attachment_kind(kind) => {}
            _ => return None,
        }
    }
    if words.is_empty() {
        None
    } else {
        Some(words)
    }
}

fn is_literal_word_or_number(node: Node) -> bool {
    if !matches!(node.kind(), "word" | "number") {
        return false;
    }
    node.named_child_count() == 0
}

fn is_allowed_heredoc_attachment_kind(kind: &str) -> bool {
    matches!(
        kind,
        "heredoc_body"
            | "simple_heredoc_body"
            | "heredoc_redirect"
            | "herestring_redirect"
            | "redirected_statement"
    )
}

fn find_single_command_node(root: Node) -> Option<Node> {
    let mut stack = vec![root];
    let mut single_command = None;
    while let Some(node) = stack.pop() {
        if node.kind() == "command" {
            if single_command.is_some() {
                return None;
            }
            single_command = Some(node);
        }
        let mut cursor = node.walk();
        for child in node.named_children(&mut cursor) {
            stack.push(child);
        }
    }
    single_command
}

fn has_named_descendant_kind(node: Node, kind: &str) -> bool {
    let mut stack = vec![node];
    while let Some(current) = stack.pop() {
        if current.kind() == kind {
            return true;
        }
        let mut cursor = current.walk();
        for child in current.named_children(&mut cursor) {
            stack.push(child);
        }
    }
    false
}

fn parse_double_quoted_string(node: Node, src: &str) -> Option<String> {
    if node.kind() != "string" {
        return None;
    }
    let mut cursor = node.walk();
    for part in node.named_children(&mut cursor) {
        if part.kind() != "string_content" {
            return None;
        }
    }
    let raw = node.utf8_text(src.as_bytes()).ok()?;
    let stripped = raw.strip_prefix('"').and_then(|text| text.strip_suffix('"'))?;
    Some(stripped.to_string())
}

fn parse_raw_string(node: Node, src: &str) -> Option<String> {
    if node.kind() != "raw_string" {
        return None;
    }
    let raw_string = node.utf8_text(src.as_bytes()).ok()?;
    let stripped = raw_string.strip_prefix('\'').and_then(|s| s.strip_suffix('\''));
    stripped.map(str::to_owned)
}

/// Returns `Some(Vec<command_words>)` if every command is a plain word-only
/// command joined by safe operators (`&&`, `||`, `;`, `|`). None otherwise.
pub fn try_parse_word_only_commands_sequence(tree: &Tree, src: &str) -> Option<Vec<Vec<String>>> {
    if tree.root_node().has_error() {
        return None;
    }

    const ALLOWED_KINDS: &[&str] = &[
        "program",
        "list",
        "pipeline",
        "command",
        "command_name",
        "word",
        "string",
        "string_content",
        "raw_string",
        "number",
        "concatenation",
    ];
    const ALLOWED_PUNCT_TOKENS: &[&str] = &["&&", "||", ";", "|", "\"", "'"];

    let root = tree.root_node();
    let mut cursor = root.walk();
    let mut stack = vec![root];
    let mut command_nodes = Vec::new();
    while let Some(node) = stack.pop() {
        let kind = node.kind();
        if node.is_named() {
            if !ALLOWED_KINDS.contains(&kind) {
                return None;
            }
            if kind == "command" {
                command_nodes.push(node);
            }
        } else {
            if kind.chars().any(|c| "&;|".contains(c)) && !ALLOWED_PUNCT_TOKENS.contains(&kind) {
                return None;
            }
            if !(ALLOWED_PUNCT_TOKENS.contains(&kind) || kind.trim().is_empty()) {
                return None;
            }
        }
        for child in node.children(&mut cursor) {
            stack.push(child);
        }
    }

    command_nodes.sort_by_key(Node::start_byte);

    let mut commands = Vec::new();
    for node in command_nodes {
        if let Some(words) = parse_plain_command_from_node(node, src) {
            commands.push(words);
        } else {
            return None;
        }
    }
    Some(commands)
}

pub fn parse_shell_script_into_commands(script: &str) -> Option<Vec<Vec<String>>> {
    let tree = try_parse_shell(script)?;
    try_parse_word_only_commands_sequence(&tree, script)
}

pub fn extract_bash_command(command: &[String]) -> Option<(&str, &str)> {
    let [shell, flag, script] = command else {
        return None;
    };
    if !matches!(flag.as_str(), "-lc" | "-c") {
        return None;
    }
    let kind = detect_shell_type(PathBuf::from(shell));
    if !matches!(kind, Some(crate::shell_type::ShellType::Zsh) | Some(crate::shell_type::ShellType::Bash) | Some(crate::shell_type::ShellType::Sh)) {
        return None;
    }
    Some((shell, script))
}

pub fn parse_shell_lc_plain_commands(command: &[String]) -> Option<Vec<Vec<String>>> {
    let (_, script) = extract_bash_command(command)?;
    parse_shell_script_into_commands(script)
}

pub fn parse_shell_lc_literal_commands(command: &[String]) -> Option<Vec<Vec<String>>> {
    let (_, script) = extract_bash_command(command)?;
    let tree = try_parse_shell(script)?;
    let root = tree.root_node();
    if root.has_error() {
        return None;
    }
    let mut commands = Vec::new();
    let mut stack = vec![root];
    while let Some(node) = stack.pop() {
        if node.kind() == "command" {
            if let Some(command) = parse_literal_command_from_node(node, script) {
                commands.push(command);
            }
        }
        let mut cursor = node.walk();
        for child in node.named_children(&mut cursor) {
            stack.push(child);
        }
    }
    Some(commands)
}

/// Extracts literal command words from an arbitrary bash source string.
/// Dynamic words and redirections are omitted. Suitable for dangerous-literal
/// detection; never for safe proofs.
pub fn extract_literal_commands(source: &str) -> Option<Vec<Vec<String>>> {
    let tree = try_parse_shell(source)?;
    let root = tree.root_node();
    if root.has_error() {
        return None;
    }
    let mut commands = Vec::new();
    let mut stack = vec![root];
    while let Some(node) = stack.pop() {
        if node.kind() == "command" {
            if let Some(command) = parse_literal_command_from_node(node, source) {
                commands.push(command);
            }
        }
        let mut cursor = node.walk();
        for child in node.named_children(&mut cursor) {
            stack.push(child);
        }
    }
    Some(commands)
}

pub fn parse_shell_lc_single_command_prefix(command: &[String]) -> Option<Vec<String>> {
    let (_, script) = extract_bash_command(command)?;
    let tree = try_parse_shell(script)?;
    let root = tree.root_node();
    if root.has_error() {
        return None;
    }
    if !has_named_descendant_kind(root, "heredoc_redirect") {
        return None;
    }
    if has_named_descendant_kind(root, "file_redirect") {
        return None;
    }
    let command_node = find_single_command_node(root)?;
    parse_heredoc_command_words(command_node, script)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_single_simple_command() {
        let cmds = parse_shell_script_into_commands("ls -1").unwrap();
        assert_eq!(cmds, vec![vec!["ls".to_string(), "-1".to_string()]]);
    }

    #[test]
    fn accepts_multiple_commands_with_allowed_operators() {
        let src = "ls && pwd; echo 'hi there' | wc -l";
        let cmds = parse_shell_script_into_commands(src).unwrap();
        let expected: Vec<Vec<String>> = vec![
            vec!["ls".to_string()],
            vec!["pwd".to_string()],
            vec!["echo".to_string(), "hi there".to_string()],
            vec!["wc".to_string(), "-l".to_string()],
        ];
        assert_eq!(cmds, expected);
    }

    #[test]
    fn extracts_double_and_single_quoted_strings() {
        let cmds = parse_shell_script_into_commands("echo \"hello world\"").unwrap();
        assert_eq!(cmds, vec![vec!["echo".to_string(), "hello world".to_string()]]);
        let cmds2 = parse_shell_script_into_commands("echo 'hi there'").unwrap();
        assert_eq!(cmds2, vec![vec!["echo".to_string(), "hi there".to_string()]]);
    }

    #[test]
    fn accepts_double_quoted_strings_with_newlines() {
        let cmds = parse_shell_script_into_commands("git commit -m \"line1\nline2\"").unwrap();
        assert_eq!(
            cmds,
            vec![vec![
                "git".to_string(),
                "commit".to_string(),
                "-m".to_string(),
                "line1\nline2".to_string(),
            ]]
        );
    }

    #[test]
    fn accepts_mixed_quote_concatenation() {
        assert_eq!(
            parse_shell_script_into_commands(r#"echo "/usr"'/'"local"/bin"#).unwrap(),
            vec![vec!["echo".to_string(), "/usr/local/bin".to_string()]]
        );
    }

    #[test]
    fn rejects_double_quoted_strings_with_expansions() {
        assert!(parse_shell_script_into_commands(r#"echo "hi ${USER}""#).is_none());
        assert!(parse_shell_script_into_commands(r#"echo "$HOME""#).is_none());
    }

    #[test]
    fn rejects_parentheses_and_subshells() {
        assert!(parse_shell_script_into_commands("(ls)").is_none());
        assert!(parse_shell_script_into_commands("ls || (pwd && echo hi)").is_none());
    }

    #[test]
    fn rejects_redirections_and_unsupported_operators() {
        assert!(parse_shell_script_into_commands("ls > out.txt").is_none());
        assert!(parse_shell_script_into_commands("echo hi & echo bye").is_none());
    }

    #[test]
    fn rejects_command_and_process_substitutions_and_expansions() {
        assert!(parse_shell_script_into_commands("echo $(pwd)").is_none());
        assert!(parse_shell_script_into_commands("echo `pwd`").is_none());
        assert!(parse_shell_script_into_commands("echo $HOME").is_none());
        assert!(parse_shell_script_into_commands("echo \"hi $USER\"").is_none());
    }

    #[test]
    fn rejects_variable_assignment_prefix() {
        assert!(parse_shell_script_into_commands("FOO=bar ls").is_none());
    }

    #[test]
    fn rejects_trailing_operator_parse_error() {
        assert!(parse_shell_script_into_commands("ls &&").is_none());
    }

    #[test]
    fn rejects_empty_command_position_with_leading_operator() {
        assert!(parse_shell_script_into_commands("&& ls").is_none());
    }

    #[test]
    fn rejects_empty_command_position_with_double_separator() {
        assert!(parse_shell_script_into_commands("ls ;; pwd").is_none());
    }

    #[test]
    fn rejects_empty_command_position_with_empty_pipeline_segment() {
        assert!(parse_shell_script_into_commands("ls | | wc").is_none());
    }

    #[test]
    fn accepts_concatenated_flag_and_value() {
        let cmds = parse_shell_script_into_commands("rg -n \"foo\" -g\"*.py\"").unwrap();
        assert_eq!(
            cmds,
            vec![vec![
                "rg".to_string(),
                "-n".to_string(),
                "foo".to_string(),
                "-g*.py".to_string(),
            ]]
        );
    }

    #[test]
    fn rejects_concatenation_with_variable_substitution() {
        assert!(parse_shell_script_into_commands("rg -g\"$VAR\" pattern").is_none());
    }

    #[test]
    fn rejects_concatenation_with_command_substitution() {
        assert!(parse_shell_script_into_commands("rg -g\"$(pwd)\" pattern").is_none());
    }

    #[test]
    fn parse_shell_lc_single_command_prefix_supports_heredoc() {
        let command = vec![
            "zsh".to_string(),
            "-lc".to_string(),
            "python3 <<'PY'\nprint('hello')\nPY".to_string(),
        ];
        assert_eq!(parse_shell_lc_single_command_prefix(&command), Some(vec!["python3".to_string()]));
    }

    #[test]
    fn parse_shell_lc_single_command_prefix_rejects_multi_command_scripts() {
        let command = vec![
            "bash".to_string(),
            "-lc".to_string(),
            "python3 <<'PY'\nprint('hello')\nPY\necho done".to_string(),
        ];
        assert_eq!(parse_shell_lc_single_command_prefix(&command), None);
    }

    #[test]
    fn parse_shell_lc_single_command_prefix_rejects_non_heredoc_redirects() {
        let command = vec![
            "bash".to_string(),
            "-lc".to_string(),
            "echo hello > /tmp/out.txt".to_string(),
        ];
        assert_eq!(parse_shell_lc_single_command_prefix(&command), None);
    }

    #[test]
    fn for_loop_rm_literal_extraction() {
        let command = vec![
            "bash".to_string(),
            "-lc".to_string(),
            "for t in /tmp/a /tmp/b; do rm -r -f \"$t\"; done".to_string(),
        ];
        let parsed = parse_shell_lc_literal_commands(&command).unwrap();
        assert!(
            parsed.iter().any(|c| c.first().map(|s| s.as_str()) == Some("rm")),
            "{parsed:?}"
        );
    }

    #[test]
    fn extract_literal_commands_double_quoted_dollar() {
        let parsed = extract_literal_commands("echo \"$(rm -rf /tmp/x)\"").unwrap();
        assert!(
            parsed.iter().any(|c| c.first().map(|s| s.as_str()) == Some("rm")),
            "{parsed:?}"
        );
    }

    #[test]
    fn parse_shell_lc_literal_commands_extracts_nested() {
        let command = vec![
            "bash".to_string(),
            "-lc".to_string(),
            "if test -d /tmp/example; then rm -rf /tmp/example; fi".to_string(),
        ];
        let parsed = parse_shell_lc_literal_commands(&command).unwrap();
        assert!(parsed.iter().any(|c| c[0] == "test"));
        assert!(parsed.iter().any(|c| c == &vec!["rm".to_string(), "-rf".to_string(), "/tmp/example".to_string()]));
    }
}
