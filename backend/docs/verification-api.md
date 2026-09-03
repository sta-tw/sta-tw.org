# 身份驗證 API

身份驗證和帳號 Email 驗證是兩套流程：Email 驗證只確認帳號可以收信；學生身份驗證才會影響查榜與年度身份。

## 使用者端

```text
GET  /api/v1/verification/requests
POST /api/v1/verification/requests/school-email
POST /api/v1/verification/requests/{requestID}/verify-email
POST /api/v1/verification/requests/document
POST /api/v1/verification/requests/{requestID}/documents
```

所有端點都需要既有帳號 session；POST 使用 Cookie session 時需要 CSRF header。學校信箱只能使用管理員維護的學校／網域白名單。證明檔只會存入 private object storage，API 不回傳 storage key。

## 管理端

```text
GET  /api/v1/admin/verification/requests/pending
GET  /api/v1/admin/verification/requests/{requestID}/documents
GET  /api/v1/admin/verification/requests/{requestID}/documents/{documentID}/download
POST /api/v1/admin/verification/requests/{requestID}/review
GET  /api/v1/admin/verification/domains
POST /api/v1/admin/verification/domains
POST /api/v1/admin/verification/domains/{domainID}/active
```

管理員先用文件清單確認 `requestID` 下的證明，再呼叫 download 取得短效 signed URL。下載 URL 有效 5 分鐘，回應使用 `Cache-Control: no-store`；只有指定申請與指定文件的管理員可以取得，不能用任意 storage key 讀檔。

審核通過後才會更新帳號身份與年度驗證資料。年度清理只移除學生驗證用的 challenge、證明檔與個人識別驗證資料；帳號、申請、簡章、心得與長期論壇資料不因此刪除。

