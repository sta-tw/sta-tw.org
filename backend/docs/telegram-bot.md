# Telegram 簡章／交叉查榜 Bot

`cmd/telegram-bot` 使用 Telegram long polling，不需要公開 webhook，也不需要啟動 Discord；用途是確認 Bot token、STA API、已上架簡章讀取鏈路，以及可選的交叉查榜意見通知。

核心交叉查榜仍由前端結果 API 提供；設定 `STA_TELEGRAM_CROSS_CHECK_TOKEN` 後，`cmd/api` 會額外註冊 `/api/v1/*/telegram-cross-check/*` adapter 路由。API 必須同時套用 `000025`／`000026` migration，Bot 才能使用交叉查榜功能。

它不是管理後台，不會從 Telegram 執行探索週期、上傳、發布或招生資料審核。所有會改變資料的操作仍由管理控制台與既有管理員認證／MFA／CSRF 邊界處理。

## 設定

```env
STA_TELEGRAM_BOT_TOKEN=<由 secret manager 注入>
STA_TELEGRAM_BACKEND_BASE_URL=http://localhost:8080
STA_TELEGRAM_API_BASE_URL=https://api.telegram.org
STA_TELEGRAM_BOT_ALLOWED_CHAT_IDS=
STA_TELEGRAM_POLL_TIMEOUT=25s
STA_TELEGRAM_CROSS_CHECK_TOKEN=<與 API 共用的內部 service bearer>
STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING=false
```

`STA_TELEGRAM_BOT_ALLOWED_CHAT_IDS` 可填逗號分隔的 Telegram chat ID；留白時只要知道 Bot username 的人都能觸發這幾個讀取指令，因此第一次測試建議先留白取得 `/id`，再填入允許清單並重新啟動。`STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING` 只應在隔離測試環境開啟。

## 本機測試

為避免載入根目錄的正式 `.env`，簡章／交叉查榜＋Telegram 的本機串接統一使用
[`test-env/brochure-tg`](../test-env/brochure-tg/README.md) 隔離環境。它使用獨立的
PostgreSQL、RabbitMQ、MinIO、ClamAV、連接埠與專用 `.env`。

若要驗收交叉查榜 adapter，請改用 [`test-env/cross-check-tg`](../test-env/cross-check-tg/README.md)，並先套用 `-include-telegram` migration。

先確認 API 已啟動，並且至少有一份簡章已由控制台審核上架：

```sh
cd test-env/brochure-tg
./run check
./run bot
```

若同一個 Bot 先前設定過 webhook，long polling 前先在受控終端移除它：

```sh
./run telegram-delete-webhook
```

Bot 啟動時會先呼叫 `getMe`，成功後才開始 `getUpdates`。日誌只寫入 Bot username 與後端 URL，不寫入 token。

在 Telegram 對 Bot 送出：

```text
/start
/id
/health
/brochure 116 001
```

交叉查榜 adapter 啟用後，另外可使用 `/start`、`/list`、`/pending`、`/status`、`/history`、`/stop`；Bot 只在私人聊天室處理個人查榜資料。

`/brochure` 只會查詢公開的 `GET /api/v1/admissions/brochures/{academicYear}/{schoolCode}/download`；如果簡章尚未上架，Bot 會回報找不到，不會繞過審核狀態。成功時回傳五分鐘有效的 signed URL，測試完成後不要把連結轉貼到公開頻道。

## 與既有 chat worker 的差異

既有 `cmd/chat-worker` 負責 canonical lounge 的 Discord／Telegram outbox 同步，需要固定目標 chat ID，並維持兩個平台的同步語意；`cmd/telegram-bot` 是獨立的簡章／交叉查榜入口，兩者可以分開啟動。若要回到正式跨平台閒聊流程，仍依 [deployment.md](deployment.md) 設定 `cmd/chat-worker`。
