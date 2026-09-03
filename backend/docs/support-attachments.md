# 客服附件

客服訊息可使用 `multipart/form-data` 上傳附件；文字 JSON 請求仍然相容。

- 欄位：`body` 與一個或多個 `attachments` 檔案欄位。
- 每則訊息最多 5 個檔案；單檔最多 10 MiB；單則訊息合計最多 25 MiB。
- 檔案會先寫入 private object storage，再以同一個資料庫 transaction 建立 metadata；資料庫失敗時會清理暫存 object。
- 回應只包含檔名、MIME、大小與 SHA-256，不回傳 storage key。
- 使用者只能下載自己 Ticket 的附件；管理員可從管理端下載，URL 有效期 5 分鐘。

下載端點：

```http
GET /api/v1/support/tickets/{ticketID}/attachments/{attachmentID}/download
GET /api/v1/admin/support/tickets/{ticketID}/attachments/{attachmentID}/download
```

檔案型別與 signature 檢查由共用 storage layer 執行，並在 object storage 前由設定的 ClamAV `INSTREAM` scanner 掃描；掃描器故障時不會寫入 object storage。設定與正式環境限制見 [file-scanning.md](file-scanning.md)。

客服人員的 Email 回覆使用獨立的 signed webhook，不接受任意 SMTP 轉寄。詳見 `POST /api/v1/support/webhooks/email` 與 `STA_SUPPORT_EMAIL_WEBHOOK_SECRET`。
