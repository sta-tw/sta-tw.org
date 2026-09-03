# 官方招生簡章檔案 API

簡章檔案由管理端維護，每個學年度／學校只保留目前最新檔案；物件名稱使用 `brochures/{學年度}-{學校編號}.pdf`，例如 `brochures/116-003.pdf`。歷次上傳、審核、上架與下架事件保存在 `brochure_document_events`，事件不可修改或刪除。檔案本體只存放在 private object storage。

所有管理端端點需要 `admin` role。Cookie session 的寫入請求需要 CSRF header；API client 可使用 opaque bearer token。

## 上傳最新檔案

```http
POST /api/v1/admin/admissions/brochures
Content-Type: multipart/form-data
```

multipart 欄位：

- `academic_year`：三位數學年度，例如 `116`
- `school_code`：三位數校院編號，例如 `001`
- `source_url`：官方來源網址，可選
- `file`：PDF，副檔名、宣告 MIME、PDF signature 與大小都會檢查，大小上限 50 MB

`source_url` 若有提供，只接受 `http`／`https` 且主機名稱位於 `.edu.tw` 或 `.gov.tw` 的官方網址；API 不會依來源網址自動抓取文件。

上傳成功後狀態為 `pending`，不會直接公開。若同年度同校已有檔案，會取代目前資料列並留下 `uploaded` 事件。若 RabbitMQ 已設定，API 會同時建立以年度／學校／SHA-256／解析器版本去重的簡章抽取工作；Python worker 的結果會寫入隔離候選區。核准單一候選時必須同時送出人工確認過的完整校系資料，API 會在同一筆資料庫交易中把它寫成 `academic_programs.pending`，之後仍須經招生資料審核才會公開。

抽取工作管理端 API：

```http
GET /api/v1/admin/ingestion/brochure-runs?academic_year=116&status=pending_review
GET /api/v1/admin/ingestion/brochure-runs/{runID}
POST /api/v1/admin/ingestion/brochure-runs/{runID}/review
POST /api/v1/admin/ingestion/brochure-candidates/{candidateID}/review
POST /api/v1/admin/ingestion/jobs/{jobID}/retry
```

候選核准範例：

```json
{
  "approved": true,
  "reason": "已逐欄核對簡章第 12 頁",
  "program": {
    "academic_year": 115,
    "school_code": "001",
    "program_code": "013",
    "admission_program_name": "機械工程學系",
    "admission_quota": 2,
    "exam_items": [{"name":"資料審查","sort_order":1,"weight_percent":100,"description":"-","source_page":"12"}],
    "brochure_is_tentative": false,
    "brochure_announcement_date": "-",
    "brochure_scheduled_date": "-",
    "registration_start_date": "-",
    "registration_end_date": "-",
    "exam_start_date": "-",
    "exam_end_date": "-",
    "result_date": "-",
    "consultation_phone": "-",
    "brochure_url": "https://admission.example.edu.tw/115.pdf",
    "special_talent_target": "-",
    "different_education_backgrounds": "-",
    "different_education_other": "-",
    "notes": "-",
    "source_page": 12
  }
}
```

核准與建立待審招生資料是原子操作；完整欄位驗證失敗時候選不會被標成已核准。整批 run 端點只用於退回；核准必須逐候選確認，最後一筆處理完成時 run 會自動變成 `approved` 或 `rejected`。

## 管理端查詢與事件

```http
GET /api/v1/admin/admissions/brochures?academic_year=116
GET /api/v1/admin/admissions/brochures/116/001/events
```

管理端回應使用 `Cache-Control: no-store`。回應不包含 object storage key，只提供檔名、大小、SHA-256、來源網址與狀態。

## 審核、上架與下架

```http
POST /api/v1/admin/admissions/brochures/116/001/review
POST /api/v1/admin/admissions/brochures/116/001/visibility
```

審核 body：

```json
{
  "approved": true,
  "reason": "已核對官方簡章"
}
```

`pending` 通過後變成 `published`；退回後變成 `rejected`。下架 body 使用 `{"published": false, "reason": "..."}`，狀態變為 `archived`；重新上架使用 `{"published": true}`。

## 下載

```http
GET /api/v1/admissions/brochures/116/001/download
GET /api/v1/admin/admissions/brochures/116/001/download
```

公開下載只允許 `published` 檔案；管理端可取得任意現存狀態的檔案。API 回傳有效期 5 分鐘的 signed URL，不直接代理檔案內容。
