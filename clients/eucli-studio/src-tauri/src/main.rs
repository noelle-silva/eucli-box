#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::Serialize;
use std::path::{Path, PathBuf};
use tauri::Manager;

const CONFIG_FILE: &str = "ai-studio-settings.json";
const WRITE_TEST_FILE: &str = ".fw-ai-studio-write-test";

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct DataDirStatus {
    data_dir: String,
    default_data_dir: String,
    configured_data_dir: Option<String>,
    writable: bool,
    error: Option<String>,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct FwLaunchInfo {
    launched: bool,
    standalone: bool,
    mode: String,
}

#[tauri::command]
fn fw_launch_info() -> FwLaunchInfo {
    FwLaunchInfo {
        launched: false,
        standalone: true,
        mode: "standalone".to_string(),
    }
}

#[tauri::command]
fn fw_initial_command() -> Option<String> {
    None
}

#[tauri::command]
fn app_ready() {}

#[tauri::command]
fn hide_to_tray(window: tauri::WebviewWindow) -> Result<(), String> {
    window.hide().map_err(|e| format!("隐藏窗口失败: {e}"))
}

#[tauri::command]
fn data_dir_status(app: tauri::AppHandle) -> Result<DataDirStatus, String> {
    let configured = load_data_dir_setting(&app)?;
    let default_data_dir = default_data_dir(&app)?;
    let data_dir = configured.clone().unwrap_or_else(|| default_data_dir.clone());
    let writable_result = ensure_writable_dir(&data_dir);
    let writable_error = writable_result.as_ref().err().cloned();
    Ok(DataDirStatus {
        data_dir: data_dir.display().to_string(),
        default_data_dir: default_data_dir.display().to_string(),
        configured_data_dir: configured.map(|path| path.display().to_string()),
        writable: writable_result.is_ok(),
        error: writable_error,
    })
}

#[tauri::command]
fn pick_data_dir(app: tauri::AppHandle) -> Result<Option<DataDirStatus>, String> {
    let folder = rfd::FileDialog::new()
        .set_title("选择 AI Studio 数据目录")
        .pick_folder();

    let Some(path) = folder else {
        return Ok(None);
    };

    save_data_dir_setting(&app, &path)?;
    Ok(Some(data_dir_status(app)?))
}

fn default_data_dir(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    app.path()
        .app_data_dir()
        .map(|dir| dir.join("data"))
        .map_err(|e| format!("读取默认数据目录失败: {e}"))
}

fn settings_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    app.path()
        .app_config_dir()
        .map(|dir| dir.join(CONFIG_FILE))
        .map_err(|e| format!("读取 App 配置目录失败: {e}"))
}

fn load_data_dir_setting(app: &tauri::AppHandle) -> Result<Option<PathBuf>, String> {
    let path = settings_path(app)?;
    if !path.is_file() {
        return Ok(None);
    }
    let text = std::fs::read_to_string(&path).map_err(|e| format!("读取数据目录配置失败: {e}"))?;
    let value: serde_json::Value = serde_json::from_str(&text).map_err(|e| format!("解析数据目录配置失败: {e}"))?;
    let data_dir = value
        .get("dataDir")
        .and_then(|v| v.as_str())
        .map(|s| s.trim())
        .filter(|s| !s.is_empty())
        .map(PathBuf::from);
    Ok(data_dir)
}

fn save_data_dir_setting(app: &tauri::AppHandle, data_dir: &Path) -> Result<(), String> {
    ensure_writable_dir(data_dir)?;
    let path = settings_path(app)?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| format!("创建配置目录失败: {e}"))?;
    }
    let payload = serde_json::json!({ "dataDir": data_dir.display().to_string() });
    let text = serde_json::to_string_pretty(&payload).map_err(|e| format!("序列化配置失败: {e}"))?;
    std::fs::write(path, format!("{text}\n")).map_err(|e| format!("保存数据目录配置失败: {e}"))
}

fn ensure_writable_dir(path: &Path) -> Result<(), String> {
    std::fs::create_dir_all(path).map_err(|e| format!("数据目录不可创建: {} ({e})", path.display()))?;
    let test_path = path.join(WRITE_TEST_FILE);
    std::fs::write(&test_path, b"ok").map_err(|e| format!("数据目录不可写: {} ({e})", path.display()))?;
    let _ = std::fs::remove_file(test_path);
    Ok(())
}

fn main() {
    #[cfg(debug_assertions)]
    let context = tauri::generate_context!("tauri.conf.dev.json");
    #[cfg(not(debug_assertions))]
    let context = tauri::generate_context!("tauri.conf.json");

    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            fw_launch_info,
            fw_initial_command,
            app_ready,
            hide_to_tray,
            data_dir_status,
            pick_data_dir
        ])
        .run(context)
        .expect("error while running AI Studio app");
}
