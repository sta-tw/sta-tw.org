# 招生資料回報表單 API

公開表單會把招生人數或歷屆簡章回報直接存入本站資料庫，不使用 Google Forms 或 Google Sheets 作為收件端。

## 公開送出

```http
POST /api/v1/admissions/contributions
Content-Type: multipart/form-data
```

欄位：

- `email`：必要，作為回覆聯絡方式；資料庫只保存欄位加密後的內容。
- `academic_year`：必要，三位數學年度。
- `category`：`applicants`、`second_stage`、`waitlist` 或 `historical_brochure`。
- `school_name`：前三種人數回報必填。
- `program_name` 與 `people_count`：前三種人數回報手動填寫時以重複欄位成對送出，每組代表一個校系名稱與非負整數人數；最多 100 組。後台保存為每行 `XX 系 XX 人`。
- `data_text`：舊版前端相容欄位；前三種人數回報仍接受每行一筆的逗號格式或 `XX 系 XX 人` 格式，並會正規化後保存。
- `file`：前三種人數回報選擇上傳方式時可重複送出，最多 5 個，每個最多 10 MB；支援 PDF、CSV、TSV、TXT、JSON、DOC、DOCX、PPT、PPTX、XLS、XLSX、JPG、PNG、GIF、WebP。歷屆簡章回報限一個 PDF，最多 10 MB，或提供 `reference_url`。
- `reference_url`：可選 HTTP(S) 網址；歷屆簡章回報須提供 PDF 或網址。
- `nickname`：可選。
- `publish_nickname`：`true` 表示同意將暱稱列入感謝名單；只有管理員採用回報後才會公開。
- `website`：隱藏的反垃圾欄位，正常送出時留空。
- `turnstile_token`：Cloudflare Turnstile 驗證 token；正式環境會驗證 action 與 hostname，驗證失敗或服務無法使用時拒絕收件。

人數回報須擇一手動填寫或上傳檔案。上傳檔案會驗證副檔名、MIME type 與檔案標頭，並通過 ClamAV 掃描；掃描器未設定或無法使用時拒絕上傳。檔案存放在非公開物件儲存空間，只有招生管理員可經 MFA 後在後台預覽 PDF 或下載檔案。成功回應不包含聯絡信箱或附件資訊。公開表單依受信任 proxy 提供的來源 IP 限制每小時最多 10 次送出。

## 管理與審核

招生管理員或招生版管理員完成 MFA 後可使用下列端點：

```http
GET  /api/v1/admin/admissions/contributions?status=pending&limit=100&offset=0
POST /api/v1/admin/admissions/contributions/{id}/review
GET  /api/v1/admin/admissions/contributions/{id}/content
GET  /api/v1/admin/admissions/contributions/{id}/attachments/{attachmentID}/content
```

審核 body：

```json
{"status":"accepted"}
```

`status` 可為 `accepted` 或 `rejected`。採用且已同意公開的暱稱會出現在 `GET /api/v1/admissions/contributors`；其餘回報資料與聯絡信箱不會公開。

附件內容 API 需要管理員 MFA，從非公開物件儲存空間透過本站後端串流附件，並設定 `nosniff` 與 `no-store` 標頭。管理頁用站內預覽 PDF 並提供原檔下載；瀏覽器不會取得 MinIO 網址。
