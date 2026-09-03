# 文件擷取 API

STA 的文件擷取流程不依賴任何內建 AI 服務。Go API 負責驗證來源、掃描檔案、寫入 private object storage、建立 durable job 與保存結果；外部 Python worker 只負責下載檔案、用本地規則解析，再回傳結果。外部 AI／搜尋服務若找到檔案，也只需把原始檔送到下列 API。

## 使用方式

人工上傳：

- 簡章：`POST /api/v1/admin/admissions/brochures`（既有管理端簡章流程）。
- 名單：`POST /api/v1/admin/extraction/candidate-lists`。
- 工作狀態：`GET /api/v1/admin/ingestion/jobs/{job_id}`。

外部 AI／搜尋服務上傳：

- 簡章：`POST /api/v1/internal/extraction/brochures`。
- 名單：`POST /api/v1/internal/extraction/candidate-lists`。

外部路由使用 `Authorization: Bearer $STA_EXTRACTION_SERVICE_TOKEN`。它只能建立待擷取來源與回報結果，不能審核、上架或直接寫入公開招生資料。所有檔案都必須使用 multipart 欄位：

```text
academic_year  三位數學年度，例如 115
school_code    三位數學校代碼
program_code   可選；名單若檔案沒有系所代碼就必填
source_url     可選；有值時必須是 .edu.tw 或 .gov.tw
file           原始 PDF／CSV／TSV／TXT／JSON
```

簡章 API 只接受 PDF；名單 API 接受 PDF、CSV、TSV、純文字與 JSON。兩者都會先經檔案大小、檔名、內容簽名與 ClamAV（若啟用）檢查。

例如外部搜尋／AI agent 找到官方檔案後，直接把原始檔交給平台：

```sh
curl -X POST http://localhost:8080/api/v1/internal/extraction/brochures \
  -H "Authorization: Bearer $STA_EXTRACTION_SERVICE_TOKEN" \
  -F academic_year=115 \
  -F school_code=001 \
  -F source_url=https://admission.example.edu.tw/115/brochure.pdf \
  -F file=@brochure.pdf

curl -X POST http://localhost:8080/api/v1/internal/extraction/candidate-lists \
  -H "Authorization: Bearer $STA_EXTRACTION_SERVICE_TOKEN" \
  -F academic_year=115 \
  -F school_code=001 \
  -F program_code=013 \
  -F source_url=https://admission.example.edu.tw/115/result.pdf \
  -F file=@candidate-list.pdf
```

人工流程則使用同樣的欄位，將路徑換成 `/api/v1/admin/admissions/brochures` 或 `/api/v1/admin/extraction/candidate-lists`，以管理員 session／CSRF 認證送出。

## HTTP Python worker

不部署 RabbitMQ 時，Python worker 可直接使用 API 租約模式：

```sh
STA_EXTRACTION_TRANSPORT=api \
STA_EXTRACTION_API_BASE_URL=http://localhost:8080 \
STA_EXTRACTION_SERVICE_TOKEN='<至少32字元的服務憑證>' \
python -m worker.sta_worker.main
```

worker 會輪詢兩種文件類型：

1. `POST /api/v1/internal/extraction/jobs/claim`，body 為 `{"document_type":"brochure"}` 或 `{"document_type":"candidate_list"}`。
2. API 回傳 job 與五分鐘有效的 `download_url`；沒有工作時回傳 `204 No Content`。
3. worker 以 checksum 驗證下載檔案，使用本地 PDF／表格／JSON 規則抽取。
4. `POST /api/v1/internal/extraction/jobs/{job_id}/result` 回傳結果。
5. 解析失敗時回傳 `POST /api/v1/internal/extraction/jobs/{job_id}/failure`；暫時性錯誤會在 30 秒後重試，格式或內容錯誤會標記為 `failed`。

名單結果 callback 的 body 上限為 16 MiB（一般 JSON API 仍使用 `STA_MAX_JSON_BODY_BYTES`）；名單最多 10,000 筆。

也可以繼續使用 RabbitMQ transport；兩種 transport 不應同時消費同一組部署的工作，避免不必要的競爭。

## 結果保存邊界

簡章結果會寫入 `brochure_extraction_runs`／`brochure_extraction_candidates`，維持 `pending_review`，管理員確認後才能建立待審校系資料，再經招生資料審核才能公開。Python 結果中的欄位可包含：

```json
{
  "result_type": "brochure",
  "job_id": "job-uuid",
  "academic_year": 115,
  "school_code": "001",
  "sha256_hex": "<64位小寫sha256>",
  "processor": "local-extraction-v1",
  "candidates": [
    {
      "program_code": "013",
      "source_page": 2,
      "confidence": 0.2,
      "data": {
        "admission_program_name": "機械工程系",
        "admission_quota": 2,
        "registration_start_date": "2026-05-01",
        "registration_end_date": "2026-05-08",
        "exam_items": []
      }
    }
  ],
  "generated_at": "2026-08-15T00:00:00Z"
}
```

名單結果的 `rows` 可含 `program_code`、`candidate_number`、`candidate_name`、`result_status`、`official_rank`、`quota` 與 `source_page`。結果 callback 中的完整姓名與准考證只作交易內暫存：Go 服務在 transaction 內將准考證做 HMAC lookup hash、保存末四碼，姓名轉成遮罩後寫入待審查榜單，並以相同 hash 與已確認申請交叉比對。原始名單檔仍保存在 private object storage，應依部署的保存／刪除政策管理；結果批次仍須管理員審核／發布。

名單寫入需要對應的年度／學校／系所已存在於 `academic_programs`；建議先完成簡章候選審核與校系建檔，再送名單。若名單先到，工作會保留失敗狀態，建檔後可由管理端重試，不必重新上傳檔案。

## 服務憑證

API 只讀取 `STA_EXTRACTION_SERVICE_TOKEN`（相容別名：`STA_EXTERNAL_INGESTION_TOKEN`）。正式環境至少 32 字元，應由 secret manager 注入；internal extraction routes 應只暴露在受控網路並經 TLS reverse proxy，不可把服務憑證放在瀏覽器或前端。
