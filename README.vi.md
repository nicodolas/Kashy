# Kashy

> Theo dõi chi phí AI để bạn không phải lo.

Kashy là một proxy nhẹ nằm giữa AI agent và LLM API của bạn. Mỗi token đều được ghi lại, mỗi đồng đều hiện ra, và giới hạn ngân sách được áp dụng trước khi hoá đơn đến.

**Một file binary duy nhất. ~10 MB. ~15 MB RAM khi chờ. Không cần runtime.**

🌐 [Read in English](README.md)

---

## Vấn đề

Lập trình viên dùng BYOK (Bring Your Own Key) với OpenRouter hoặc bất kỳ API tương thích OpenAI nào không có cách theo dõi chi phí thời gian thực. Hoá đơn đến *sau khi* tiền đã tiêu. Kashy chặn mỗi cuộc gọi và hiển thị chi phí ngay khi nó xảy ra.

---

## Cài đặt

### Cách A — Tải binary có sẵn (khuyến nghị)

Vào [Releases](https://github.com/nicodolas/kashy/releases/latest) và tải zip cho nền tảng của bạn:

| Nền tảng | File |
|---|---|
| Windows | `kashy_vX.Y.Z_windows_amd64.zip` |
| macOS | `kashy_vX.Y.Z_darwin_amd64.zip` |
| Linux | `kashy_vX.Y.Z_linux_amd64.zip` |

Giải nén và thêm vào PATH:

**Windows (PowerShell):**
```powershell
# Giải nén kashy.exe từ zip, sau đó:
$bin = "$env:USERPROFILE\bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Copy-Item kashy.exe "$bin\kashy.exe"
$p = [Environment]::GetEnvironmentVariable("PATH","User")
if ($p -notlike "*$bin*") {
    [Environment]::SetEnvironmentVariable("PATH","$p;$bin","User")
}
# Mở terminal mới, sau đó: kashy --version
```

**Linux / macOS:**
```bash
unzip kashy_vX.Y.Z_*.zip
sudo mv kashy /usr/local/bin/
kashy --version
```

### Cách B — Build từ source

**Yêu cầu:** [Go 1.21+](https://go.dev/dl/)

```bash
git clone https://github.com/nicodolas/kashy
cd kashy
go build -o kashy ./cmd/kashy/        # Linux / macOS
go build -o kashy.exe ./cmd/kashy/    # Windows
```

### Cập nhật

Sau khi đã cài, cập nhật lên phiên bản mới nhất bằng:

```bash
kashy update
```

---

## Bắt đầu nhanh (< 5 phút)

### 1 — Đặt API key

```bash
kashy config set-key sk-or-...
```

Lấy key miễn phí tại [openrouter.ai/keys](https://openrouter.ai/keys)

### 2 — Khởi động Kashy

```bash
kashy start
# [kashy] proxy started  →  http://localhost:4000/v1
# [kashy] status server  →  http://localhost:4001/status
```

### 3 — Trỏ agent của bạn về Kashy

| Agent | Cài đặt |
|---|---|
| **OMX** (`.codex/config.toml`) | `openai_base_url = "http://localhost:4000/v1"` |
| **OpenCode** (`opencode.json`) | `{ "provider": { "openrouter": { "options": { "baseURL": "http://localhost:4000/v1" } } } }` |
| **Bất kỳ agent tương thích OpenAI** | Đặt base URL thành `http://localhost:4000/v1` |

Hoặc để Kashy tự vá tất cả:

```bash
kashy doctor --fix
```

### 4 — Theo dõi chi phí

```bash
kashy status     # phiên hiện tại: token, chi phí, thanh ngân sách
kashy history    # chi phí theo ngày (30 ngày gần nhất)
kashy balance    # số dư tài khoản OpenRouter thời gian thực
```

---

## Các lệnh

| Lệnh | Mô tả |
|---|---|
| `kashy start` | Khởi động proxy (:4000) + status server (:4001) |
| `kashy stop` | Dừng daemon đang chạy |
| `kashy status` | Chi phí phiên hiện tại + thanh ngân sách ASCII |
| `kashy history` | Chi phí theo ngày (30 ngày gần nhất) |
| `kashy balance` | Chi tiêu OpenRouter thời gian thực: hôm nay / tuần / tháng |
| `kashy update` | Cập nhật Kashy lên phiên bản mới nhất |
| `kashy config set-key <key>` | Đặt OpenRouter API key |
| `kashy config set-budget <usd>` | Đặt giới hạn hard-stop theo phiên (USD) |
| `kashy config show` | Hiện cấu hình hiện tại |
| `kashy doctor` | Kiểm tra agent nào đang kết nối với Kashy |
| `kashy doctor --fix` | Tự động vá file cấu hình agent |
| `kashy mcp` | Khởi động như MCP stdio server |

---

## Tích hợp MCP

Thêm Kashy như một MCP tool server để agent của bạn có thể truy vấn dữ liệu chi phí trực tiếp:

```json
{
  "mcpServers": {
    "kashy": {
      "command": "kashy",
      "args": ["mcp"]
    }
  }
}
```

### Tích hợp với Kiro

1. Mở `~/.kiro/settings/mcp.json` (tạo mới nếu chưa có)
2. Thêm cấu hình kashy:

```json
{
  "mcpServers": {
    "kashy": {
      "type": "stdio",
      "command": "kashy",
      "args": ["mcp"],
      "autoApprove": [
        "kashy_cost_status",
        "kashy_cost_history",
        "kashy_verify_done",
        "kashy_reset_budget"
      ]
    }
  }
}
```

3. Reload MCP servers trong Kiro (Command Palette → "MCP: Restart Servers")
4. Kiểm tra kết nối: `kashy doctor`

**Các MCP tool:**

| Tool | Mô tả |
|---|---|
| `kashy_cost_status` | Chi phí phiên hiện tại, token, trạng thái ngân sách |
| `kashy_cost_history` | N mục lịch sử gần nhất (mặc định 10, tối đa 50) |
| `kashy_verify_done` | Chạy tests + LLM review; trả về AUTO-CHECKED / NEEDS-REVIEW / FAILED / NO-TESTS |
| `kashy_reset_budget` | Reset bộ đếm chi phí phiên về $0.00 |
| `kashy_budget_remaining` | Chi tiêu OpenRouter thời gian thực (hôm nay / tuần / tháng) |
| `kashy_account_usage` | Thông tin key đầy đủ: giới hạn, free tier, rate limit |

---

## Cấu hình

File cấu hình: `~/.kashy/config.toml`

```toml
[providers.openrouter]
api_key  = "sk-or-..."
base_url = "https://openrouter.ai/api/v1"

[budget]
session_hard_stop = 1.00   # USD — trả HTTP 429 khi vượt giới hạn
warn_at           = 0.80   # cảnh báo ở 80% qua header X-Kashy-Budget-Warning
daily_limit       = 10.00  # USD — reset lúc nửa đêm (giờ địa phương)

[loop]
default_model = "anthropic/claude-3-haiku"
max_iter      = 50
```

**Biến môi trường** (ghi đè file cấu hình):

| Biến | Tác dụng |
|---|---|
| `OPENROUTER_API_KEY` | Ghi đè `api_key` |
| `KASHY_CONFIG` | Ghi đè đường dẫn file cấu hình |

---

## Cách hoạt động

```
AI Agent của bạn
      │
      │  POST /v1/chat/completions
      ▼
┌──────────────────────┐
│  Kashy Proxy :4000   │  ← kiểm tra ngân sách phiên & giới hạn ngày
│                      │  ← chèn Authorization header
│                      │  ← trích xuất token usage từ response
│                      │  ← ghi chi phí vào ~/.kashy/history.jsonl
└─────────┬────────────┘
          │
          │  chuyển tiếp request
          ▼
    OpenRouter / LLM API
```

Khi ngân sách phiên bị vượt, Kashy trả HTTP 429 ngay lập tức — LLM không bao giờ được gọi, nên bạn không tốn thêm tiền.

---

## Giấy phép

MIT — xem [LICENSE](LICENSE)
