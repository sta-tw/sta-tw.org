# 來源管理總清單契約

## 設計目的

來源不是「一間學校一個固定入口」，而是全域可篩選的來源清單。每一列代表一個學校可能使用的官方公開來源 URL；同一學校可以有多列，同一來源也可以因學年度或文件類型產生不同版本。臨時入口變更時新增列，保留舊列的歷史與最後看見時間。

來源管理頁使用一張總表，不建立一校一頁。至少提供學校、學年度、狀態、來源類型、網域、最後抓取時間、信心、拒絕原因及證據定位的篩選與排序。

## 建議欄位

```json
{
  "source_id": "uuid",
  "school_code": "001",
  "academic_year": "115",
  "source_url": "https://temporary-admission.school.edu.tw/",
  "normalized_url": "https://temporary-admission.school.edu.tw/",
  "hostname": "temporary-admission.school.edu.tw",
  "source_type": "official_entry",
  "status": "active",
  "decision_mode": "agent",
  "affiliation_confidence": "high",
  "discovery_method": "official_link|search_discovery|page_link|manual",
  "evidence": [
    {
      "url": "https://www.school.edu.tw/admissions/",
      "page_or_locator": "#notice-2026",
      "text": "招生系統入口"
    }
  ],
  "first_seen_at": "2026-08-16T00:00:00Z",
  "last_seen_at": "2026-08-16T00:00:00Z",
  "last_crawled_at": null,
  "last_discovery_at": null,
  "discovery_needed": false,
  "discovery_reason": null,
  "rejected_reason": null,
  "manual_note": null
}
```

## 狀態規則

- `candidate`：Agent 發現且通過 suffix／公開性，但學校關聯證據尚不足；可保留並持續觀察，不可作為公開資料發布來源。
- `active`：Agent 或人工判定來源關聯證據充分，可以自動擷取公開文件。
- `rejected`：人工拒絕的來源。即使 Agent 之後再次發現，也不得自動恢復。
- `expired`：長期未再發現或入口已失效；保留歷史資料，不刪除來源證據。

人工覆寫優先於 Agent：

1. 管理員可在總清單拒絕單一 URL 或必要時拒絕整個 hostname。
2. 管理員可新增特定來源 URL，並標記 `decision_mode=manual`。
3. 管理員可恢復 `expired`，但不能直接解除 `rejected` 而不留下操作紀錄。
4. Agent 的週期掃描只能更新 `last_seen_at`、證據與候選狀態，不能覆蓋人工拒絕。

## Agent 自動建冊流程

```text
初始化或觸發條件成立
        ↓
搜尋／官方頁面連結發現候選
        ↓
http(s) + .edu.tw/.gov.tw + 連線安全檢查
        ↓
公開性檢查（不需登入、Email、驗證碼）
        ↓
學校關聯性檢查與證據保存
        ↓
upsert 全域來源清單
        ├─ 證據充分 → active
        └─ 證據不足 → candidate
```

初始化完成後，正常輪詢不重新執行全校關鍵字搜尋，只使用 `active` 來源清單。下列情況才設定 `discovery_needed=true`，對單一學校觸發入口探索：

- 該校沒有任何 `active` 來源；
- 來源連續失效、重新導向到不合格網域或頁面長期為空；
- 已到簡章預定的公告／榜單日期，但來源清單找不到對應文件；
- 管理員手動要求重新探索。

每次探索後設定冷卻時間與 `last_discovery_at`，避免因單次失敗反覆搜尋。搜尋引擎結果只提供候選 URL；真正的來源證據必須來自候選官方頁面本身、官方頁面對它的連結，或文件內可定位的校方資訊。

## 去重與學年度

以 `school_code`、標準化 URL 及必要時的 `academic_year` 去重。不要因為新年度入口名稱不同就覆蓋舊資料；也不要因為同一 hostname 下有多個招生系統，就把不同 URL 合併成一列。來源清單是自動化爬取的控制面，文件版本與內容 SHA-256 另存在文件證據表。
