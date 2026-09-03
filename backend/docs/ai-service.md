# AI 服務移除說明

STA 已移除執行期對外部 AI API 的依賴。Go API 不建立 AI client、不呼叫模型；Python worker 的正式流程也只使用本地 PDF／文字／表格／JSON 規則解析。這讓人工上傳、外部搜尋／AI 找到檔案、檔案擷取與管理員審核都能透過 STA 自己的 API 完成。

目前的資料流是：

```text
人工上傳或外部搜尋／AI 找到原始檔
    ↓
Go API 驗證來源、掃描、private storage、建立 job
    ↓
Python worker 透過 RabbitMQ 或 HTTP claim 取得檔案
    ↓
本地規則抽取簡章／名單資料
    ↓
Go API 保存隔離候選／待審榜單
    ↓
管理員確認、建立校系或發布結果
```

舊版 AI client 與 extraction orchestration 已從本專案移除；主 API、RabbitMQ worker、HTTP worker 與 discovery worker 都不會載入模型 SDK 或遠端 AI client。新的部署不應設定 `STA_AI_*`；請改用 `STA_EXTRACTION_SERVICE_TOKEN`、`STA_EXTRACTION_TRANSPORT` 與 [文件擷取 API](extraction-api.md)。

若外部 AI 服務要協助找檔，只需要把原始 PDF／名單送到：

- `POST /api/v1/internal/extraction/brochures`
- `POST /api/v1/internal/extraction/candidate-lists`

外部服務不能以此 token 審核或上架資料，也不會取得資料庫／object storage 長期憑證。
