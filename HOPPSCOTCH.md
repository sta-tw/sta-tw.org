# Hoppscotch 整合指南

STA Platform API 完整支援 [Hoppscotch](https://hoppscotch.io/) 匯入與測試。

## 快速開始

### 1. 匯入 OpenAPI 規格

**方法一：直接從線上匯入（推薦）**

1. 開啟 [Hoppscotch](https://hoppscotch.io/)
2. 點擊左側 **Import/Export**
3. 選擇 **Import from URL**
4. 輸入 OpenAPI 規格 URL：
   ```
   https://sta-backend.sta-tw.workers.dev/openapi.json
   ```
5. 點擊 **Import**

**方法二：從本地開發環境匯入**

如果你在本地運行後端：
```
http://localhost:12004/openapi.json
```

所有 API 端點會自動載入到你的 Collection 中。

### 2. 設定環境變數

建立一個新的 Environment（點擊右上角齒輪圖示）：

| 變數名稱 | 值 | 說明 |
|---------|---|------|
| `baseUrl` | `https://sta-backend.sta-tw.workers.dev/api/v1` | Production API |
| `accessToken` | (留空) | 登入後自動填入 |

### 3. 測試第一個 API

#### 步驟 1：註冊新帳號

1. 在左側 Collection 找到 **Auth 認證** → **註冊新用戶**
2. 點擊進入，會看到預設的 Request Body：

```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "displayName": "John Doe",
  "password": "mypassword123"
}
```

3. 修改成你自己的資料
4. 點擊 **Send** 按鈕

#### 步驟 2：複製 Access Token

成功後會收到 Response：

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "...",
    "username": "johndoe",
    ...
  }
}
```

複製 `access_token` 的值。

#### 步驟 3：設定 Authorization

**方法一：在環境變數設定（推薦）**

1. 點擊右上角齒輪圖示
2. 在 `accessToken` 變數貼上你的 token
3. 之後所有需要認證的 API 會自動使用這個 token

**方法二：手動設定每個請求**

在每個需要認證的請求中：
1. 切換到 **Authorization** 標籤
2. Type 選擇 `Bearer Token`
3. Token 欄位貼上你的 `access_token`

#### 步驟 4：測試認證 API

1. 找到 **Auth 認證** → **取得當前用戶資料**
2. 點擊 **Send**
3. 應該會看到你的用戶資料

## 常用 API 測試流程

### 1. 用戶登入

```http
POST {{baseUrl}}/auth/login
Content-Type: application/json

{
  "email": "john@example.com",
  "password": "mypassword123"
}
```

### 2. 列出所有頻道

```http
GET {{baseUrl}}/channels
Authorization: Bearer {{accessToken}}
```

### 3. 發送訊息

先從上一步取得 `channel_id`，然後：

```http
POST {{baseUrl}}/channels/{channel_id}/messages
Authorization: Bearer {{accessToken}}
Content-Type: application/json

{
  "content": "Hello, world!",
  "replyToId": null
}
```

### 4. 搜尋訊息

```http
GET {{baseUrl}}/search?q=關鍵字&limit=20
Authorization: Bearer {{accessToken}}
```

### 5. 查詢備審資料

```http
GET {{baseUrl}}/portfolio?schoolName=國立臺灣大學&deptName=資訊工程學系
Authorization: Bearer {{accessToken}}
```

## WebSocket 測試

Hoppscotch 也支援 WebSocket 測試：

1. 切換到 **Realtime** 標籤
2. 選擇 **WebSocket**
3. 輸入 URL：
   ```
   wss://sta-backend.sta-tw.workers.dev/ws?token=YOUR_ACCESS_TOKEN
   ```
   （記得替換 `YOUR_ACCESS_TOKEN`）
4. 點擊 **Connect**

### 訂閱頻道

連線成功後，在下方輸入框發送：

```json
{
  "type": "subscribe",
  "channelId": "your-channel-uuid"
}
```

### 發送打字中指示

```json
{
  "type": "typing",
  "channelId": "your-channel-uuid"
}
```

## 進階功能

### Pre-request Script（自動更新 Token）

在 Hoppscotch 的 **Scripts** → **Pre-request Script** 中：

```javascript
// 檢查 token 是否過期（15 分鐘）
const tokenExpiry = pw.env.get("tokenExpiry")
const now = Date.now()

if (!tokenExpiry || now > tokenExpiry) {
  // 呼叫 refresh endpoint
  const response = await pw.http.post("{{baseUrl}}/auth/refresh", {
    credentials: "include"
  })
  
  if (response.status === 200) {
    const data = response.body
    pw.env.set("accessToken", data.access_token)
    pw.env.set("tokenExpiry", now + 14 * 60 * 1000) // 14 分鐘後過期
  }
}
```

### Tests Script（自動儲存 Token）

在登入/註冊請求的 **Scripts** → **Tests** 中：

```javascript
// 檢查狀態碼
pw.expect(pw.response.status).toBe(200)

// 自動儲存 token
const body = pw.response.body
if (body.access_token) {
  pw.env.set("accessToken", body.access_token)
  pw.env.set("tokenExpiry", Date.now() + 14 * 60 * 1000)
  console.log("Token saved!")
}
```

## 匯出與分享

### 匯出 Collection

1. 點擊 Collection 右側的 **⋮** 選單
2. 選擇 **Export**
3. 選擇格式：
   - **Hoppscotch Collection** (推薦)
   - **OpenAPI 3.0**
   - **Postman Collection**

### 分享給團隊

1. 點擊右上角的 **Share** 按鈕
2. 選擇分享方式：
   - **Share Link** - 產生公開連結
   - **Export JSON** - 匯出檔案給團隊成員

## 常見問題

### Q: 匯入後看不到任何端點？

A: 確認：
1. URL 是否正確：`https://sta-backend.sta-tw.workers.dev/openapi.json`
2. 網路連線是否正常
3. 後端服務是否正在運行

### Q: Token 過期怎麼辦？

A: Access Token 有效期為 15 分鐘。過期後：
1. 使用 `POST /auth/refresh` 更新 Token
2. 或重新登入取得新 Token

### Q: 如何測試需要管理員權限的 API？

A: 需要先在資料庫中手動將用戶的 `role` 改為 `admin` 或 `developer`。

### Q: WebSocket 連線失敗？

A: 確認：
1. Token 是否有效
2. URL 使用 `wss://`（HTTPS 環境）或 `ws://`（HTTP 環境）
3. Token 參數是否正確帶入

### Q: CORS 錯誤？

A: CloudFlare Workers 後端已設定允許 Hoppscotch。如果仍有問題，請檢查後端 CORS 設定。

## 快速測試連結

直接點擊以下連結在 Hoppscotch 中測試：

- [匯入 OpenAPI 規格](https://hoppscotch.io/?import=https://sta-backend.sta-tw.workers.dev/openapi.json)

## 相關資源

- [完整 API 文件](./api-docs.md)
- [OpenAPI 規格](https://sta-backend.sta-tw.workers.dev/openapi.json)
- [Hoppscotch 官方文件](https://docs.hoppscotch.io/)
