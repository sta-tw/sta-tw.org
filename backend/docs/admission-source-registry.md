# 官方招生來源總清單

來源清單是招生資料擷取的控制面，不會直接建立或發布 `academic_programs`、榜單或其他公開資料。來源只接受公開的 `.edu.tw`／`.gov.tw` URL，並保存學校關聯證據供管理員審核。

## 管理端 API

所有端點都需要已驗證的管理員帳號；寫入請求需要 CSRF 或 Bearer mutation authorization。

```http
GET /api/v1/admin/admission-sources?academic_year=116&status=active
POST /api/v1/admin/admission-sources
PATCH /api/v1/admin/admission-sources/{sourceID}
```

建立與更新資料至少包含：`school_code`、`academic_year`、`source_url`、`evidence`。每筆證據包含官方 URL、頁面／定位資訊與原文短引文。相同學校、學年度與標準化 URL 不可重複建立。

來源狀態為 `candidate`、`active`、`rejected` 或 `expired`。只有 `active` 來源可以交給後續公開文件擷取器；來源清單本身不會自動下載文件，也不會呼叫 AI。
