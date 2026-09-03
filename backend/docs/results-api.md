# 交叉比對、查榜結果與意願 API

這是核心交叉比對 API，供前端直接使用；它不需要 Telegram token、Telegram account link 或 Telegram outbox，也不由 Telegram Bot 路由包裝。其他通知／展示渠道若要接入，必須在核心之外透過 channel adapter 呼叫同一個 canonical willingness transaction。

交叉比對採「校系數值陣列」與「官方錄取造冊」分離的設計：

- `academic_programs.willingness_values` 只保存依官方錄取序排列的 `0`、`20`、`40`、`60`、`80`、`100` 數值。
- `official_results` 保存官方來源、正取／備取、錄取序與准考證 lookup hash；`willingness_index` 是目前發布名冊中的 0-based 陣列位置。
- User ID、申請資料與詢問識別碼只用於暫時匹配、授權與發送詢問，不會寫入校系意願陣列。
- 官方結果先進入 `pending_review`，管理員確認官方來源與解析結果後才發布。

## 校系意願陣列

建立校系時由資料庫建立空陣列：

```json
{
  "willingness_values": []
}
```

發布官方榜單後，系統只為正取與備取資料列建立陣列槽位，並以 `100` 初始化。例如官方順序為「正取 1、正取 2、備取 1、備取 2」時：

```json
{
  "willingness_values": [100, 100, 100, 100]
}
```

正取先於備取排序；正取／備取類別與實際錄取序仍保存在官方造冊，不能由陣列位置直接猜測類別。

初始 `100` 是未回覆時的計算預設值。使用者回覆後，後端依官方資料列的 `willingness_index` 原子更新陣列位置。意願陣列不保存准考證號、User ID 或詢問 ID。

## 管理端結果批次

所有管理端 GET 需要 `admin` role；GET 不需要 CSRF，POST 寫入仍需要 CSRF 或既有 Bearer 寫入認證。

### 匯入

```http
POST /api/v1/admin/results/import
Content-Type: application/json

{
  "academic_year": 116,
  "school_code": "001",
  "source_url": "https://example.edu.tw/result.pdf",
  "source_sha256": "<64 位小寫 hex>",
  "rows": [
    {
      "academic_year": 116,
      "school_code": "001",
      "program_code": "023",
      "candidate_number": "AB-123456",
      "masked_name": "王○○",
      "result_status": "waitlisted",
      "official_rank": 4,
      "quota": 3,
      "source_page": 42
    }
  ]
}
```

結果狀態為 `admitted`、`waitlisted`、`rejected` 或 `unknown`。正取與備取必須有正整數 `official_rank`，因為它們需要建立意願陣列位置；拒絕與未知資料不建立槽位。准考證不保存明文，只保存加密值、HMAC lookup hash 與末四碼。

`source_url` 必須是 HTTP(S) 的官方 `*.edu.tw` 或 `*.gov.tw` 網域；搜尋結果只能協助發現入口，不能作為來源證據。

批次建立後為 `pending_review`，同一學年度／學校／來源 hash 不可重複匯入。

### 清單與明細

```http
GET /api/v1/admin/results/batches?academic_year=116&school_code=001&status=pending_review&limit=50&offset=0
GET /api/v1/admin/results/batches/{batchID}
```

批次明細只回傳准考證末四碼，不回傳完整准考證。管理端可看到來源、解析結果、匹配數與已建立詢問數。

### 發布

```http
POST /api/v1/admin/results/{batchID}/publish
```

發布交易會：

1. 將同年度／學校的舊發布批次標記為 `superseded`。
2. 將正取排在備取之前，為每個官方資料列建立 `willingness_index`。
3. 依每個校系的正取／備取人數重建 `willingness_values`，全部初始化為 `100`。
4. 清除該新批次的舊目前意願快取。
5. 對已匹配的正取與備取使用者建立 `result_released` 意願詢問。

發布前會重新以目前已確認申請的准考證 lookup hash 比對整個批次，因此在榜單匯入後才加入平台或才填寫准考證的人不會漏掉。尚未填寫准考證的使用者不會被建立詢問；使用者之後補填准考證並成功匹配時，系統會立即補建詢問。

### 截止前再次詢問

```http
POST /api/v1/admin/results/{batchID}/inquiries/acceptance-deadline
Content-Type: application/json

{"deadline":"2026-12-20T23:59:00+08:00"}
```

只對目前尚未回覆的正取／備取資料列建立或更新 `acceptance_deadline` 詢問。通知透過 durable outbox 交給通知 worker 發送。

### 官方更正

```http
POST /api/v1/admin/results/{resultID}/correct
```

更正必須附原因，並寫入 append-only `audit_log`。更正後會重建該批次的意願索引與校系陣列；仍可對應到目前意願的資料列會保留其數值，沒有回覆的資料列回到 `100`。若更正後成為正取或備取且尚未回覆，會建立結果發布詢問。

## 使用者端

使用者只能讀取自己擁有且已確認的申請；需要已驗證 `student` 身份。所有 JSON 回應都使用 `Cache-Control: no-store`。

### 查詢結果與推估

```http
GET /api/v1/applications/{applicationID}/result
```

系統在後端使用登入帳號、申請資料與准考證 lookup hash 取得目前發布的官方結果，不接受前端任意指定 User ID 或錄取序。

回傳本人官方狀態、錄取序、名額、前方候選人數、前方已回覆人數、目前意願值與參考推估：

- 正取的 `reference_probability` 固定為 `100`。
- 備取使用前方正取／備取候選人的意願數值平均作為目前參考值。
- `position_after_declines` 依前方意願值大於 `0` 的候選人計算。
- 這是匿名參考值，不是學校正式遞補結果，也不是錄取保證。

### 詢問輪次

```http
GET /api/v1/applications/{applicationID}/inquiries
```

回傳輪次、截止時間、是否回覆與目前意願值。結果發布與截止前各為一個詢問輪次；意願修改會追加事件紀錄。

若目前詢問已設定 `response_deadline` 且已逾期，意願回覆會被拒絕；未設定截止時間的結果發布詢問仍可依目前流程回覆。

### 准考證比對

```http
PUT /api/v1/applications/{applicationID}/candidate-number
Content-Type: application/json

{"candidate_number":"AB-123456"}
```

後端以年度、學校、校系與 HMAC lookup hash 進行精確匹配。候選號碼不會回傳或寫入校系意願陣列。

### 回覆意願

```http
PUT /api/v1/applications/{applicationID}/willingness
Content-Type: application/json

{"value":20,"inquiry_id":"<optional inquiry uuid>"}
```

意願只接受 `0`、`20`、`40`、`60`、`80`、`100`。後端會驗證登入帳號與詢問的關聯，依官方錄取序更新校系的純數值陣列，並追加 `willingness_response_events`。

成功回傳：

```json
{
  "data": {
    "response_id": 123,
    "academic_year": 116,
    "school_code": "001",
    "program_code": "023",
    "result_status": "waitlisted",
    "admission_rank": 2,
    "willingness": 20
  }
}
```

回應不包含 User ID 或完整准考證號。`response_id` 只作為必要的去識別化識別碼。
