#![allow(dead_code)] // Ported-from-reference interfaces, some not yet wired into the protocol; kept for later stages.
pub const PROTOCOL_VERSION: u32 = 1;

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ShellKind {
    Bash,
    Zsh,
    Sh,
    PowerShell,
    Cmd,
    Nushell,
    Unknown,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Classification {
    Safe,
    Dangerous,
    Unknown,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Reliability {
    High,
    Medium,
    Low,
}

#[derive(Debug, Clone, serde::Deserialize)]
pub struct AnalyzeRequest {
    pub command: String,
    pub shell: ShellKind,
    #[serde(default)]
    pub workdir: String,
}

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AnalyzeResponse {
    pub version: u32,
    pub shell: ShellKind,
    pub classification: Classification,
    pub classification_reasons: Vec<String>,
    pub command_structure: CommandStructure,
    pub semantics: SemanticsReport,
    pub dangerous_match: Option<DangerousMatch>,
    pub read_only: ReadOnlyReport,
    pub impact: ImpactReport,
    pub reliability: Reliability,
    pub reasons: Vec<String>,
}

#[derive(Debug, Clone, Default, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CommandStructure {
    pub commands: Vec<CommandNode>,
    pub operators: Vec<String>,
    pub parse_error: bool,
    /// OpenCode BashArity: the "human-understandable command" prefix of the
    /// first command (e.g. `git checkout` for `git checkout main`), used by
    /// the permission layer as the memorable allow-prefix.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub arity_prefix: String,
}

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CommandNode {
    pub argv: Vec<String>,
    pub text: String,
    pub dynamic: bool,
}

#[derive(Debug, Clone, Default, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SemanticsReport {
    pub dangerous: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub reasons: Vec<String>,
}

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DangerousMatch {
    #[serde(rename = "kind")]
    pub kind: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub detail: String,
}

#[derive(Debug, Clone, Default, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ReadOnlyReport {
    pub verified: bool,
    pub verified_command: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub unsafe_flags: Vec<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub reason: String,
}

#[derive(Debug, Clone, Default, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ImpactReport {
    pub items: Vec<ImpactItem>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub dangerous_removal_paths: Vec<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub unresolved: Vec<String>,
}

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ImpactItem {
    pub path: String,
    pub effects: Vec<String>,
    pub outside_workdir: bool,
    pub dynamic: bool,
    pub kind: String,
}

impl ImpactItem {
    pub fn new(path: String, kind: &str) -> Self {
        Self {
            path,
            effects: Vec::new(),
            outside_workdir: false,
            dynamic: false,
            kind: kind.to_string(),
        }
    }
}

#[derive(Debug, Clone, Default)]
pub struct InternalCommand {
    pub argv: Vec<String>,
    pub text: String,
    pub env_vars: Vec<(String, String)>,
    pub redirects: Vec<InternalRedirect>,
    pub dynamic: bool,
}

#[derive(Debug, Clone)]
pub struct InternalRedirect {
    pub op: String,
    pub target: String,
    pub fd: Option<u32>,
    pub dynamic: bool,
}
