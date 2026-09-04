# STA security baseline

這份文件記錄已實作與尚未完成的資安邊界。它不是「通過資安檢測」的宣告；正式上線前仍需做環境、依賴、滲透與備份還原檢查。

## 已實作

- 密碼使用 Argon2id；資料庫只保存編碼後的 hash，不保存原始密碼。
- Email 使用 AES-256-GCM 加密保存，使用 HMAC-SHA-256 lookup hash 做唯一查找。
- 欄位加密支援金鑰輪替：ciphertext 前綴一個 byte 的 key version，`FieldCipher`
  可同時持有多把金鑰（舊金鑰保留供讀取，primary 供寫入），未加版本的舊資料以
  legacy key 作 fallback。以 `STA_FIELD_ENCRYPTION_KEYS` /
  `STA_FIELD_ENCRYPTION_PRIMARY_VERSION` 設定，換金鑰後執行 `reencrypt -apply`
  把靜態資料搬到新金鑰，再淘汰舊金鑰。
- HMAC lookup hash（`email_lookup_hash`、OAuth subject hash）也支援輪替：
  `STA_LOOKUP_HMAC_KEY` 為 primary（寫入），`STA_LOOKUP_HMAC_SECONDARY_KEYS`
  放退役金鑰供讀取（登入／查找以 `= ANY` 同時比對）。`reencrypt -apply` 會用
  primary 金鑰重算 `accounts.email_lookup_hash`（明文可由 email_ciphertext 還原）；
  OAuth subject hash 於下次該帳號 OAuth 登入時就地重算。全部搬完即可清掉
  secondary。其餘 lookup hash（准考證、學校信箱）不在此流程內。
- session、CSRF、OAuth state 與 OAuth provider subject 不保存明文值。
- OAuth 使用 state 與 PKCE；callback 一次性消耗 state。
- 原生帳號先建立；OAuth 未綁定時只拒絕登入，不自動建立帳號。
- 同一 provider 的 OAuth identity 只能綁定一個 STA 帳號；同一 STA 帳號每個 provider 最多一筆。
- Cookie session 的修改請求需要 CSRF 驗證；Bearer 認證不依賴 cookie。
- 登入、註冊、Email 驗證、聊天室、客服訊息與學生信箱驗證寄送有 process-local 快速限流，並以 PostgreSQL fixed-window bucket 做跨副本共用的原子限流。
- CORS 只接受精確 allowlist，禁止 wildcard；production origin 要求 HTTPS。
- request body 上限、安全 headers、panic recovery、未知 JSON 欄位拒絕已加入。
- 稽核資料表 append-only；資料異動的 before／after 必須由應用層先去識別化。
- 學校信箱只接受管理員白名單；驗證碼以 HMAC hash 保存並限制嘗試次數。
- 帳號註冊 Email 可透過獨立的一次性 token 驗證；token 只保存 hash、短效且消耗一次，寄送走加密 Email outbox，不與學生身份驗證混用。
- 在校證明只進 private object storage；檔名、大小、MIME、副檔名與檔案 signature 都會檢查，並在 object storage 前執行 ClamAV `INSTREAM` 掃描；公開下載只能拿短效 signed URL。
- 學生驗證證明與 Email payload 使用 outbox；一般 API 不直接寄信，也不把驗證碼寫入 log。
- Discord／Telegram inbound webhook 使用 HMAC、dedup；outbound Bot token 僅由 worker 使用，錯誤訊息不包含 response body。
- 外部文件擷取 API 使用獨立服務憑證；原始檔仍經官方來源、大小、signature、ClamAV、private storage 與 checksum 邊界，服務憑證不能取代管理員審核。
- outbox 以資料庫鎖、processing stale reclaim、失敗重試與事件狀態避免跨平台／Email 重複投遞造成資料覆寫。
- 學校主檔管理 API 只允許 admin role；批次更新使用 transaction 與 advisory lock，停用採 soft delete，並將修改前後資料寫入 append-only audit log。
- 招生科系管理 API 只允許 admin role；校名由校院主檔重新解析，識別碼／定位編號拒絕外部偽造，批次建檔與考試項目替換使用同一 transaction，修改後必須重新進入 pending，審核／上架／退回均寫入 append-only audit log。
- 官方簡章管理 API 只允許 admin role；PDF 上傳會做大小、檔名、MIME 與 signature 檢查，檔案放在 private object storage，管理端下載只發 5 分鐘 signed URL，管理回應禁止快取，事件表 append-only。
- 備審檔案 API 只讓擁有者讀取自己的專案／版本／事件；管理端清單與事件需要 admin role，寫入操作需要 CSRF，版本建立鎖定專案列避免併發碰撞，所有 metadata 回應禁止快取，檔案儲存失敗會清除暫存 object。
- 查榜結果只保存准考證加密值、lookup hash 與末四碼；結果批次發布以 transaction／advisory lock 保證同校同年度只有最新批次，使用者只能讀取自己的 confirmed application，意願修改綁定申請與詢問輪次，通知 worker 會跳過已回覆詢問。
- 管理員 MFA 使用加密 TOTP seed；正式環境預設要求 `/api/v1/admin/` 請求帶 `X-MFA-Code`，設定流程與期限見 [admin-mfa.md](admin-mfa.md)。TOTP 驗證失敗以帳號為單位限流（15 分鐘內 8 次失敗即鎖到視窗結束，正確碼不計次），擋住用竊得的 admin session 暴力猜 6 位數碼。`POST /api/v1/auth/admin-mfa/verify` 成功後開啟 `STA_ADMIN_MFA_GRANT_TTL`（預設 15 分鐘）的授權視窗，期間 admin 請求可免帶 `X-MFA-Code`；設 0 則每次都要帶。
- 招生簡章、備審、學生證明與客服附件在 private object storage 前通過 ClamAV 掃描；掃描器故障時 fail-closed。

## 禁止事項

- 不要把密碼、OAuth access／refresh token、原始驗證文件、准考證號碼、Email 明文或完整 IP 寫入一般 log。
- 不要讓前端傳入 `account_id` 後直接查資料；每個物件都必須由後端依 session 權限重新判斷。
- 不要讓 Python Worker 直接發布公開招生資料。
- 不要讓 OAuth callback 依 provider email 自動找帳號；綁定依 provider subject 且必須經由既有 STA 帳號流程。

## 正式環境仍需由部署與維運完成

- 以正式 secret manager 注入 AES／HMAC key；AES key 輪替走
  `STA_FIELD_ENCRYPTION_KEYS` + `reencrypt`。
- 實際套用 migration 並驗證 append-only trigger、備份與還原。
- 維護 ClamAV 病毒庫、隔離網路與解壓縮資源限制；signature 與 ClamAV 仍不等同完整滲透測試。
- 完成最小權限、稽核告警與年度驗證文件清理演練。
- 以正式監控系統對 outbox／登入／審核失敗、限流器故障與 ClamAV 故障建立告警。
