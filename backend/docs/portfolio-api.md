# 備審資料與檔案版本 API

備審檔案本體只放在 private object storage；PostgreSQL 保存專案、版本、checksum、狀態與 append-only 事件。所有回應都加上 `Cache-Control: no-store`，下載 API 只回傳有效 5 分鐘的 signed URL。

## 使用者端

所有使用者端 endpoint 都要求已驗證的 `student` 或 `senior` 身份。Cookie session 的寫入請求需要 CSRF；Bearer token 依既有認證流程驗證。

### 列出備審專案

```http
GET /api/v1/portfolio/projects
```

專案只能由擁有者讀取。建立專案時，對應申請必須已確認，且同一筆申請只能建立一個專案：

```http
POST /api/v1/portfolio/projects
Content-Type: application/json

{"application_id":"<uuid>","title":"國立○○大學｜○○系"}
```

### 上傳與列出版本

```http
POST /api/v1/portfolio/projects/{projectID}/files
Content-Type: multipart/form-data

file=<file>
```

允許的檔案類型由檔名、副檔名、宣告 MIME 與檔案 signature 共同檢查，單檔上限 50 MiB。上傳完成後狀態固定為 `hidden`，同一專案的版本號由後端產生；新增版本時會鎖定專案列，避免併發上傳得到相同版本號。

```http
GET /api/v1/portfolio/projects/{projectID}/files
```

回傳該擁有者的所有版本，依版本號由新到舊排列。`storage_key` 與 `owner_account_id` 永不序列化給前端。

### 送審、上下架與事件

```http
POST /api/v1/portfolio/files/{fileID}/submit
POST /api/v1/portfolio/files/{fileID}/unpublish
POST /api/v1/portfolio/files/{fileID}/hide
GET  /api/v1/portfolio/files/{fileID}/events
```

狀態規則：

| 目前狀態 | 操作 | 新狀態 |
| --- | --- | --- |
| `hidden`、`unpublished`、`rejected` | 作者送審 | `pending_review` |
| `published` | 作者下架 | `unpublished` |
| `published`、`unpublished`、`rejected` | 作者隱藏 | `hidden` |

每次上傳、送審、審核、下架與隱藏都追加至 `portfolio_file_events`；事件不允許更新或刪除。事件 API 只回傳該檔案擁有者自己的資料。

### 下載

```http
GET /api/v1/portfolio/files/{fileID}/download
```

`published` 檔案可公開取得短效 signed URL；其他狀態只有已驗證的檔案擁有者或管理員可以取得。API 不代理檔案內容，也不把 object storage key 回傳給呼叫端。

## 管理端

管理端 endpoint 要求 `admin` role。GET 查詢不需要 CSRF；POST 審核仍需要 CSRF 或既有 Bearer 寫入認證。

### 待審清單

```http
GET /api/v1/admin/portfolio/files?status=pending_review&limit=50&offset=0
```

查詢參數：

- `status`：`hidden`、`pending_review`、`published`、`unpublished`、`rejected`。
- `project_id`：可選的專案 UUID。
- `limit`：1–100，預設 50。
- `offset`：0–10000，預設 0。

回傳管理員需要的專案標題、申請 ID、版本與檔案 metadata，但不回傳 owner account ID 或 private storage key。

### 審核與事件

```http
POST /api/v1/admin/portfolio/files/{fileID}/review
Content-Type: application/json

{"approved":true,"reason":"已核對備審資料內容"}
```

只有 `pending_review` 可以審核；通過轉為 `published`，退回轉為 `rejected`，退回時 `reason` 必填。管理員事件查詢：

```http
GET /api/v1/admin/portfolio/files/{fileID}/events
```

管理員下載仍透過 `GET /api/v1/portfolio/files/{fileID}/download` 取得短效 URL，避免建立另一套檔案輸出邏輯。
