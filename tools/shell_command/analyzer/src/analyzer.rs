//! Unified analysis orchestration: combines the Codex safety core, Claude
//! semantics/read-only checks and OpenCode impact scan into one report.

use crate::bash_walk;
use crate::impact;

use crate::protocol::*;
use crate::readonly;
use crate::safety;
use crate::semantics;

pub fn analyze_bash(command: &str, workdir: &str) -> AnalyzeResponse {
    let mut reasons: Vec<String> = Vec::new();
    let mut classification_reasons: Vec<String> = Vec::new();
    let mut classification = Classification::Unknown;
    let mut reliability = Reliability::High;

    // Pre-checks (Claude parseForSecurityFromAst).
    if let Err(reason) = bash_walk::precheck(command) {
        reasons.push(format!("precheck: {reason}"));
        classification_reasons.push(format!("command contains statically-unanalysable content: {reason}"));
        return build_response(
            ShellKind::Bash,
            classification,
            classification_reasons,
            CommandStructure::default(),
            SemanticsReport::default(),
            None,
            ReadOnlyReport::default(),
            ImpactReport::default(),
            Reliability::Low,
            reasons,
        );
    }

    let Some(tree) = crate::bash::try_parse_shell(command) else {
        reasons.push("tree-sitter parse failed".to_string());
        classification_reasons.push("command did not parse".to_string());
        return build_response(
            ShellKind::Bash,
            Classification::Unknown,
            classification_reasons,
            CommandStructure { parse_error: true, ..Default::default() },
            SemanticsReport::default(),
            None,
            ReadOnlyReport::default(),
            ImpactReport::default(),
            Reliability::Low,
            reasons,
        );
    };

    // Codex dangerous detection runs on literal command extraction from the
    // raw source. It is independent from the Claude structure verdict: a
    // complex script that the walker cannot model (e.g. a for loop) may still
    // contain a literal dangerous command that must flag the classification.
    let mut dangerous_match: Option<DangerousMatch> = None;
    let mark_danger = |cmd: &[String], reasons: &mut Vec<String>, classification_reasons: &mut Vec<String>| {
        if let Some(m) = safety::dangerous_command_match(cmd) {
            let kind = match m {
                safety::DangerousCommandMatch::ForcedRm => "forced_rm",
                safety::DangerousCommandMatch::Other => "other",
            };
            reasons.push(format!("dangerous rule {kind}: {}", cmd.join(" ")));
            classification_reasons.push(format!(
                "matches dangerous command rule {kind}: {}",
                cmd.join(" ")
            ));
            true
        } else {
            false
        }
    };
    if let Some(found) = detect_dangerous_raw(command) {
        mark_danger(&found.cmd, &mut reasons, &mut classification_reasons);
        dangerous_match = Some(DangerousMatch {
            kind: found.kind.clone(),
            detail: format!("matched: {}", found.cmd.join(" ")),
        });
    }

    // Structure extraction (fail-closed for argv, not for classification).
    let mut structure = CommandStructure { commands: Vec::new(), operators: Vec::new(), parse_error: false, arity_prefix: String::new() };
    let mut parsed_commands: Vec<InternalCommand> = Vec::new();
    let mut walker_ok = false;
    match bash_walk::parse_for_security(&tree, command) {
        Ok(cmds) => {
            structured(cmds.iter().map(|c| CommandNode {
                argv: c.argv.clone(),
                text: c.text.clone(),
                dynamic: c.dynamic,
            }).collect(), &mut structure);
            parsed_commands = cmds;
            walker_ok = true;
            // OpenCode BashArity prefix of the first command (permission
            // layer memorable-prefix input).
            if let Some(first) = parsed_commands.first() {
                if !first.argv.is_empty() {
                    structure.arity_prefix = impact::bash_arity_prefix(&first.argv).join(" ");
                }
            }
        }
        Err(e) => {
            structure.parse_error = tree.root_node().has_error();
            reasons.push(format!("structure: {e:?}"));
            classification_reasons.push("command structure could not be statically analysed".to_string());
            reliability = Reliability::Low;
        }
    }

    // Semantic checks (Claude checkSemantics): dangerous by name/content.
    // Only when the walker produced trustworthy argv.
    let mut semantics_report = SemanticsReport::default();
    let mut semantics_untrusted = false;
    if walker_ok {
        match semantics::check_semantics(&parsed_commands) {
            semantics::SemanticVerdict::Safe => {}
            semantics::SemanticVerdict::Dangerous(reason) => {
                semantics_report.dangerous = true;
                semantics_report.reasons.push(reason.clone());
                reasons.push(format!("semantics: {reason}"));
                classification_reasons.push(format!("command semantics are dangerous: {reason}"));
            }
            semantics::SemanticVerdict::Untrusted(reason) => {
                semantics_untrusted = true;
                reasons.push(format!("semantics-untrusted: {reason}"));
                classification_reasons.push(format!(
                    "command arguments cannot be fully trusted: {reason}"
                ));
                if reliability != Reliability::Low {
                    reliability = Reliability::Medium;
                }
            }
        }
    }

    // Classification decision: semantics-danger or any dangerous rule wins;
    // else Codex known-safe proof (walker-based argv) -> safe; else unknown.
    let safecmd: Vec<Vec<String>> = parsed_commands.iter().map(|c| c.argv.clone()).collect();
    let mut walker_all_safe = false;
    if walker_ok {
        walker_all_safe = !safecmd.is_empty()
            && safecmd.iter().all(|cmd| !cmd.is_empty() && safety::is_known_safe_command(cmd));
    }
    if semantics_report.dangerous || dangerous_match.is_some() {
        classification = Classification::Dangerous;
    } else if walker_all_safe && !semantics_untrusted {
        classification = Classification::Safe;
        classification_reasons.push("all commands are provably read-only".to_string());
    } else {
        classification = Classification::Unknown;
        classification_reasons.push(
            "command is not provably read-only; requires confirmation".to_string(),
        );
    }

    // Read-only verification per subcommand (Claude layers), plus write
    // redirects make the whole command not provably read-only.
    let mut ro_report = ReadOnlyReport::default();
    let mut ro_all = walker_ok;
    for cmd in &parsed_commands {
        if cmd.argv.is_empty() {
            continue;
        }
        for r in &cmd.redirects {
            if matches!(r.op.as_str(), ">" | ">>" | ">|" | "&>" | "&>>")
                || (r.op == ">&" && !r.target.chars().all(|c| c.is_ascii_digit()))
            {
                ro_all = false;
                ro_report.unsafe_flags.push(format!("redirect {}", r.op));
            }
        }
        if cmd.dynamic || cmd.argv.iter().any(|a| a.contains('$') || a.contains("`")) {
            ro_all = false;
            ro_report.unsafe_flags.push("dynamic argument".to_string());
            continue;
        }
        let text = cmd.argv.join(" ");
        if !crate::readonly_simple::is_command_read_only(&text) {
            ro_all = false;
            ro_report.unsafe_flags.push(cmd.argv.first().cloned().unwrap_or_default());
        }
    }
    if ro_all {
        ro_report.verified = true;
        ro_report.verified_command = command.to_string();
        reasons.push("read-only flags verified".to_string());
    } else {
        reasons.push("read-only check did not verify".to_string());
    }

    // UNC check (Claude).
    if readonly::contains_vulnerable_unc_path(command) {
        classification_reasons.push("command contains vulnerable UNC path".to_string());
        reliability = Reliability::Medium;
    }

    // Impact analysis (Claude PATH_EXTRACTORS + redirects + dangerous paths).
    let mut impact_report = ImpactReport::default();
    for cmd in &parsed_commands {
        let (items, dangerous) = impact::analyze_command_impact(
            &cmd.argv,
            &cmd.redirects,
            if workdir.is_empty() { "." } else { workdir },
        );
        impact_report.items.extend(items);
        impact_report.dangerous_removal_paths.extend(dangerous);
    }

    // OpenCode style: file commands with outside-workdir args -> mark.
    for cmd in &parsed_commands {
        if let Some(first) = cmd.argv.first() {
            let (_, outside) = impact::collect_file_paths(
                first.clone(),
                &cmd.argv[1..],
                false,
                false,
            );
            for p in outside {
                if !p.is_empty() {
                    // Already present via extract_paths on file commands; only
                    // ensure workdir-boundary facts are visible.
                    impact_report.unresolved.push(p);
                }
            }
        }
    }

    build_response(
        ShellKind::Bash,
        classification,
        classification_reasons,
        structure,
        semantics_report,
        dangerous_match,
        ro_report,
        impact_report,
        reliability,
        reasons,
    )
}

struct RawDanger {
    kind: String,
    cmd: Vec<String>,
}

/// Codex-style dangerous arbitration over the raw command string. Uses the
/// literal extraction plus one unwrapped argv pass so that `bash -lc "…"`,
/// `sudo …`, `env …` style wrappers are unwrapped exactly like Codex does.
fn detect_dangerous_raw(command: &str) -> Option<RawDanger> {
    if let Some(literals) = crate::bash::extract_literal_commands(command) {
        for literal in &literals {
            if let Some(m) = safety::dangerous_command_match(literal) {
                return Some(RawDanger {
                    kind: kind_of(m),
                    cmd: literal.clone(),
                });
            }
        }
    }
    if let Some(tokens) = shlex::split(command) {
        let mut candidates: Vec<&[String]> = Vec::new();
        if tokens.len() > 1 {
            candidates.push(&tokens[1..]);
        }
        candidates.push(&tokens[..]);
        for candidate in candidates {
            if let Some(m) = safety::dangerous_command_match(candidate) {
                return Some(RawDanger {
                    kind: kind_of(m),
                    cmd: candidate.to_vec(),
                });
            }
        }
    }
    None
}

fn kind_of(m: safety::DangerousCommandMatch) -> String {
    match m {
        safety::DangerousCommandMatch::ForcedRm => "forced_rm".to_string(),
        safety::DangerousCommandMatch::Other => "other".to_string(),
    }
}

fn structured(nodes: Vec<CommandNode>, structure: &mut CommandStructure) {
    structure.commands = nodes;
}

pub fn analyze_powershell(command: &str, workdir: &str) -> AnalyzeResponse {
    let reasons: Vec<String> = Vec::new();
    let mut classification_reasons: Vec<String> = Vec::new();
    let structure = CommandStructure::default();

    // Tokenize the command as an invocation: [exe, args...].
    let tokens = match crate::powershell::tokenize_powershell_invocation(command) {
        Some(mut t) => {
            crate::powershell::resolve_powershell_executable(&mut t);
            t
        }
        None => {
            classification_reasons.push("command did not parse as a PowerShell invocation".to_string());
            return build_response(
                ShellKind::PowerShell,
                Classification::Unknown,
                classification_reasons,
                structure,
                SemanticsReport::default(),
                None,
                ReadOnlyReport::default(),
                ImpactReport::default(),
                Reliability::Low,
                reasons,
            );
        }
    };

    // Compute safety via Codex windows rules (evaluation order mirrors
    // Codex shell-escalation: safe check then dangerous check).
    let safe = safety::is_known_safe_command(&tokens);
    let dangerous = safety::dangerous_command_match(&tokens);

    let mut response = if safe && dangerous.is_none() {
        build_response(
            ShellKind::PowerShell,
            Classification::Safe,
            vec!["PowerShell invocation is provably read-only".to_string()],
            structure,
            SemanticsReport::default(),
            None,
            ReadOnlyReport { verified: true, verified_command: command.to_string(), unsafe_flags: Vec::new(), reason: String::new() },
            ImpactReport::default(),
            Reliability::High,
            vec!["powershell safelist verified".to_string()],
        )
    } else if dangerous.is_some() {
        build_response(
            ShellKind::PowerShell,
            Classification::Dangerous,
            vec!["matches dangerous PowerShell rule".to_string()],
            structure,
            SemanticsReport::default(),
            Some(DangerousMatch {
                kind: "powershell_dangerous".to_string(),
                detail: command.to_string(),
            }),
            ReadOnlyReport::default(),
            ImpactReport::default(),
            Reliability::High,
            reasons,
        )
    } else {
        build_response(
            ShellKind::PowerShell,
            Classification::Unknown,
            vec!["PowerShell invocation is not provably read-only".to_string()],
            structure,
            SemanticsReport::default(),
            None,
            ReadOnlyReport::default(),
            ImpactReport::default(),
            Reliability::Medium,
            reasons,
        )
    };
    response.command_structure.commands.push(CommandNode {
        argv: tokens.clone(),
        text: command.to_string(),
        dynamic: false,
    });
    // Impact for PS: file cmdlets (Codex's set) with -Path values.
    let mut impact_report = response.impact.clone();
    if let Some(first) = tokens.first() {
        let (items, _outside) = impact::collect_file_paths(
            first.clone(),
            &tokens[1..],
            true,
            false,
        );
        for p in items {
            let mut item = ImpactItem::new(p, "argument");
            item.effects.push("access".into());
            impact_report.items.push(item);
        }
    }
    response.impact = impact_report;
    let _ = workdir;
    response
}

pub fn analyze_unsupported(shell: ShellKind, command: &str) -> AnalyzeResponse {
    build_response(
        shell,
        Classification::Unknown,
        vec![format!("{shell:?} command analysis is not supported by this analyzer")],
        CommandStructure::default(),
        SemanticsReport::default(),
        None,
        ReadOnlyReport::default(),
        ImpactReport::default(),
        Reliability::Low,
        vec![format!("unsupported shell: {shell:?} (command len {})", command.len())],
    )
}

pub fn analyze_full(request: &AnalyzeRequest) -> AnalyzeResponse {
    match request.shell {
        ShellKind::Bash | ShellKind::Sh | ShellKind::Zsh => {
            analyze_bash(&request.command, request.workdir.trim())
        }
        ShellKind::PowerShell => analyze_powershell(&request.command, request.workdir.trim()),
        _ => analyze_unsupported(request.shell, &request.command),
    }
}

#[allow(clippy::too_many_arguments)]
pub fn build_response(
    shell: ShellKind,
    classification: Classification,
    classification_reasons: Vec<String>,
    command_structure: CommandStructure,
    semantics: SemanticsReport,
    dangerous_match: Option<DangerousMatch>,
    read_only: ReadOnlyReport,
    impact: ImpactReport,
    reliability: Reliability,
    reasons: Vec<String>,
) -> AnalyzeResponse {
    AnalyzeResponse {
        version: PROTOCOL_VERSION,
        shell,
        classification,
        classification_reasons: classification_reasons,
        command_structure: command_structure,
        semantics,
        dangerous_match: dangerous_match,
        read_only: read_only,
        impact,
        reliability,
        reasons,
    }
}
