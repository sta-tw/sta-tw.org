# STA admission worker

Python worker 只負責檔案資料擷取，不呼叫任何 AI endpoint。Go API 負責上傳驗證、ClamAV、private object storage、job lease 與結果保存；Python 以本地規則解析簡章與准考證／考生名單，結果仍須管理員審核後才會建立或發布資料。

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

簡章本地擷取會從 PDF 文字抽取校系代碼、校系名稱、招生名額、報名／考試／放榜日期、考試項目與頁面證據；名單本地擷取會從 PDF、CSV、TSV、TXT 或 JSON 抽取准考證號、姓名遮罩、系所代碼、錄取狀態、名次、名額與頁碼。解析結果只進隔離的 pending review／pending result batch，不會直接公開。

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
