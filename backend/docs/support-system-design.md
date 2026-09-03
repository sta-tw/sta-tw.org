# 客服工單、Email 與 Discord 整合設計

## 1. 設計結論

客服採用 Ticket 工單模式。網站資料庫是唯一正式紀錄；Email 負責寄送通知，Discord 是管理員的即時操作介面。三者不各自保存一套獨立對話，避免訊息分歧。

```text
使用者網站
    │  建立工單／傳送訊息
    ▼
STA API ── PostgreSQL（canonical record）
    ├── Email outbox ── SMTP／官方客服信箱
    └── Discord outbox ── 私人 Ticket 頻道
```

客服 Ticket 不使用目前的公開 `lounge` 閒聊頻道，但可以沿用既有的 chat message、Discord bridge 與 outbox 重試機制。

目前已有的 `application_service_tickets` 是「追加申請／修改申請」的審核單，仍維持原本的管理流程；本設計新增的 `support_tickets` 是通用客服對話，不取代也不混用前者。

## 2. 帳號註冊與信箱驗證

### 一般使用者

1. 使用者註冊時輸入 `edu.tw` 信箱。
2. 系統檢查信箱格式與網域，建立一次性 Email challenge。
3. 透過既有的 `email_verification_challenges` 寄送驗證連結。
4. 使用者點擊連結後，才啟用帳號與登入功能。

驗證 token 只以 hash 保存，明文只存在驗證信中；信件本身不放准考證或其他敏感資料。

這裡的 `edu.tw` 判定只用於「建立 STA 帳號與寄送註冊驗證信」。它不套用到客服聯絡信箱、`STA_SUPPORT_EMAIL`、一般 Email 通知、Discord 通知或其他收件者。客服 API 要求的是帳號已完成 Email 驗證，這是驗證狀態檢查，不是再次判定信箱網域。

### 特殊學生或非 `edu.tw` 使用者（帳號註冊例外）

無法使用一般註冊流程時，可以從「特殊註冊／聯絡團隊」表單提交申請。此時先建立沒有 `account_id` 的客服 Ticket，使用者提供的聯絡信箱以加密方式保存。

這類聯絡信箱不受 `edu.tw` 網域判定限制；客服通知仍可寄送到使用者提供且通過格式檢查的信箱。這條人工處理流程只處理帳號註冊例外，不代表官方資料來源的網域驗證規則改變。

管理員確認後，系統寄送一次性邀請連結。使用者完成註冊後，將 Ticket 的 `account_id` 綁定到新帳號。所有人工核准與綁定動作都寫入 audit log。

## 3. Ticket 生命週期

| 狀態 | 意義 |
|---|---|
| `open` | 新建立，等待客服處理 |
| `waiting_staff` | 目前需要客服回覆或處理 |
| `waiting_user` | 客服已回覆，等待使用者補充資料 |
| `closed` | 已結案，對話保留為唯讀歷史紀錄 |
| `spam` | 管理員判定為垃圾或無效案件 |

主要狀態轉換：

```text
open → waiting_staff → waiting_user → waiting_staff
                         │
                         └──────────→ closed
```

使用者在已關閉 Ticket 留言時，預設建立新 Ticket 並引用原 Ticket；管理員也可以依權限重新開啟，但必須寫入事件紀錄。

## 4. 建立客服 Ticket

使用者在網站填寫：

- 分類，例如帳號、簡章、查榜、准考證、意願詢問、其他
- 主旨
- 問題內容
- 可選附件

API 在同一個資料庫 transaction 中完成：

1. 建立 Ticket。
2. 建立第一則 canonical message。
3. 建立通知事件與 Email outbox。
4. 建立 Discord 頻道建立任務。

API 不同步等待 SMTP 或 Discord 回應。即使外部服務暫時故障，Ticket 仍會成功建立，worker 會依 outbox 狀態重試。

建立後會寄送兩類信件：

- 給官方客服：新 Ticket 編號、分類、主旨與網站處理連結。
- 給使用者：Ticket 編號、已收到通知與網站查看連結。

## 5. 網站對話與 Discord 同步

### 網站端

- 使用者只能查看自己的 Ticket。
- 使用者可以新增訊息與附件。
- 使用者可以看到 Ticket 狀態、客服回覆、通知狀態與歷史紀錄。
- 關閉後的訊息不可修改或刪除，只能依權限加註事件。

### Discord 端

每個 Ticket 建立一個私人頻道，例如：

```text
客服分類
└── ticket-000123
```

頻道權限只開放客服角色、管理員角色與 bot。使用者不需要加入 Discord，也不會直接看到 Discord 頻道。

- 網站訊息：保存後排入 Discord outbox。
- Discord 管理員訊息：透過簽名 webhook 或 bot gateway 寫入同一筆 canonical message。
- 同一則訊息以 `platform + external_message_id` 去重。
- Discord 頻道建立失敗時，網站仍可正常使用，worker 會重試。
- Ticket 關閉時，Discord 頻道鎖定並可移至封存分類。

Discord 訊息只顯示必要資訊；准考證、完整 Email、驗證碼與私有檔案不可直接貼入頻道。

## 6. Email 通知規則

Email 通知與客服人員回覆都回寫網站 Ticket 的 canonical message；入站回覆只接受帶 HMAC 簽章的 provider webhook，正式環境不開放任意 SMTP 轉寄或未驗證信箱輪詢。

| 事件 | 通知對象 | 內容 |
|---|---|---|
| Ticket 建立 | 官方客服、使用者 | Ticket 編號與連結 |
| 使用者新增訊息 | 官方客服 | 未讀訊息與處理連結 |
| 管理員回覆 | 使用者 | 回覆摘要與查看連結 |
| Ticket 關閉 | 使用者 | 結案通知與歷史紀錄連結 |
| Discord 同步失敗 | 管理員 | 系統告警，不通知使用者 |

信件透過既有加密 `email_outbox` 發送。每個事件都要有 dedup key，避免 worker 重試造成重複寄信。

入站端點為 `POST /api/v1/support/webhooks/email`，要求 `X-STA-Signature` 與 `STA_SUPPORT_EMAIL_WEBHOOK_SECRET`，payload 需含 provider message ID、`T-######` 工單編號、寄件者、主旨與內文；provider message ID 會在資料庫去重。Email 附件目前不接受，請使用網站附件端點。

## 7. 建議資料表

`000016_support_tickets.sql` 已建立客服資料表；Email 沿用既有 `email_outbox`，Discord 由 `cmd/support-worker` 處理。

### `support_tickets`

| 欄位 | 用途 |
|---|---|
| `id` | 內部 UUID |
| `ticket_number` | 使用者看到的工單編號，例如 `T-000123` |
| `account_id` | 已註冊使用者；特殊申請可為 `NULL` |
| `requester_email_ciphertext` | 未登入／特殊申請的聯絡信箱 |
| `category` | 問題分類 |
| `subject` | 主旨 |
| `status` | Ticket 狀態 |
| `assigned_to` | 負責管理員，可為 `NULL` |
| `discord_channel_id` | Discord 頻道 ID，可暫時為 `NULL` |
| `created_at`、`closed_at` | 時間欄位 |

### `support_ticket_events`

append-only 保存建立、指派、狀態變更、Discord 建立、Email 發送、關閉與重新開啟等事件。

### 對既有表的使用

- `support_messages`：保存 Ticket 對話的 canonical message。
- `support_message_bridges`：保存 Discord 訊息對照。
- `support_discord_outbox`：建立私人頻道、同步訊息、封存與重試。
- `notifications`／`email_outbox`：站內通知與加密 Email outbox。
- `audit_log`：管理員操作與敏感資料異動。

客服對話使用獨立 API 與資料表，不會被目前的公開 `lounge` API 查到。

## 8. API

### 使用者 API

```text
POST  /api/v1/support/tickets
GET   /api/v1/support/tickets
GET   /api/v1/support/tickets/{ticketID}
POST  /api/v1/support/tickets/{ticketID}/messages
POST  /api/v1/support/tickets/{ticketID}/close
POST  /api/v1/support/tickets/{ticketID}/reopen
```

### 管理員 API

```text
GET   /api/v1/admin/support/tickets
GET   /api/v1/admin/support/tickets/{ticketID}
PATCH /api/v1/admin/support/tickets/{ticketID}
POST  /api/v1/admin/support/tickets/{ticketID}/messages
POST  /api/v1/admin/support/tickets/{ticketID}/close
```

### Discord webhook

```text
POST /api/v1/support/webhooks/discord
```

Webhook 必須驗證簽名、限制 body 大小，並以 Discord channel ID 找到 Ticket；找不到對應頻道時拒絕寫入，避免訊息落入錯誤工單。

## 9. 權限與個資規則

- 使用者只能讀取自己是 requester 的 Ticket。
- 管理員依角色決定是否能查看全部 Ticket、下載附件或刪除垃圾案件。
- Discord 只給客服角色與 bot 權限，不開放公開邀請。
- Email、准考證、驗證 token 與特殊學生證明只保存加密值或 hash。
- audit log 不寫入明文個資與機密內容。
- 所有訊息、附件與 webhook 都要有大小限制、速率限制與惡意內容檢查。
- Email 與 Discord 的失敗狀態只能影響同步，不得回滾已建立的 Ticket。

## 10. 已完成與後續

1. `000016_support_tickets.sql`、Ticket repository 與網站 API 已完成。
2. Email template 已接到既有 notification worker 的 `email_outbox`。
3. Discord 私人頻道建立、訊息同步、封存與 signed webhook 已完成；啟動 `cmd/support-worker` 後生效。
4. 網站前端客服頁面可依本文件 API 接入。
5. 網站客服 Ticket 已支援 private object storage 附件、大小／數量／檔案 signature 限制與擁有者／管理員短效下載入口；Email 回覆只透過簽章 webhook 進入同一套 canonical 對話來源。

6. Email provider 回覆可透過 `POST /api/v1/support/webhooks/email` 回寫 Ticket。Webhook 必須使用 `STA_SUPPORT_EMAIL_WEBHOOK_SECRET` 產生 `X-STA-Signature`，payload 提供 `external_message_id`、`ticket_number`、`from`、`subject` 與 `body`；系統以去重表避免重複建立訊息。未簽章的原始信箱內容不會直接送入 API。

## 11. 驗收條件

- 使用者送出 Ticket 後，即使 SMTP 或 Discord 暫時中斷，Ticket 仍存在且任務會重試。
- 使用者與管理員在網站看到同一份訊息歷史。
- 管理員從 Discord 回覆後，網站、站內通知與 Email 都能留下可追蹤紀錄。
- Ticket 關閉後使用者仍能查看完整紀錄，Discord 頻道不可再任意發言。
- 同一個 Discord webhook 重送不會產生重複訊息。
- 未登入的特殊註冊申請完成人工核准後，可以安全綁定到新帳號。
