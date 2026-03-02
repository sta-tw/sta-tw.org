# STA Platform — API 使用說明書

> **Base URL:** `http://localhost:12004`
> **API 版本前綴:** `/api/v1`
> **WebSocket:** `ws://localhost:12004/ws`
> **更新日期:** 2026-02-25

---

## 目錄

1. [認證機制](#認證機制)
2. [通用規範](#通用規範)
3. [Auth 認證 API](#auth-認證-api)
4. [Users 用戶 API](#users-用戶-api)
5. [Channels 頻道 API](#channels-頻道-api)
6. [Messages 訊息 API](#messages-訊息-api)
7. [Search 搜尋 API](#search-搜尋-api)
8. [Portfolio 備審資料 API](#portfolio-備審資料-api)
9. [Verification 驗證 API](#verification-驗證-api)
10. [Tickets 客服票 API](#tickets-客服票-api)
11. [Admissions 招生文件 API](#admissions-招生文件-api)
12. [Admin 管理 API](#admin-管理-api)
13. [WebSocket 即時通訊](#websocket-即時通訊)
14. [Discord Bot 串接指南](#discord-bot-串接指南)

---

## 認證機制

### JWT Bearer Token

所有需要認證的端點，請在 HTTP Header 加上：

```
Authorization: Bearer <access_token>
```

| Token 類型 | 有效期 | 存放位置 |
|-----------|--------|---------|
| Access Token | 15 分鐘 | 記憶體（客戶端自管） |
| Refresh Token | 30 天 | httpOnly Cookie (`refresh_token`) |

### Token 更新流程

```
1. 用 POST /api/v1/auth/refresh 取得新 Access Token
2. Refresh Token 以 httpOnly Cookie 自動帶入
3. Access Token 過期 → 呼叫 refresh → 繼續請求
```

### 角色權限等級

| 角色 | 說明 |
|-----|------|
| `visitor` | 訪客（最低） |
| `prospective_student` | 準考生 |
| `special_student` | 特殊生 |
| `student` | 在學生 |
| `senior` | 學長姐（可上傳備審） |
| `dept_moderator` | 系所版主 |
| `school_moderator` | 學校版主 |
| `admin` | 管理員 |
| `developer` | 開發者 |
| `super_admin` | 超級管理員 |

---

## 通用規範

### 錯誤回應格式

```json
{
  "detail": "錯誤描述字串"
}
```

### 通用 HTTP 狀態碼

| 狀態碼 | 說明 |
|-------|------|
| `200` | 成功 |
| `201` | 建立成功 |
| `400` | 請求參數錯誤 |
| `401` | 未認證 / Token 無效 |
| `403` | 權限不足 |
| `404` | 資源不存在 |
| `422` | 驗證失敗（欄位格式錯誤） |
| `500` | 伺服器內部錯誤 |

### UUID 格式

所有 `id` 欄位均為 UUID v4 字串，例如：`"550e8400-e29b-41d4-a716-446655440000"`

---

## Auth 認證 API

### 1. 註冊

```
POST /api/v1/auth/register
```

**Request Body:**
```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "displayName": "John Doe",
  "password": "mypassword123"
}
```

**Response `201`:**
```json
{
  "access_token": "<jwt>",
  "user": {
    "id": "<uuid>",
    "username": "johndoe",
    "email": "john@example.com",
    "displayName": "John Doe",
    "avatarUrl": null,
    "role": "visitor",
    "verificationStatus": "none",
    "reputationScore": 0,
    "bio": null,
    "createdAt": "2026-02-25T00:00:00Z"
  }
}
```

---

### 2. 登入

```
POST /api/v1/auth/login
```

**Request Body:**
```json
{
  "email": "john@example.com",
  "password": "mypassword123"
}
```

**Response `200`:** 同上 `TokenResponse`，並設置 `refresh_token` httpOnly Cookie。

---

### 3. 登出

```
POST /api/v1/auth/logout
Authorization: Bearer <token>
```

**Response `200`:**
```json
{ "message": "logged out" }
```

---

### 4. 更新 Access Token

```
POST /api/v1/auth/refresh
Cookie: refresh_token=<refresh_jwt>
```

**Response `200`:**
```json
{
  "access_token": "<new_jwt>"
}
```

---

### 5. 取得當前用戶資料

```
GET /api/v1/auth/me
Authorization: Bearer <token>
```

**Response `200`:** `UserOut` 物件（見下方）

---

### 6. Discord OAuth 登入

```
GET /api/v1/auth/oauth/discord
```

→ 重定向至 Discord 授權頁面

```
GET /api/v1/auth/oauth/discord/callback?code=<code>&state=<state>
```

→ 完成後重定向至前端並附帶 `access_token` 參數

---

### 7. Email 驗證 / 密碼重置

```
POST /api/v1/auth/verify-email          Body: { "token": "<email_token>" }
POST /api/v1/auth/resend-verification   Body: { "email": "john@example.com" }
POST /api/v1/auth/forgot-password       Body: { "email": "john@example.com" }
POST /api/v1/auth/reset-password        Body: { "token": "<email_token>", "newPassword": "..." }
POST /api/v1/auth/change-password       Body: { "oldPassword": "...", "newPassword": "..." }
```

---

### 8. Session 管理

```
GET    /api/v1/auth/sessions               取得所有 sessions
DELETE /api/v1/auth/sessions/{session_id}  撤銷指定 session
DELETE /api/v1/auth/sessions               撤銷所有其他 sessions
```

**SessionOut:**
```json
{
  "id": "<uuid>",
  "deviceInfo": "Mozilla/5.0 ...",
  "ipAddress": "1.2.3.4",
  "createdAt": "2026-02-25T00:00:00Z",
  "expiresAt": "2026-03-26T00:00:00Z",
  "isCurrent": true
}
```

---

## Users 用戶 API

### UserOut 物件

```json
{
  "id": "<uuid>",
  "username": "johndoe",
  "email": "john@example.com",
  "displayName": "John Doe",
  "avatarUrl": "https://...",
  "role": "student",
  "verificationStatus": "approved",
  "reputationScore": 150,
  "bio": "自我介紹",
  "createdAt": "2026-02-25T00:00:00Z"
}
```

### 端點

```
GET   /api/v1/users/me                    取得自己的完整資料
PATCH /api/v1/users/me                    更新個人資料
GET   /api/v1/users/me/avatar-upload-url  取得頭像上傳預簽名 URL
GET   /api/v1/users/{username}            取得他人公開資料
```

**PATCH Body (all optional):**
```json
{
  "displayName": "新名字",
  "bio": "新介紹",
  "avatarUrl": "https://..."
}
```

---

## Channels 頻道 API

### ChannelOut 物件

```json
{
  "id": "<uuid>",
  "name": "一般討論",
  "description": "頻道說明",
  "type": "text",
  "scopeType": "global",
  "schoolCode": null,
  "deptCode": null,
  "parentId": null,
  "isArchived": false,
  "cohortYear": null,
  "audience": null,
  "unreadCount": 3,
  "order": 1
}
```

**`type`:** `text` | `announcement`
**`scopeType`:** `global` | `school` | `dept`

### 端點

```
GET /api/v1/channels                       列出所有頻道
GET /api/v1/channels/{channel_id}          取得頻道詳情
GET /api/v1/channels/{channel_id}/messages 取得訊息列表（分頁）
POST /api/v1/channels/{channel_id}/messages 發送訊息
GET /api/v1/channels/{channel_id}/pinned   取得置頂訊息
```

### 訊息列表（游標分頁）

```
GET /api/v1/channels/{channel_id}/messages?limit=50&before=<message_id>
```

**Query Parameters:**

| 參數 | 類型 | 說明 |
|-----|------|------|
| `limit` | int | 每頁數量（預設 50，最大 100） |
| `before` | UUID | 游標：取得此訊息 ID 之前的訊息 |

**Response:**
```json
{
  "messages": [ /* MessageOut[] */ ],
  "hasMore": true,
  "nextCursor": "<message_id>"
}
```

### 發送訊息

```
POST /api/v1/channels/{channel_id}/messages
Authorization: Bearer <token>
```

```json
{
  "content": "訊息內容",
  "replyToId": "<message_id 或 null>"
}
```

---

## Messages 訊息 API

### MessageOut 物件

```json
{
  "id": "<uuid>",
  "channelId": "<uuid>",
  "authorId": "<uuid>",
  "author": { /* UserOut 精簡版 */ },
  "content": "訊息內容",
  "status": "active",
  "createdAt": "2026-02-25T00:00:00Z",
  "updatedAt": "2026-02-25T00:00:00Z",
  "isEdited": false,
  "isPinned": false,
  "replyTo": null,
  "reactions": [
    { "emoji": "👍", "count": 3, "myReaction": true }
  ],
  "threadCount": 0,
  "forwardFromId": null
}
```

**`status`:** `active` | `withdrawn` | `deleted`

### 端點

| 方法 | 路徑 | 說明 | 權限 |
|------|------|------|------|
| `PATCH` | `/{message_id}` | 編輯訊息 | 作者本人 |
| `POST` | `/{message_id}/withdraw` | 撤回訊息 | 作者 / 版主 |
| `DELETE` | `/{message_id}` | 刪除訊息 | 作者 / 版主 |
| `POST` | `/{message_id}/reactions` | 加 Emoji 反應 | 登入用戶 |
| `DELETE` | `/{message_id}/reactions/{emoji}` | 移除反應 | 登入用戶 |
| `POST` | `/{message_id}/pin` | 置頂訊息 | 版主+ |
| `DELETE` | `/{message_id}/pin` | 取消置頂 | 版主+ |
| `GET` | `/{message_id}/thread` | 取得 Thread 回覆 | 登入用戶 |
| `POST` | `/{message_id}/forward` | 轉發到其他頻道 | 登入用戶 |

**PATCH Body:**
```json
{ "content": "新內容" }
```

**POST /reactions Body:**
```json
{ "emoji": "👍" }
```

**POST /forward Body:**
```json
{ "targetChannelId": "<uuid>" }
```

---

## Search 搜尋 API

```
GET /api/v1/search?q=關鍵字&channelId=<uuid>&limit=20
Authorization: Bearer <token>
```

**Query Parameters:**

| 參數 | 必填 | 說明 |
|-----|------|------|
| `q` | 是 | 搜尋字串（1-200 字） |
| `channelId` | 否 | 限制在特定頻道搜尋 |
| `limit` | 否 | 結果數量（預設 20，最大 50） |

**Response:**
```json
{
  "messages": [ /* MessageOut[] */ ],
  "channels": [ /* ChannelOut[] */ ],
  "users": [ /* UserOut[] */ ]
}
```

---

## Portfolio 備審資料 API

### 端點

```
GET  /api/v1/portfolio/schools                      取得學校列表
GET  /api/v1/portfolio/schools/{school_name}/depts  取得科系列表
POST /api/v1/portfolio/school-request               申請新增學校/科系

GET  /api/v1/portfolio                              瀏覽備審文件（可篩選）
POST /api/v1/portfolio/upload-url                   取得上傳預簽名 URL（學長姐+）
POST /api/v1/portfolio                              建立備審文件（學長姐+）
GET  /api/v1/portfolio/{doc_id}                     取得文件詳情
GET  /api/v1/portfolio/{doc_id}/download-url        取得文件下載連結
POST /api/v1/portfolio/{doc_id}/long-view           記錄長閱覽（30分鐘+）
POST /api/v1/portfolio/{doc_id}/share-view          分享連結被開啟（+20 信譽）
POST /api/v1/portfolio/{doc_id}/heartbeat           閱讀心跳（每30分鐘 +10 信譽）
DELETE /api/v1/portfolio/{doc_id}                   刪除文件（上傳者 / 管理員）
PATCH  /api/v1/portfolio/{doc_id}/approve           審核通過/拒絕（管理員）
```

### PortfolioDocumentOut 物件

```json
{
  "id": "<uuid>",
  "uploaderId": "<uuid>",
  "title": "台大資工備審資料",
  "description": "...",
  "schoolName": "國立臺灣大學",
  "deptName": "資訊工程學系",
  "admissionYear": 2025,
  "category": "個人陳述",
  "applicantName": "王小明",
  "resultType": "admitted",
  "admittedRank": 3,
  "totalAdmitted": 30,
  "waitlistRank": null,
  "portfolioScore": 85.5,
  "fileKey": "portfolio/xxx.pdf",
  "fileName": "resume.pdf",
  "fileSize": 1048576,
  "downloadUrl": "https://...",
  "isApproved": true,
  "viewCount": 42,
  "longViewCount": 5,
  "createdAt": "2026-02-25T00:00:00Z",
  "updatedAt": "2026-02-25T00:00:00Z"
}
```

**`resultType`:** `admitted` | `waitlisted` | `not_admitted`

---

## Verification 驗證 API

```
GET  /api/v1/verification/upload-url         取得驗證文件上傳 URL
POST /api/v1/verification/upload-local       本地上傳（開發模式）
GET  /api/v1/verification/local/{filename}   取得本地上傳檔案（開發模式）
POST /api/v1/verification                    送出驗證申請
GET  /api/v1/verification/status             取得自己的驗證申請狀態
GET  /api/v1/verification/admin/queue        待審驗證清單（管理員）
PATCH /api/v1/verification/admin/{req_id}    審核通過/拒絕（管理員）
```

**Submit Body:**
```json
{
  "fileKeys": ["uploads/student-id.jpg"],
  "docType": "學生證"
}
```

**VerificationRequestOut:**
```json
{
  "id": "<uuid>",
  "userId": "<uuid>",
  "status": "pending",
  "fileKeys": ["uploads/student-id.jpg"],
  "docType": "學生證",
  "adminNote": null,
  "submittedAt": "2026-02-25T00:00:00Z",
  "reviewedAt": null,
  "reviewedById": null
}
```

**`status`:** `pending` | `approved` | `rejected`

---

## Tickets 客服票 API

```
GET  /api/v1/tickets                     取得自己的客服票列表
POST /api/v1/tickets                     建立客服票
GET  /api/v1/tickets/{ticket_id}         取得票詳情
POST /api/v1/tickets/{ticket_id}/messages 新增訊息
GET  /api/v1/tickets/admin/all           所有票（客服人員）
PATCH /api/v1/tickets/admin/{ticket_id}  更新狀態/指派（客服人員）
```

**Create Body:**
```json
{
  "category": "account",
  "subject": "帳號問題",
  "content": "我的帳號無法登入..."
}
```

**`category`:** `account` | `verification` | `content` | `bug` | `other`

**TicketOut:**
```json
{
  "id": "<uuid>",
  "userId": "<uuid>",
  "category": "account",
  "subject": "帳號問題",
  "status": "open",
  "assigneeId": null,
  "messages": [ /* TicketMessageOut[] */ ],
  "createdAt": "2026-02-25T00:00:00Z",
  "updatedAt": "2026-02-25T00:00:00Z"
}
```

**`status`:** `open` | `processing` | `pending` | `closed`

---

## Admissions 招生文件 API

```
GET  /api/v1/admissions                  列出招生文件
GET  /api/v1/admissions/{doc_id}         取得文件詳情
POST /api/v1/admissions/import-url       從 URL 匯入 PDF（客服+）
POST /api/v1/admissions/import-file      上傳 PDF 匯入（客服+）
DELETE /api/v1/admissions/{doc_id}       刪除文件（客服+）
GET  /api/v1/admissions/{doc_id}/pdf     下載 PDF（token 參數認證）
```

**AdmissionDocumentOut:**
```json
{
  "id": "<uuid>",
  "sourceUrl": "https://...",
  "title": "113學年度個人申請招生簡章",
  "schoolName": "國立臺灣大學",
  "academicYear": 2024,
  "pageCount": 48,
  "textPreview": "...",
  "keyDates": {},
  "schoolCode": "0001"
}
```

---

## Admin 管理 API

> **權限要求:** `admin` 或 `developer` 角色

```
GET   /api/v1/admin/stats                         平台統計
GET   /api/v1/admin/users                         用戶列表（可搜尋、分頁）
GET   /api/v1/admin/users/{user_id}               用戶詳情
PATCH /api/v1/admin/users/{user_id}               修改用戶（角色、封禁等）
POST  /api/v1/admin/users/{user_id}/force-logout  強制登出用戶

GET    /api/v1/admin/channels                     頻道列表
POST   /api/v1/admin/channels                     建立頻道
PATCH  /api/v1/admin/channels/{channel_id}        更新頻道
DELETE /api/v1/admin/channels/{channel_id}        刪除頻道

GET  /api/v1/admin/audit-log                      稽核日誌（分頁）
GET  /api/v1/admin/audit-log/export               匯出 CSV

GET  /api/v1/admin/reputation/{user_id}           用戶信譽歷史
POST /api/v1/admin/reputation/{user_id}/adjust    手動調整信譽值

GET    /api/v1/admin/portfolio-rules              評分規則列表
POST   /api/v1/admin/portfolio-rules              新增規則
PATCH  /api/v1/admin/portfolio-rules/{rule_id}    更新規則
DELETE /api/v1/admin/portfolio-rules/{rule_id}    刪除規則
GET    /api/v1/admin/portfolio-rules/export       匯出 CSV
POST   /api/v1/admin/portfolio-rules/import       從 CSV 匯入

GET    /api/v1/admin/school-options               學校/科系選項列表
POST   /api/v1/admin/school-options               新增選項
DELETE /api/v1/admin/school-options/{option_id}   刪除選項
GET    /api/v1/admin/school-requests              用戶申請列表
PATCH  /api/v1/admin/school-requests/{req_id}     審核申請
```

---

## WebSocket 即時通訊

### 連線

```
ws://localhost:12004/ws?token=<access_token>
```

連線後若 token 無效，server 以 code `4001` 關閉連線。

### 客戶端 → 伺服器

**訂閱頻道：**
```json
{ "type": "subscribe", "channelId": "<uuid>" }
```

**取消訂閱：**
```json
{ "type": "unsubscribe", "channelId": "<uuid>" }
```

**發送打字中指示：**
```json
{ "type": "typing", "channelId": "<uuid>" }
```

### 伺服器 → 客戶端

**新訊息：**
```json
{
  "type": "message.new",
  "data": { /* MessageOut */ }
}
```

**訊息更新（編輯）：**
```json
{
  "type": "message.update",
  "data": { /* MessageOut */ }
}
```

**打字中廣播：**
```json
{
  "type": "typing",
  "data": {
    "userId": "<uuid>",
    "channelId": "<uuid>"
  }
}
```

---

## Discord Bot 串接指南

### 概覽

STA 平台已原生支援 Discord OAuth 登入，Discord Bot 可以透過以下方式整合平台功能：

1. **帳號綁定** — 用戶在 Discord 使用 `/link` 指令綁定 STA 帳號
2. **頻道橋接** — 將 Discord 頻道與 STA 頻道雙向同步
3. **查詢指令** — 在 Discord 查詢備審資料、招生文件
4. **通知推播** — 接收 STA 平台通知推播到 Discord

---

### 步驟一：建立 Discord OAuth Bot 帳號

1. 在 Discord Developer Portal 建立 Application
2. 取得 `client_id` 與 `client_secret`
3. 設定後端環境變數：

```env
DISCORD_CLIENT_ID=your_client_id
DISCORD_CLIENT_SECRET=your_client_secret
```

4. 在 Discord OAuth2 Redirect URI 加入：
```
http://localhost:12004/api/v1/auth/oauth/discord/callback
```

---

### 步驟二：帳號綁定流程

```
Discord 用戶使用 /link 指令
        ↓
Bot 回傳一次性授權連結：
https://discord.com/oauth2/authorize?client_id=...&redirect_uri=...&scope=identify+email
        ↓
用戶在瀏覽器完成 Discord 授權
        ↓
GET /api/v1/auth/oauth/discord/callback?code=xxx
        ↓
STA 後端建立/綁定帳號，回傳 access_token
        ↓
Bot 儲存 access_token + Discord user_id 對應關係
```

**綁定後，Bot 持有的 access_token 可代表用戶操作 STA API。**

> **注意：** Access Token 有效期為 15 分鐘，Bot 需要自行實作 Token 刷新邏輯（使用 Refresh Token Cookie）。建議使用 Bot 的 Service Account 模式：以管理員 Bot 帳號登入，取得長期 token。

---

### 步驟三：Bot Service Account 登入

為 Bot 建立一個專用的 STA 帳號（role: `admin`），使用一般帳號密碼登入：

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "discord-bot@sta.tw",
  "password": "..."
}
```

儲存 `access_token`（15分鐘），並在 Response 的 `Set-Cookie` 中儲存 `refresh_token` 供定期刷新。

---

### Discord Bot 可用 API 功能

#### `/sta search <關鍵字>` — 搜尋訊息/頻道/用戶

```http
GET /api/v1/search?q=台大資工&limit=5
Authorization: Bearer <bot_token>
```

回傳結果後，Bot 以 Embed 格式顯示搜尋結果。

---

#### `/sta portfolio <學校> <科系>` — 查詢備審資料

```http
GET /api/v1/portfolio?schoolName=國立臺灣大學&deptName=資訊工程學系&limit=5
Authorization: Bearer <bot_token>
```

**Query Parameters:**

| 參數 | 說明 |
|-----|------|
| `schoolName` | 學校名稱 |
| `deptName` | 科系名稱 |
| `admissionYear` | 入學年份 |
| `resultType` | `admitted` / `waitlisted` / `not_admitted` |
| `limit` | 筆數（最大 50） |

回傳 `PortfolioDocumentOut[]`

---

#### `/sta channels` — 列出平台頻道

```http
GET /api/v1/channels
Authorization: Bearer <bot_token>
```

---

#### `/sta ticket <類別> <主題> <內容>` — 建立客服票

代表用戶（需要用戶的 access_token）：

```http
POST /api/v1/tickets
Authorization: Bearer <user_token>
Content-Type: application/json

{
  "category": "account",
  "subject": "帳號問題",
  "content": "我遇到了..."
}
```

---

#### 頻道訊息橋接（雙向同步）

**STA → Discord：** Bot 建立 WebSocket 連線監聽訊息

```javascript
const ws = new WebSocket('ws://localhost:12004/ws?token=' + botToken)

ws.on('message', (raw) => {
  const event = JSON.parse(raw)

  if (event.type === 'message.new') {
    const msg = event.data
    // 轉發到對應的 Discord 頻道
    discordChannel.send({
      embeds: [{
        author: { name: msg.author.displayName },
        description: msg.content,
        timestamp: msg.createdAt
      }]
    })
  }
})

// 訂閱 STA 頻道
ws.send(JSON.stringify({
  type: 'subscribe',
  channelId: 'sta-channel-uuid'
}))
```

**Discord → STA：** 監聽 Discord 訊息後呼叫 API

```javascript
discordClient.on('messageCreate', async (msg) => {
  if (msg.channelId !== BRIDGED_DISCORD_CHANNEL) return
  if (msg.author.bot) return

  const userToken = await getUserTokenByDiscordId(msg.author.id)
  if (!userToken) return

  await fetch(`http://localhost:12004/api/v1/channels/${STA_CHANNEL_ID}/messages`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${userToken}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ content: msg.content })
  })
})
```

---

### Discord Embed 範例

#### 備審資料查詢結果

```javascript
{
  title: doc.title,
  color: doc.resultType === 'admitted' ? 0x57F287 : 0xED4245,
  fields: [
    { name: '學校', value: doc.schoolName, inline: true },
    { name: '科系', value: doc.deptName, inline: true },
    { name: '入學年份', value: String(doc.admissionYear), inline: true },
    { name: '結果', value: resultTypeLabel[doc.resultType], inline: true },
    { name: '名次', value: `${doc.admittedRank} / ${doc.totalAdmitted}`, inline: true },
    { name: '備審分數', value: String(doc.portfolioScore), inline: true }
  ],
  footer: { text: `👁 ${doc.viewCount} 次瀏覽` },
  timestamp: doc.createdAt
}
```

#### 平台新訊息通知

```javascript
{
  author: { name: msg.author.displayName, icon_url: msg.author.avatarUrl },
  description: msg.content,
  color: 0x5865F2,
  footer: { text: `#${channelName} · STA Platform` },
  timestamp: msg.createdAt
}
```

---

### Token 管理建議

```javascript
class STAClient {
  constructor() {
    this.accessToken = null
    this.refreshTimer = null
  }

  async login(email, password) {
    const res = await fetch('http://localhost:12004/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',  // 儲存 refresh_token cookie
      body: JSON.stringify({ email, password })
    })
    const data = await res.json()
    this.accessToken = data.access_token
    this._scheduleRefresh()
  }

  _scheduleRefresh() {
    // 每 14 分鐘刷新一次（access token 15 分鐘過期）
    this.refreshTimer = setTimeout(() => this._refresh(), 14 * 60 * 1000)
  }

  async _refresh() {
    const res = await fetch('http://localhost:12004/api/v1/auth/refresh', {
      method: 'POST',
      credentials: 'include'
    })
    const data = await res.json()
    this.accessToken = data.access_token
    this._scheduleRefresh()
  }

  async get(path) {
    return fetch(`http://localhost:12004${path}`, {
      headers: { 'Authorization': `Bearer ${this.accessToken}` }
    })
  }

  async post(path, body) {
    return fetch(`http://localhost:12004${path}`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.accessToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(body)
    })
  }
}
```

---

### 推薦 Discord 指令清單

| 指令 | 說明 |
|-----|------|
| `/sta link` | 綁定 STA 帳號 |
| `/sta me` | 查看自己的 STA 資料 |
| `/sta search <query>` | 全文搜尋 |
| `/sta portfolio <school> <dept>` | 查詢備審資料 |
| `/sta channels` | 列出平台頻道 |
| `/sta ticket <category> <subject>` | 建立客服票 |
| `/sta status` | 查看個人驗證狀態 |
| `/sta admissions <school>` | 查詢招生文件 |

---

### 健康檢查

```
GET /health
```

**Response:**
```json
{ "status": "ok" }
```

---

*STA Platform API v1 · 2026-02-25*
