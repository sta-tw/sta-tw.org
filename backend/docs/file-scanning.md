# 上傳檔案掃描

所有招生簡章、備審資料、學生證明與客服附件，在寫入 private object storage 前都會先送至 ClamAV `INSTREAM` 掃描。`STA_CLAMAV_ADDRESS` 支援 `tcp://host:3310` 與 `unix:///path/to/clamd.ctl`。

正式環境若已啟用 object storage，`STA_REQUIRE_FILE_SCAN=true`（預設值）時沒有掃描器設定會拒絕 API 啟動。掃描結果為 `FOUND` 會回傳 `422 malware_detected`；掃描器連線或回應異常會回傳 `503 scan_unavailable`，檔案不會送入 object storage。

ClamAV 不是解壓縮炸彈的完整政策引擎；正式環境仍應設定病毒庫更新、資源上限、隔離網路與 ZIP／Office 解壓縮資源監控，並在備份還原演練中使用合成檔案。
