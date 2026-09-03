# STA Backend

STA 後端採 Go API 為核心，後續以 PostgreSQL 保存業務資料，Python Worker 負責簡章與榜單等資料處理。前端只依賴穩定的 `/api/v1` API，不綁定特定前端框架。

## 目前狀態

目前已完成 Phase 1～12（含本地文件擷取 queue／HTTP transport）的第一版核心實作：

- Go HTTP API 啟動與 graceful shutdown
- `/healthz`、`/readyz`、`/api/v1/meta`
- 精確 CORS allowlist，不允許 `*`
- request ID 與結構化日誌基礎
- JSON API body size limit
- 基本安全 response headers
- panic recovery，不將內部錯誤回傳給用戶端
- 設定值驗證與 production HTTPS origin 檢查

尚未接入資料庫的 `/readyz` 會視為 ready；資料庫加入後會改為檢查實際連線狀態。

Phase 2～3 已加入資料庫 migration 與認證核心：

- `migrations/000001_initial.sql`：帳號、OAuth 綁定、學校／科系／簡章、申請、客服單、稽核。
- `migrations/000002_auth_sessions.sql`：session、CSRF、Email challenge、OAuth state／PKCE。
- 原生帳號必須先建立；Google／Discord 未綁定時不會自動建立帳號。
- OAuth 綁定一個 provider 一個 STA 帳號一筆，第三方身份不能覆蓋原生身份。
- 正式啟用持久化 API 前必須設定 `STA_DATABASE_URL`、Email AES-GCM key 與 lookup HMAC key。

Phase 4～10 已加入：

- 招生資料查詢、Python 本地簡章／名單解析 worker（PDF、CSV、TSV、TXT、JSON）、RabbitMQ／HTTP durable job contract。
- 官方招生來源總清單、官方網域驗證、來源關聯證據與管理端狀態維護 API。
- 簡章上傳後的 durable extraction job、結果回寫、候選隔離與管理員審核 API；候選核准會原子建立待審招生資料，仍須經招生資料審核才可公開。
- 招生科系管理端批次／單筆維護、待審、審核上架與異動歷程 API；識別碼與定位編號仍由資料庫／輸入欄位自動生成。
- 招生建檔輸入只需提供原始欄位；`program_identifier`（`學年度-學校編號-類組`）與 `source_locator`（`學校編號-頁碼`）由資料庫自動產生。
- 官方簡章 private 檔案的上傳、待審、上架／下架與不可修改的上傳紀錄。
- 使用者申請鎖定、准考證 hash／加密、查榜參考機率與兩輪意願詢問。
- 備審檔案 private object storage、版本、送審、公開、下架與管理員審核。
- 論壇與心得文章 revision 審核；文章沒有留言，論壇可以引用文章。
- 網站 canonical 閒聊與 Discord／Telegram webhook、去重、編輯／刪除、outbox worker。
- 學校信箱白名單、在校證明上傳、人工審核、年度驗證結果與六月清理 command。
- 加密站內通知、Email outbox、SMTP notification worker 與 migration checksum runner。
- 通用客服 Ticket、網站 canonical 對話、Email 通知、Discord 私人頻道同步與關閉歷史紀錄。
- 客服網站附件、簽章 Email 回覆 webhook、管理員 TOTP MFA、跨副本限流與上傳檔案 ClamAV 掃描邊界。
- 外部文件擷取 API：人工或外部搜尋／AI 服務可上傳簡章與名單，Python 以 API 租約取得檔案並回傳本地規則擷取結果。
- 特殊選才簡章探索控制面：由管理端建立、啟動與結束學年度週期，以完整校院母表建立任務，提供外部 discovery agent lease、五種搜尋狀態、無簡章終止原因、候選 checksum 關聯、人工確認／補檔與 append-only 事件。
- 簡章探索工作程式：以固定 149 間學校名冊領取任務，透過搜尋服務尋找官方 PDF，限制公開 `.edu.tw`／`.gov.tw` 網址並以本地規則核對學校、動態學年度與文件類型。
- 簡章處理驗收控制台：可操作探索週期、逐校任務、候選確認、PDF 抽取明細、候選招生資料 JSON 修正與重試。
- Telegram-only 簡章 smoke bot：可用長輪詢驗證 Bot token、STA API healthz 與已上架簡章短效連結，不需要啟動 Discord worker。
- 交叉查榜核心與 Telegram adapter 分層；核心只提供前端使用的查榜／意願 API。設定 `STA_TELEGRAM_CROSS_CHECK_TOKEN` 並套用可選 migration 後，`cmd/api` 才會掛載 Telegram adapter。

Phase 13 為可上線化：CI（build／vet／golangci-lint／govulncheck／整合測試）、`Dockerfile` 與 `deploy/` 單機 compose、Prometheus `/metrics` 與 access log、深度 `/readyz`、worker graceful shutdown、outbox 重試上限與退避（migration `000027`）、RabbitMQ 斷線重連與有界 publish、`internal/dbtest` 整合測試骨架與 12 個 repository 的真 PostgreSQL 整合測試。整合測試過程中修掉數個 `jsonb_build_object($n)`／參數型別推斷過脆的 SQL（support ticket 建立、chat outbox、application service ticket、verification request、brochure discovery 事件），這些在預備語句模式下會直接回 42P08／42P18。

## 開發

```sh
cp .env.example .env
GOCACHE=/tmp/sta-go-cache go test ./...
GOCACHE=/tmp/sta-go-cache go vet ./...
GOCACHE=/tmp/sta-go-cache go build -buildvcs=false ./cmd/api ./cmd/chat-worker ./cmd/telegram-bot ./cmd/notification-worker ./cmd/support-worker ./cmd/ingestion-worker ./cmd/annual-maintenance ./cmd/bootstrap-admin ./cmd/migrate
GOCACHE=/tmp/sta-go-cache go run ./cmd/api
```

有資料庫時先執行 `go run ./cmd/migrate -dir migrations`；需要簡章／名單解析時可啟動 `cmd/ingestion-worker` 搭配 RabbitMQ，或以 `STA_EXTRACTION_TRANSPORT=api python -m worker.sta_worker.main` 改走 Go API 租約，不需要 RabbitMQ。需要跨平台閒聊同步時另啟動 `cmd/chat-worker`，需要 Telegram 簡章／交叉查榜測試時啟動 `cmd/telegram-bot`，需要客服 Discord 頻道同步時另啟動 `cmd/support-worker`，需要 Email／查榜提醒時另啟動 `cmd/notification-worker`。若要接回 Telegram 交叉查榜，API 與 Bot 都設定 `STA_TELEGRAM_CROSS_CHECK_TOKEN`，並以 `go run ./cmd/migrate -dir migrations -include-telegram` 套用 adapter schema；未設定 token 時核心查榜仍只使用下方結果 API。年度清理使用明確的 `cmd/annual-maintenance -academic-year <三位數學年度>`，不由程式猜測年份。

預設 API 位於 `http://localhost:8080`。開發環境預設允許的前端 origin 是 `http://localhost:3000`；正式環境必須設定明確的 HTTPS origin。

### 容器化執行

`Dockerfile` 建置單一 distroless 映像，內含全部 `cmd/*` binary 與 `migrations/`；`deploy/docker-compose.yml` 在單機起 PostgreSQL、MinIO、（可選）RabbitMQ、ClamAV、MailHog、SearXNG 加上 API 與各 worker：

```sh
cd deploy && cp .env.example .env
docker compose up -d --build            # HTTP 擷取 transport
docker compose --profile rabbitmq up -d --build   # 加上 RabbitMQ transport
```

`migrate` 會作為 `api` 的相依 init job 自動套用 migration。細節見 [deploy/README.md](deploy/README.md)。

### 可觀測性

- 每個 HTTP 請求輸出一行 JSON access log（method／route／status／latency／request_id）；inbound `X-Request-ID` 會沿用。
- `GET /metrics` 提供 Prometheus 指標（`sta_http_request_duration_seconds`、Go runtime、DB pool）。
- `GET /readyz` 回 `{"status","checks":{...}}`，逐一檢查 database，以及有設定時的 broker／object_storage／clamav，任一失敗回 503。
- API 與 Go worker 都處理 SIGTERM，在 `STA_SHUTDOWN_TIMEOUT` 內排空；Python worker 在下一個迴圈邊界停止。

### 整合測試

需要真實 PostgreSQL 的測試以 `//go:build integration` 標記，讀 `STA_TEST_DATABASE_URL`（未設定即 skip）。`internal/dbtest` 會以 migration 建立臨時 database、測完清除。

```sh
STA_TEST_DATABASE_URL='postgres://sta:sta@localhost:5432/sta_test?sslmode=disable' \
  go test -tags integration ./...
```

認證 API：

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/email-verification/resend`
- `POST /api/v1/auth/email-verification/confirm`（可用 Email token，不要求先登入；若帶既有 session，Cookie request 仍需 CSRF）
- `GET /api/v1/auth/admin-mfa/status`
- `POST /api/v1/auth/admin-mfa/setup`
- `POST /api/v1/auth/admin-mfa/enable`
- `POST /api/v1/auth/admin-mfa/disable`
- `GET /api/v1/auth/oauth/{google|discord}/start`
- `POST /api/v1/auth/oauth/{google|discord}/bind/start`
- `GET /api/v1/auth/oauth/{google|discord}/callback`

登入 session 使用 HttpOnly cookie；使用 cookie 呼叫會改變資料的 API 時，必須同時送出 CSRF cookie 與 `X-CSRF-Token` header。也支援 opaque bearer token 作為 API client 的認證方式。

學校主檔的公開模糊搜尋與管理端同步 API 見 [docs/school-master-api.md](docs/school-master-api.md)。管理端同步會以 transaction 更新資料並留下 append-only 稽核紀錄，因此日後可由管理後台或同步程式更新，不需要直接進伺服器修改 database。

論壇讀取 API 不要求登入；全域論壇公開可讀，年度／校系論壇只會回傳目前帳號有資格查看的範圍。發文、加入／離開論壇仍需登入與 CSRF／Bearer mutation authorization。確認申請時會在同一個 transaction 建立對應年度與校系論壇空間。

招生科系資料的管理端維護、審核與上架流程見 [docs/admission-program-api.md](docs/admission-program-api.md)；官方簡章檔案的上傳、審核、上架／下架與事件紀錄見 [docs/brochure-file-api.md](docs/brochure-file-api.md)；備審專案、檔案版本與審核流程見 [docs/portfolio-api.md](docs/portfolio-api.md)。

查榜結果批次、准考證匹配、參考機率與兩輪意願詢問流程見 [docs/results-api.md](docs/results-api.md)。

通用客服 Ticket、僅限註冊帳號使用的 `edu.tw` 信箱判定、Email 通知與 Discord 私人客服頻道設計見 [docs/support-system-design.md](docs/support-system-design.md)；追加申請仍使用既有的 `application_service_tickets` 流程。

文件上傳、Python 本地擷取、job claim／result callback 與外部搜尋／AI 服務串接方式見 [docs/extraction-api.md](docs/extraction-api.md)。系統不會從 Go API 或 Python worker 對外呼叫 AI endpoint。

官方招生來源總清單見 [docs/admission-source-registry.md](docs/admission-source-registry.md)。來源清單只控制可擷取的公開官方來源，不直接發布招生資料。

逐校簡章探索週期、狀態與候選回報契約見 [docs/brochure-discovery-api.md](docs/brochure-discovery-api.md)；控制台與 Telegram smoke bot 的操作方式見 [docs/telegram-bot.md](docs/telegram-bot.md)。此核心不依賴 Telegram，管理變更仍須經管理控制台與既有權限邊界。

學生身份驗證、學校信箱白名單、在校證明人工審核與管理員短效下載入口見 [docs/verification-api.md](docs/verification-api.md)。

## 安全邊界

- `.env` 不進版本控制；第三方憑證與 token 不寫入程式碼。
- CORS 只接受明確列出的 origin。
- API 回應不直接暴露 readiness 或其他內部錯誤內容。
- 上傳檔案、內容審核、查榜、通知、跨平台同步與管理員權限均有各自的審核狀態與測試 gate；正式上線前仍須實際套用 migration、做依賴／滲透掃描、維護 ClamAV 與完成備份還原演練。

部署與備份流程見 [docs/deployment.md](docs/deployment.md) 與 [docs/backup-restore.md](docs/backup-restore.md)，資安邊界見 [docs/security.md](docs/security.md)。
