# STA admission worker

Python worker 只負責檔案資料擷取，不呼叫任何 AI endpoint。Go API 負責上傳驗證、ClamAV、private object storage、job lease 與結果保存；Python 以本地規則解析簡章與准考證／考生名單，簡章遇到文字層不足的頁面時會 fallback 到本地 Tesseract OCR，結果仍須管理員審核後才會建立或發布資料。

## RabbitMQ 模式

```sh
python -m venv .venv
. .venv/bin/activate
pip install -r worker/requirements.txt
STA_RABBITMQ_URL=amqp://sta:change-me@localhost:5672/%2f \
STA_EXTRACTION_TRANSPORT=rabbitmq \
STA_WORKER_OBJECT_STORAGE_ENDPOINT=localhost:9000 \
STA_WORKER_OBJECT_STORAGE_ACCESS_KEY=change-me \
STA_WORKER_OBJECT_STORAGE_SECRET_KEY=change-me \
STA_WORKER_OBJECT_STORAGE_BUCKET=sta-private \
python -m worker.sta_worker.main
```

此模式由 Go API 將 `brochure` 或 `candidate_list` 工作送進 RabbitMQ；Python 從 private object storage 讀取來源，再把 `brochure` 結果送回簡章結果 queue，`candidate_list` 結果送回名單結果 queue。

## API 模式（不需要 RabbitMQ）

```sh
STA_EXTRACTION_TRANSPORT=api \
STA_EXTRACTION_API_BASE_URL=http://localhost:8080 \
STA_EXTRACTION_SERVICE_TOKEN='<至少32字元的服務憑證>' \
python -m worker.sta_worker.main
```

API 模式不需要 MinIO／S3 credentials。worker 從 `POST /api/v1/internal/extraction/jobs/claim` 取得五分鐘 signed download URL，checksum 驗證後在暫存目錄解析，再呼叫 `/result` 或 `/failure` callback。這是人工上傳與外部 AI／搜尋服務送檔後的建議部署方式。

## 環境變數

- `STA_EXTRACTION_TRANSPORT`：`api`（預設）或相容的 `rabbitmq`。
- `STA_EXTRACTION_SERVICE_TOKEN`：API 模式必填，至少 32 字元；可用 `STA_EXTERNAL_INGESTION_TOKEN` 相容別名。
- `STA_EXTRACTION_API_BASE_URL`：API 模式的 Go API 位址，預設 `http://localhost:8080`。
- `STA_EXTRACTION_API_TIMEOUT`：API request／signed URL timeout，預設 `60s`。
- `STA_EXTRACTION_POLL_INTERVAL`：API 沒有工作時的輪詢間隔，預設 `5s`。
- `STA_RABBITMQ_URL`：RabbitMQ 模式必填。
- `STA_WORKER_DOCUMENT_ROOT`：RabbitMQ 模式可選的共享檔案根目錄；未設定時使用下列 private object storage 設定。
- `STA_WORKER_OBJECT_STORAGE_ENDPOINT`、`STA_WORKER_OBJECT_STORAGE_ACCESS_KEY`、`STA_WORKER_OBJECT_STORAGE_SECRET_KEY`、`STA_WORKER_OBJECT_STORAGE_BUCKET`：RabbitMQ 模式使用 object storage 時必填。
- `STA_WORKER_OBJECT_STORAGE_USE_SSL`：object storage 是否使用 TLS，預設 `false`。
- `STA_WORKER_PROCESSOR_VERSION`：本地解析器版本，預設 `local-extraction-v1`；需與 Go API `internal/ingestion.DefaultProcessor` 一致。
- `STA_WORKER_MAX_FILE_BYTES`：檔案大小上限，預設 50 MiB。
- `STA_WORKER_OCR_ENABLED`：是否啟用簡章 PDF OCR fallback，預設 `true`。
- `STA_WORKER_OCR_LANG`：Tesseract 語言，預設 `chi_tra+eng`。
- `STA_WORKER_OCR_DPI`：PDF 渲染解析度，預設 `250`。
- `STA_WORKER_OCR_MIN_TEXT_CHARS`：每頁文字少於此數量才啟用 OCR，預設 `40`。
- `STA_WORKER_OCR_MAX_PAGES`：單份文件最多 OCR 頁數，預設 `300`；超過會保留失敗狀態供管理端處理。
- `STA_WORKER_OCR_TIMEOUT`：單頁渲染／OCR timeout 秒數，預設 `120`。

### Cloudflare Workers AI 簡章擷取（選配）

設定 `STA_WORKER_AI_ENABLED=true` 並提供 Cloudflare 憑證後，簡章擷取改由 Workers AI 聊天模型分析在本地擷取／OCR 出來的 PDF 文字，回傳結構化的校系清單；本地規則解析器保留為 fallback（AI 關閉、API 連不上、或回傳無法使用時自動改用規則）。結果一樣是待審 candidate，管理員仍須逐欄確認。

- `STA_WORKER_AI_ENABLED`：`true` 才啟用，預設關閉。
- `STA_WORKER_AI_ACCOUNT_ID`：Cloudflare account id。
- `STA_WORKER_AI_API_TOKEN`：具 Workers AI 權限的 API token（相容別名 `STA_WORKER_AI_TOKEN`）。
- `STA_WORKER_AI_MODEL`：Workers AI 模型，預設 `@cf/mistralai/mistral-small-3.1-24b-instruct`（大 context、快、非 reasoning）。小 context 模型（如 `@cf/meta/llama-3.3-70b-instruct-fp8-fast` 只有 24k）請把 `STA_WORKER_AI_MAX_INPUT_CHARS` 調到 12000 以下。
- `STA_WORKER_AI_BASE_URL`：預設 `https://api.cloudflare.com/client/v4`。
- `STA_WORKER_AI_TIMEOUT`：單次請求 timeout 秒數，預設 `120`。
- `STA_WORKER_AI_MAX_INPUT_CHARS`：每次送給模型的文字上限，預設 `40000`（超過會分段）。若 prompt 仍超過模型 context，API 回 4xx，worker 不重試、直接改用規則路徑。
- `STA_WORKER_AI_MAX_OUTPUT_TOKENS`：回應 token 上限，預設 `2048`。
- `STA_WORKER_AI_MAX_CHUNKS`：單份簡章最多分段呼叫次數，預設 `12`（控制成本）。
- `STA_WORKER_AI_CONFIDENCE`：AI candidate 的信心度，預設 `0.3`。

簡章本地擷取會從 PDF 文字或 OCR 文字抽取學年度、學校代碼／名稱、招生學系、組別、招生名額、報名／考試／放榜日期、考試項目與頁面證據；考試項目優先取每個初審／複審／階段的官方總比重，階段內的指定資料或評量細項合併到項目說明，不把階段內分數誤當成整體比重；沒有階段結構時，才解析帶有比重的平面列。簡章通常不提供系統內部校系代碼，造冊時會依各招生學系在文件中的出現順序產生 `001`、`002` 等三位數代碼。名單本地擷取會從 PDF、CSV、TSV、TXT 或 JSON 抽取准考證號、姓名遮罩、系所代碼、錄取狀態、名次、名額與頁碼。管理員直接上傳的簡章會在前端開啟原始 PDF 與建檔欄位，確認後才一次建立並公開簡章與校系資料；外部簡章 API 送入的來源則隔離在 `external_api` 的 `pending_review` 佇列，供人工複核後上架。OCR 只處理文字層不足的簡章頁面，以控制 CPU 與處理時間。

失敗工作會透過 RabbitMQ dead-letter 或 API `/failure` 留下錯誤；暫時性錯誤最多重試五次，格式／內容錯誤則保留為 `failed` 供管理端處理。

## 簡章探索工作程式（選配）

逐校探索是獨立 process，不直接接觸 PostgreSQL 或 object storage。它向 Go API 領取目前 active 學年度任務，透過自架 SearXNG 尋找候選，僅下載 `.edu.tw`／`.gov.tw` 公開網址，並以本地規則核對學校、學年度與「特殊選才招生簡章」，最後把候選 PDF 送回 Go API。

```sh
STA_BROCHURE_DISCOVERY_API_BASE_URL=http://localhost:8080 \
STA_BROCHURE_DISCOVERY_AGENT_TOKEN='<至少32字元的服務憑證>' \
STA_SEARXNG_URL=http://localhost:8888 \
STA_SEARXNG_LANGUAGE=zh-TW \
python -m worker.sta_worker.discovery
```

探索也不需要 AI 金鑰；官方網域限制、TLS 憑證、公開 IP SSRF 防護與管理員確認仍由程式執行。若 PDF 文字不足以本地確認，worker 會保守回報無候選，等待人工補檔或下一次搜尋。
