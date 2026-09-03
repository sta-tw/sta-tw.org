# 特殊選才簡章探索

簡章探索核心不依賴任何前端或聊天平台。Telegram Bot、管理網站及外部 discovery agent 都只能透過相同的狀態與審核邊界操作。

## 學年度週期

- 學年度由管理控制面板建立，不寫死在程式或外部 agent prompt。
- 新週期先是 `draft`；管理員明確啟動後成為 `active`，結束排程後成為 `closed`。
- 同一時間最多一個 `active` 週期，discovery agent 的 `claim` 只會取得該週期任務。
- `closed` 週期不再派發搜尋任務，但已進入人工審核的資料仍可完成審核。
- 掃描母體是 `brochure_discovery_school_roster` 的固定 149 間學校名冊，不以前一年度是否參與特殊選才作為篩選條件。
- 建立週期時會複製固定名冊；學校主檔後續臨時新增或退出不會改變掃描母體。初始 migration 建立 115 學年度草稿週期與 149 筆任務。
- 外部候選的 `detected_academic_year` 必須精確等於任務所屬學年度；其他年度不會進入該週期。
- 候選來源與 PDF URL 必須是公開的 `.edu.tw` 或 `.gov.tw` URL。

## 狀態

| 系統值 | 顯示名稱 | 說明 |
|---|---|---|
| `completed` | 已完成 | 外部 agent 找到且人工確認、人工上傳，或人工確認本年度無簡章 |
| `under_review` | 審核中 | 外部 agent 已找到並保存候選 PDF，等待人工確認 |
| `searching` | 搜尋中 | 已由 agent 領取或等待再次搜尋 |
| `pending_search` | 待搜尋 | 尚未加入搜尋處理 |
| `needs_attention` | 待處理 | 搜尋、下載或分析發生技術錯誤 |

`completed` 另以 `completion_method` 區分 `agent_confirmed`、`manual_upload` 與 `no_brochure_confirmed`。確認無簡章必須附理由，完成後不再重複派發。

探索狀態與 `brochure_documents.review_status` 是兩個不同狀態機。前者追蹤是否取得各校簡章；後者控制 PDF 的審核與公開狀態。

## API

所有端點目前使用既有管理員認證、MFA 與 mutation authorization：

```http
GET  /api/v1/admin/admissions/brochure-discovery/cycles
POST /api/v1/admin/admissions/brochure-discovery/cycles
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/start
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/close
GET  /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks
GET  /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/events
POST /api/v1/admin/admissions/brochure-discovery/claim
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/candidate
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/failure
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/retry
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/review
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/manual-complete
POST /api/v1/admin/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/no-brochure
```

`claim` 使用資料庫列鎖與 15 分鐘 lease，同一個任務不會同時交給多個 agent；agent 中斷後可重新領取。

正式探索工作程式使用獨立服務憑證，不使用管理員帳號：

```http
POST /api/v1/internal/admissions/brochure-discovery/claim
POST /api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/candidate
POST /api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/failure
POST /api/v1/internal/admissions/brochure-discovery/cycles/{academicYear}/tasks/{schoolCode}/no-match
Authorization: Bearer <STA_BROCHURE_DISCOVERY_AGENT_TOKEN>
```

`candidate` 是 multipart PDF 上傳；API 會重新檢查年度、官方網址、PDF signature、檔案大小、惡意程式掃描與 SHA-256。`no-match` 不會把學校標成完成，而是依嘗試次數延後 1～7 天；只有管理員可確認「本年度無簡章」。

## 外部候選契約

外部 discovery agent 必須先將 PDF 送入既有簡章保存管線，完成 PDF signature、大小、ClamAV、private object storage 與 checksum 驗證。之後才能回報候選：

```json
{
  "detected_academic_year": 115,
  "source_url": "https://admission.example.edu.tw/special-selection/115",
  "document_url": "https://admission.example.edu.tw/files/115-special-selection.pdf",
  "sha256": "64-character-lowercase-sha256",
  "confidence": 0.95,
  "evidence": {
    "title": "115 學年度特殊選才招生簡章",
    "year_evidence": "PDF 封面",
    "document_type_evidence": "特殊選才招生簡章"
  }
}
```

核心會再次確認候選年度與 URL path 中的週期一致，且同校、同年度、相同 checksum 的 `pending` PDF 確實存在，才把任務改為 `under_review`。

人工核准會在同一個 PostgreSQL transaction 中：

1. 將 PDF 從 `pending` 改為 `published`。
2. 將探索任務改為 `completed`／外部候選確認（資料庫相容值仍為 `ai_confirmed`）。
3. 寫入簡章事件與探索事件。

退回則把 PDF 改為 `rejected`，探索任務回到 `searching`。人工補檔完成也會在同一筆 transaction 中發布 PDF，並標記 `completed`／`manual_upload`。
