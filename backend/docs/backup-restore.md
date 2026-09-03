# 備份與還原 runbook

備份目標是 PostgreSQL 與 private object storage 兩部分；只備份資料庫不能還原備審檔案與學生驗證檔案。

## PostgreSQL

在受控的 backup host 執行：

```sh
pg_dump --format=custom --no-owner --file=sta-$(date -u +%Y%m%dT%H%M%SZ).dump "$STA_DATABASE_URL"
```

備份檔應再使用 KMS／受控金鑰加密、限制存取、記錄 checksum，並遵守學生驗證檔案的最短保留政策。不要把 `STA_DATABASE_URL` 或密碼寫入 shell history、CI log 或一般監控 log。

還原到隔離資料庫後執行：

```sh
createdb sta_restore_check
pg_restore --clean --if-exists --no-owner --dbname="$STA_RESTORE_DATABASE_URL" sta-YYYYmmddTHHMMSSZ.dump
go run ./cmd/migrate -dir migrations
```

還原檢查至少包含：`/readyz`、登入、權限隔離、private object signed URL、查榜匿名化、append-only audit trigger，以及年度清理紀錄。確認完成前不要覆蓋正式資料庫。

## Object storage

- 啟用 bucket versioning、server-side encryption、限制 API key 只能讀寫指定 bucket。
- 備份 `portfolio/`、`verification/` 物件與其 metadata；驗證物件不可因一般備份無限期保留。
- 年度清理前先確認備份 retention 已符合資料政策；清理後同步刪除 live 與不再需要的 backup 副本。
- 還原測試不得把真實學生證明放入開發環境；使用合成檔案與假的帳號。
