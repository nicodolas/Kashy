# kashy-ui

Cross-platform desktop app for Kashy — AI spending monitor.

Built with [Tauri v2](https://tauri.app) + React + TypeScript.

## For end users

Download the installer from [GitHub Releases](https://github.com/nicodolas/kashy/releases/latest):

| Platform | File |
|---|---|
| Windows | `Kashy_x.y.z_x64-setup.exe` |
| macOS | `Kashy_x.y.z_aarch64.dmg` |
| Linux | `kashy_x.y.z_amd64.AppImage` |

Install and open — the proxy starts automatically. No CLI needed.

## How it works

```
User opens Kashy app
    │
    ▼ (auto-start on open)
kashy proxy :4000    ← intercepts LLM calls from AI agents
kashy status :4001   ← serves data to the UI
    │
    ▼
kashy-ui (Tauri)     ← polls :4001 every 3 seconds, shows spending
```

## For developers

### Prerequisites
- [Rust](https://rustup.rs/) (stable)
- [Node.js](https://nodejs.org/) 18+

### Setup

```bash
cd kashy-ui

# Copy kashy binary for sidecar (Windows)
mkdir binaries
copy %USERPROFILE%\bin\kashy.exe binaries\kashy-x86_64-pc-windows-msvc.exe

# Install JS deps
npm install
```

### Development

```bash
npm run tauri dev
```

The app opens, auto-starts the proxy, and hot-reloads on UI changes.

### Build installer

```bash
npm run tauri build
```

Output: `src-tauri/target/release/bundle/`

## Architecture

- **Frontend**: React + TypeScript, polls `http://localhost:4001` REST API
- **Backend**: Tauri (Rust), bundles `kashy` binary as sidecar
- **Sidecar**: `kashy start` spawned on app open, killed on app close
- **No Electron**: Uses system WebView — Windows (~5MB), macOS, Linux

## Icons

Generate proper icons with:
```bash
npm run tauri icon path/to/icon.png
```
