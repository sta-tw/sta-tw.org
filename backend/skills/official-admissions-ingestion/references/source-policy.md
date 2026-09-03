# 官方來源政策

## 允許的主機名稱

先將主機名稱轉成小寫、移除結尾的單一句點，並以 IDNA 形式比較。允許的 suffix 是：

- `.edu.tw`
- `.gov.tw`

主機名稱必須是 suffix 本身的子網域，且 suffix 前要有 `.`。例如：

| 網址 | 結果 | 原因 |
|---|---|---|
| `https://admission.ntu.edu.tw/a.pdf` | 接受 | 官方教育網域 |
| `https://www.moe.gov.tw/notice` | 接受 | 官方政府網域 |
| `https://dept.school.edu.tw/` | 接受 | 官方教育網域的子網域 |
| `https://school.edu.tw.example.com/a.pdf` | 拒絕 | 真正主機是 `example.com` |
| `https://drive.google.com/a.pdf` | 拒絕 | 第三方檔案主機 |
| `https://example.com/school.edu.tw/a.pdf` | 拒絕 | 網址路徑不是網域 |

只符合 suffix 還不代表一定是指定學校的官方來源。系統仍要用 Agent 自動檢查來源與學校的關聯性，並把檢查證據寫入全域來源管理總清單。可接受的關聯證據包括：

- 已確認的官方頁面直接連到該暫時入口；
- 頁面公開標示學校正式名稱、招生單位或校方識別資訊，且與學校主檔吻合；
- 文件內容、頁面標題與 URL 的學校線索彼此一致。

來源不需要事前人工登錄。Agent 判定證據充分時自動建立或更新來源列；證據不足時建立 `candidate` 列供後續觀察，不直接當成可發布來源。人工只在來源管理總清單中做拒絕、恢復或新增特定例外。

## 重新導向與附件

每次重新導向都要驗證 `Location`，最終 URL 也要通過相同規則。官方頁面若連到另一個符合 `.edu.tw`／`.gov.tw` 的臨時入口，可以由 Agent 自動發現並建立來源列；仍須驗證該入口的公開內容與學校關聯。官方頁面若連到第三方雲端硬碟、論壇或其他非允許網域，嚴格模式下拒絕自動取得；保留官方頁面作為線索即可。不要因為第三方檔案內容看起來像簡章，就把它當成官方來源。

## HTTP 安全界線

網址驗證腳本不負責 DNS 或網路安全。實際 fetcher 必須另外：

- 只使用 `http`／`https`，通常只允許 80／443；
- 拒絕 userinfo、localhost、環回、鏈路本地、私有及保留 IP；
- 每次連線與重新導向重新解析並檢查 IP，防止 DNS rebinding；
- 限制回應大小、下載時間、重新導向次數與內容類型；
- 不傳送 Authorization、Cookie 或使用者資料。

## 來源證據

來源證據至少包含：`source_url`、`final_url`、`retrieved_at`、`sha256`、`content_type`、`document_title`、`page_or_locator`、`evidence_text`。搜尋結果、第三方摘要及模型推論不能取代 `evidence_text`。

## 公開性界線

只處理不需帳號、登入、Email magic link、驗證碼或個人權限即可取得的內容。公開榜單可以整理；寄給個人的錄取信、個人信箱內容及登入後通知不處理，也不要求使用者轉寄給系統。
