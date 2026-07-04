#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|_app| {
            // Proxy is started from the frontend (App.tsx) via sidecar.
            // Nothing to do here — keeping setup minimal.
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running kashy-ui");
}
