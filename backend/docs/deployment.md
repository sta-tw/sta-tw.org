# STA 部署與執行

STA 的正式執行單位分成七個常駐 process，加上一個排程 command：

- `cmd/api`：只處理 HTTP API 與資料庫交易。
- `cmd/ingestion-worker`：RabbitMQ transport 的相容結果 consumer，將 Python 簡章／名單結果寫入隔離資料表；API transport 不需要啟動它。
- `cmd/chat-worker`：讀取 chat outbox，送往 Discord／Telegram。
- `cmd/telegram-bot`：可選的 Telegram-only 長輪詢 Bot，用來驗證已上架簡章查詢；設定 cross-check service token 後也會派送交叉查榜詢問。不取代 `cmd/chat-worker` 的跨平台閒聊同步。
- `cmd/support-worker`：建立客服 Ticket 的 Discord 私人頻道、同步客服訊息並監聽 Discord Gateway 回覆。
- `cmd/notification-worker`：處理查榜提醒、站內通知與 Email outbox。
- `python -m worker.sta_worker.discovery`：從固定學校名冊搜尋官方簡章並回報候選，不直接連線資料庫。
- `cmd/annual-maintenance`：每年六月由排程器明確指定要封存的學年度，只執行一次。

## 啟動順序

1. PostgreSQL、private MinIO/S3 與 SMTP 先啟動；只有使用相容 RabbitMQ transport 時才需要 RabbitMQ。
2. 使用 migration command 套用資料庫版本：

   ```sh
   go run ./cmd/migrate -dir migrations
   ```

   先用一般註冊流程建立一個管理員帳號，再由受控部署環境執行一次：

   ```sh
   go run ./cmd/bootstrap-admin -username <existing-username>
   ```

   這個 command 只會將既有 active 帳號加入 `admin` role，不接受密碼、不建立新帳號，也不應暴露給公網。

3. 啟動 API、Python 擷取 worker 與簡章探索工作程式。API transport 下，Python worker 透過 Go API claim／download／result callback 工作；worker 可以水平擴充，資料庫 lease 會避免重複領取。

   ```sh
   go run ./cmd/api
   go run ./cmd/chat-worker
   go run ./cmd/telegram-bot # optional brochure／cross-check Telegram adapter
   go run ./cmd/support-worker
   go run ./cmd/notification-worker
   STA_EXTRACTION_TRANSPORT=api \
   STA_EXTRACTION_API_BASE_URL=http://localhost:8080 \
   STA_EXTRACTION_SERVICE_TOKEN='<至少32字元的服務憑證>' \
   python -m worker.sta_worker.main
   python -m worker.sta_worker.discovery
   ```

   Python worker 以 `worker/sta_worker/main.py` 的 API transport 消費簡章／名單工作；它不需要資料庫、RabbitMQ 或 object-storage credentials，來源由 Go API 透過短效 signed URL 提供。

   若要使用 RabbitMQ 相容模式，另外啟動 `go run ./cmd/ingestion-worker`，並讓 Python worker 設定 `STA_EXTRACTION_TRANSPORT=rabbitmq` 與 `STA_WORKER_OBJECT_STORAGE_*`（或共享檔案根目錄）。兩種 transport 不可同時消費同一部署的工作。完整流程見 [extraction-api.md](extraction-api.md)；服務憑證只可呼叫對應的 internal ingestion routes，不能取代管理員登入。

   先做 Telegram 簡章讀取串接驗證時，只需要設定 `STA_TELEGRAM_BOT_TOKEN`、`STA_TELEGRAM_BACKEND_BASE_URL`；若要接回交叉查榜，API 與 Bot 另外設定同一個 `STA_TELEGRAM_CROSS_CHECK_TOKEN`，並使用 `go run ./cmd/migrate -dir migrations -include-telegram`。完整測試與指令見 [telegram-bot.md](telegram-bot.md)。長輪詢啟動前必須先移除同一個 Bot 的既有 webhook。

   上述 migration 指令預設只建立核心 schema；啟用 Telegram cross-check adapter 的環境才明確使用 `go run ./cmd/migrate -dir migrations -include-telegram`。

4. 每年六月由受控排程執行，例如封存 115 學年度：

   ```sh
   go run ./cmd/annual-maintenance -academic-year 115
   ```

   這個 command 會先移除 private object storage 的學生證明，再刪除該學年度的驗證申請與一次性驗證資料；管理員維護的學校信箱白名單不會被清掉。帳號、申請、年度／學校／科系及已驗證結果會保留，過期結果轉為非 active。執行紀錄在 `annual_maintenance_runs`，同一學年度重跑是冪等的。

## 網路邊界

- API 前方必須有 TLS reverse proxy；正式 `STA_ALLOWED_ORIGINS` 只列明確的 HTTPS origin。
- PostgreSQL、RabbitMQ、MinIO、SMTP 不暴露給瀏覽器或公網。
- MinIO bucket 必須 private；檔案只由 API 產生短效 signed URL。
- Discord／Telegram webhook 只接受 `X-STA-Signature` HMAC；Bot token 只放 worker secret。
- `cmd/support-worker` 的 Discord bot 必須啟用 Guild Messages 與 Message Content privileged intent；客服分類與客服角色 ID 必須使用私有 Discord 設定。
- `/readyz` 必須接到負載平衡器的 readiness probe；`/healthz` 只代表 process 存活。

正式環境要把 `.env` 改成 secret manager 注入，並限制 API、worker、migration command 的資料庫權限。API 啟用 PostgreSQL、private object storage 時，必須同時設定 `STA_CLAMAV_ADDRESS`；管理員首次登入後依 [admin-mfa.md](admin-mfa.md) 完成 TOTP 啟用。實際上線前仍需完成依賴掃描、TLS 憑證輪替、migration checksum 檢查、備份還原演練與監控告警驗證。
