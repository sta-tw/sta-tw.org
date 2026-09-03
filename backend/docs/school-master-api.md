# 學校主檔維護 API

學校主檔由後端保存，前端只透過 API 讀取；未來的教育部名錄更新、校名修正與停用不需要 SSH 進伺服器或直接修改 PostgreSQL。

## 公開搜尋

```http
GET /api/v1/schools?q=台大&limit=20
```

`q` 是可選的模糊搜尋字串。API 會對正式校名做大小寫、空白、標點與「臺／台」正規化，並支援前綴、包含與順序縮寫比對，例如 `台大`、`北科`。不維護人工別名表，也不會替使用者自動猜選唯一學校；回傳候選清單讓前端顯示選項。

公開結果只包含 `is_active = true` 的學校。`limit` 預設為：有搜尋字串時 30 筆、無搜尋字串時 200 筆，最大 200 筆。

## 管理端同步

所有管理端 API 都需要已登入且具有 `admin` role 的帳號。Cookie session 的修改請求要帶 `X-CSRF-Token`；外部同步程式可使用既有認證流程取得的 opaque bearer session token，不新增未受控的永久 API key。

### 批次同步

```http
POST /api/v1/admin/schools/sync
Content-Type: application/json
Authorization: Bearer <admin-session-token>
```

```json
{
  "reason": "教育部 115 學年度校院名錄更新",
  "items": [
    {
      "school_code": "003",
      "school_name": "國立臺灣大學",
      "institution_type": "general_university",
      "is_active": true
    },
    {
      "school_code": "149",
      "school_name": "基督教台灣浸會神學院",
      "institution_type": "religious_research_college",
      "is_active": true
    }
  ]
}
```

每批最多 500 筆，整批在同一個 transaction 中完成；任何一筆失敗時整批回滾。同步是 upsert，不會因為某筆沒有出現在本次清單就自動停用學校；要停用時必須明確送出 `is_active: false`。停用只會從公開搜尋隱藏，歷史招生、論壇與驗證資料仍可保留外鍵關聯。

### 單筆修正

```http
PUT /api/v1/admin/schools/003
Content-Type: application/json
```

Body 欄位與批次項目相同，另外以 `reason` 說明修正原因。

### 查詢異動紀錄

```http
GET /api/v1/admin/schools/003/history
```

每次實際新增、修改、停用或重新上架都會寫入既有 append-only `audit_log`，包含操作者、修改前後資料、動作與原因；沒有實際變化的重複同步不會產生無意義的紀錄。
