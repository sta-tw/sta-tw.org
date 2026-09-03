# 招生文件抽取契約

擷取器或外部 agent 只能提交候選 JSON；所有欄位都必須能回指原始文件。未明示的內容使用 `null`、空陣列或 `unknown`，不可自行補值。正式流程應在提交後執行 `scripts/validate_extraction.py`；正式 STA Python worker 使用本地規則，不載入模型。

## 頂層格式

```json
{
  "document": {
    "document_type": "brochure",
    "title": "學校原始文件標題",
    "academic_year": "115",
    "school_code": "001",
    "program_code": null,
    "source_url": "https://admission.example.edu.tw/notice.pdf",
    "source_page": 3,
    "published_at": null,
    "source_sha256": "0000000000000000000000000000000000000000000000000000000000000000",
    "review_status": "pending",
    "evidence": [
      {
        "page_or_locator": "PDF p.3",
        "text": "原始證據短引文"
      }
    ]
  },
  "schedules": [
    {
      "original_label": "報名暨繳費",
      "original_text": "115年5月1日上午9時至5月8日下午5時",
      "start_date": "2026-05-01",
      "start_time": "09:00",
      "end_date": "2026-05-08",
      "end_time": "17:00",
      "timezone": "Asia/Taipei",
      "location_original": null,
      "classification_suggestion": null,
      "confidence": "high",
      "review_status": "pending",
      "evidence_ref": "document.evidence[0]"
    }
  ],
  "results": []
}
```

`document_type` 只能是 `brochure`、`announcement`、`stage_notice`、`result`、`waitlist_notice` 或 `unknown`。`review_status` 只能是 `pending`、`needs_review`、`approved` 或 `rejected`。

## 抽取規則

- `original_label`、`original_text`、`location_original` 以學校原文保存，不翻譯、不套用全域同義詞。
- `classification_suggestion` 可為 `null`；它只是查詢輔助，不得覆蓋原文。
- 日期與時間要分開保存。只有日期時，時間必須是 `null`；不能補 `00:00`。
- `start_date`／`end_date` 是可驗證的 ISO 日期候選值；民國年、模糊日期及跨年日期須保留原文，並由後處理確認，不能只靠模型心算。
- 「報名暨繳費」等合併文字維持一筆；只有原文分別列出報名與繳費時才拆分。
- 「另行公告」「依學校通知」「時間未定」等內容不能轉成推測值，應使用 `unknown` 或 `needs_review`。
- 每筆日程與榜單資料都要有 `evidence_ref`，指向頁碼、表格列、HTML selector 或可重現的文字定位。
- 文件內有修正公告時，保留原文件版本，並在上層資料建立版本關係；不可直接覆寫原始證據。

## 榜單資料

公開榜單只抽取產品需要的欄位，例如原始結果狀態、正備取標記、名次、名額、系所及公開的候選編號片段。若姓名不是產品必要欄位，應省略；不得從文件推導完整個人身分資料。

建議結果項目包含：

```json
{
  "program_code": "A01",
  "result_status_original": "正取",
  "result_status_suggestion": "admitted",
  "official_rank": 1,
  "quota": 2,
  "candidate_number_last4": "1234",
  "evidence_ref": "document.evidence[1]",
  "confidence": "high"
}
```

`result_status_suggestion` 不能取代 `result_status_original`；各校用語不同時，以原文為準。
