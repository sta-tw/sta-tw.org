# 招生科系資料管理 API

招生科系資料由管理端 API 維護，公開查詢只會讀取 `review_status = published` 的資料。`program_identifier` 與 `source_locator` 不接受外部輸入：前者由學年度／學校編號／科系編號生成，後者由學校編號／頁碼生成。

`brochure_url` 若有提供，只接受 `http`／`https` 且主機名稱位於 `.edu.tw` 或 `.gov.tw` 的官方網址；第三方網域、IP、userinfo 與非標準 port 會被拒絕。

管理端端點需要已登入且具有 `admin` role 的 session；使用 cookie session 的寫入請求還需要 CSRF header，使用 opaque bearer token 的 API client 則以 `Authorization: Bearer` 驗證。

## 管理端清單與單筆查詢

```http
GET /api/v1/admin/admissions/programs?academic_year=116&review_status=pending&q=國文
GET /api/v1/admin/admissions/programs/116-001-023
GET /api/v1/admin/admissions/programs/116-001-023/history
```

管理端清單可用 `academic_year`、`school_code`、`review_status`、`q`、`limit` 與 `offset` 篩選。所有管理端資料不允許瀏覽器快取。

## 簡章搜尋顯示最新年度

管理端可設定簡章搜尋的顯示年度上限：

```http
GET /api/v1/admin/admissions/display-latest-year
PUT /api/v1/admin/admissions/display-latest-year
Content-Type: application/json
```

更新時送出 `{ "academic_year": 117 }`。公開搜尋會在此年度及更早的已上架資料中，為每個校系顯示最新一筆；若該校系尚無此年度已上架資料，會沿用最近一筆較早年度資料。歷屆資料只會列出早於設定年度的資料，因此切換到 117 後，116 會列入歷屆。資料列即使尚無錄取統計也會顯示，缺少的統計欄位以 `-` 呈現。新資料上架後，搜尋會自動優先顯示新年度資料；要隱藏尚未被新年度資料取代的舊資料，仍可在後台手動下架。

查詢回傳 `{ "data": { "academic_year": 116, "is_set": true } }`；尚未建立設定時，`academic_year` 會使用目前最新的已上架年度，`is_set` 為 `false`。

Migration 000076 會將目前最新的已上架年度設為初始顯示年度，避免部署後應屆年度立刻出現在歷屆清單。

## 公開簡章搜尋

```http
GET /api/v1/admissions/programs/search?q=台大&interview=required&limit=50&offset=0
```

搜尋端點只回傳結果卡片需要的欄位，並在資料庫端套用關鍵字、術科、面試與作品集條件。`interview`、`skills_test`、`portfolio` 可設為 `required` 或 `not-required`；`limit` 預設 50、上限 100，`offset` 用於分頁，`meta.count` 是符合條件的總筆數。完整招生資料與時程仍由單筆校系端點讀取。

## 批次同步

```http
POST /api/v1/admin/admissions/programs/sync
Content-Type: application/json
Authorization: Bearer <admin-session-token>
```

```json
{
  "reason": "教育部 115 學年度特殊選材資料建檔",
  "items": [
    {
      "academic_year": 115,
      "school_code": "001",
      "program_code": "023",
      "admission_program_name": "特殊選材國文學系甲組",
      "admission_quota": 3,
      "exam_items": [
        {
          "name": "書面審查",
          "sort_order": 1,
          "weight_percent": 60,
          "description": "審查學習歷程與作品",
          "source_page": "23"
        },
        {
          "name": "面試",
          "sort_order": 2,
          "weight_percent": 40,
          "description": "評估專業興趣與學習動機",
          "source_page": "24"
        }
      ],
      "brochure_is_tentative": false,
      "consultation_phone": "-",
      "brochure_url": "-",
      "special_talent_target": "-",
      "different_education_backgrounds": "-",
      "different_education_other": "-",
      "notes": "-",
      "source_page": 23
    }
  ]
}
```

每批最多 500 筆，在同一個 transaction 中處理；任何一筆失敗時整批回滾。校名由 `school_code` 查主檔取得，不能由同步資料覆蓋。

新增資料或實際內容變更會進入 `pending`；若原本是 `published`，修改後也會先撤回公開狀態，等待重新審核。完全相同的重複同步不會重新改動資料或產生稽核紀錄。

## 單筆修正

```http
PUT /api/v1/admin/admissions/programs/115-001-023
Content-Type: application/json
```

Body 使用 `{ "reason": "...", "item": { ...ProgramInput } }`。`item` 內的學年度、學校編號與科系編號必須與路徑一致，避免修改錯誤資料。

回傳的 `Program` 會包含唯讀欄位 `willingness_values`。建立校系時它是空陣列；官方錄取榜單發布後，系統依正取／備取名冊建立並以 `100` 初始化。這個欄位不接受管理端輸入，也不包含准考證號或 User ID。

## 審核與上架

```http
POST /api/v1/admin/admissions/programs/115-001-023/review
Content-Type: application/json
```

```json
{
  "approved": true,
  "reason": "已核對 115 學年度官方簡章"
}
```

`approved = true` 將 `pending` 轉成 `published`；否則轉成 `rejected`，拒絕時必須填寫原因。所有新增、修改、審核與上架動作都寫入 append-only `audit_log`。
