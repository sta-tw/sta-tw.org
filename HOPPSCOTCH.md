# Hoppscotch 整合指南

STA Platform API 完整支援 [Hoppscotch](https://hoppscotch.io/) 匯入與測試。

## 快速開始

### 1. 匯入 OpenAPI 規格

在 Hoppscotch 中：

1. 點擊左側 **Import/Export**
2. 選擇 **Import from URL**
3. 輸入 OpenAPI 規格 URL：
   ```
   http://localhost:12004/openapi.json
   ```
4. 點擊 **Import**

所有 API 端點會自動載入到你的 Collection 中。

### 2. 設定環境變數

建立一個新的 Environment：

| 變數名稱 | 值 | 說明 |
|---------|---|------|
| `baseUrl` | `http://localhost:12004/api/v1` | API Base URL |
| `accessToken` | (留空) | 登入後自動填入 |

### 3. 認證流程

#### 步驟 1：註冊或登入

使用 `POST /auth/register` 或 `POST /auth/login`：

```json
{
  "email": "test@example.com",
  "password": "password123"
}
```

#### 步驟 2：複製 Access Token

從 Response 中複製 `access_token` 欄位的值。

#### 步驟 3：設定 Authorization Header

在 Hoppscotch 的 **Authorization** 標籤：
- Type: `Bearer Token`
- Token: 貼上你的 `access_token`

或者在環境變數中設定 `accessToken`，然後在 Header 使用：
```
Authorization: Bearer <<accessToken>>
```

## 常用 API 測試流程

### 1. 用戶註冊與登入

```http
POST {{baseUrl}}/auth/register
Content-Type: application/json

{
  "username": "johndoe",
  "email": "john@example.com",
  "displayName": "John Doe",
  "password": "mypassword123"
}
```

### 2. 取得當前用戶資料

```http
GET {{baseUrl}}/auth/me
Authorization: Bearer {{accessToken}}
```

### 3. 列出所有頻道

```http
GET {{baseUrl}}/channels
Authorization: Bearer {{accessToken}}
```

### 4. 發送訊息

```http
POST {{baseUrl}}/channels/{channel_id}/messages
Authorization: Bearer {{accessToken}}
Content-Type: application/json

{
  "content": "Hello, world!",
  "replyToId": null
}
```

### 5. 搜尋訊息

```http
GET {{baseUrl}}/search?q=關鍵字&limit=20
Authorization: Bearer {{accessToken}}
```

### 6. 查詢備審資料

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
   ws://localhost:12004/ws?token={{accessToken}}
   ```
4. 點擊 **Connect**

### 訂閱頻道

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

### Pre-request Script

在 Hoppscotch 的 **Pre-request Script** 中自動更新 Token：

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

### Tests Script

驗證 Response：

```javascript
// 檢查狀態碼
pw.expect(pw.response.status).toBe(200)

// 檢查 Response Body
const body = pw.response.body
pw.expect(body).toHaveProperty("access_token")

// 自動儲存 token
if (body.access_token) {
  pw.env.set("accessToken", body.access_token)
  pw.env.set("tokenExpiry", Date.now() + 14 * 60 * 1000)
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

### Q: Token 過期怎麼辦？

A: Access Token 有效期為 15 分鐘。過期後：
1. 使用 `POST /auth/refresh` 更新 Token
2. 或重新登入取得新 Token

### Q: 如何測試需要管理員權限的 API？

A: 需要先在資料庫中手動將用戶的 `role` 改為 `admin` 或 `developer`。

### Q: WebSocket 連線失敗？

A: 確認：
1. Token 是否有效
2. URL 格式是否正確（`ws://` 而非 `http://`）
3. 後端服務是否正在運行

### Q: CORS 錯誤？

A: 確認後端 CORS 設定包含你的 Hoppscotch 來源（通常是 `https://hoppscotch.io`）。

## 相關資源

- [完整 API 文件](./api-docs.md)
- [OpenAPI 規格](http://localhost:12004/openapi.json)
- [Hoppscotch 官方文件](https://docs.hoppscotch.io/)
