# 交叉查榜＋Telegram adapter 隔離驗收

這套隔離流程會把 Telegram adapter 接到目前的 `cmd/api` 與核心 `internal/results` transaction。核心查榜仍可不設定 Telegram token 獨立運作；本資料夾的 `migrate` 會明確套用 `000025`／`000026` adapter migration。

這個資料夾的目的，是驗證來源專案真的完成交叉查榜與 Telegram 串接；不是建立另一套縮小版系統。`flow`、`verify`、`status` 與 `simulate-starts` 全部呼叫執行中的 STA HTTP API，不含 mock、不直接查詢或修改 PostgreSQL，也不會讀取專案根目錄 `.env`。

唯一直接碰觸資料庫的步驟是既有 `cmd/bootstrap-admin`：全新資料庫沒有管理員，因此必須先把透過註冊 API 建立的第一個隔離帳號授予 admin role。完成 bootstrap 後，校系建立／審核、測試名單同步、官方榜單匯入／發布與 outbox 驗證都只走 API。

## 第一次啟動

在不同終端依序執行：

```sh
cd test-env/cross-check-tg
./run env-check
./run infra-up
./run migrate
./run api
```

API 啟動後，另一個終端執行：

```sh
./run register-admin
./run bootstrap-admin
./run flow roster.example.json
```

成功條件不是單純顯示「完成」，而是來源 API 回報：

- 測試校系確實是 `published`；
- 官方榜單批次確實是 `published`；
- 榜單每一列都匹配到真實 `applications`；
- Telegram 參與者數量符合 roster；
- `willingness_inquiries` 觸發的 Telegram outbox 數量符合應詢問人數；
- 尚未對 Bot 執行 `/start` 的人保持 `waiting_binding`，不會冒充「已傳送」。

`./run verify roster.example.json` 可重做純 API 驗證；`./run status` 顯示來源 API 的 outbox 狀態彙總。

## 換成你提供的名單

複製 `roster.example.json`，填入真實測試用 Telegram numeric user ID、校系與准考證號碼。每個 assignment 同時描述該人的官方結果列，`result_sources` 則按學年度／學校提供官方來源網址與 SHA-256。網址必須符合來源專案既有的 `.edu.tw`／`.gov.tw` 邊界。

名單同步不會讓 Bot 主動建立私人聊天室；Telegram 不允許 Bot 主動聯絡從未啟動 Bot 的帳號。真人測試時，每位測試者必須先在 Bot 私人聊天室送出 `/start`。只有為了不連 Telegram 的 API 驗收，才可明確執行：

```sh
./run simulate-starts --yes my-roster.json
```

這個指令只模擬 `/start` 綁定，會把 outbox 從 `waiting_binding` 改成 `pending`；它不會把訊息標成已送達。啟動真實 Bot 後才會呼叫 Telegram API，成功取得 Telegram message ID 後，來源 API 才會把 delivery 標成 `sent`。

若要再驗證 callback 冪等、歷程與 outbox claim，可在隔離範例資料執行：

```sh
./run simulate-response --yes my-roster.json
./run probe-outbox --yes
```

`simulate-response` 會用同一個 callback ID 呼叫兩次來源 API，並要求歷程仍只有一筆；`probe-outbox` 不會呼叫 Telegram，也不會冒充成功，而是把已領取 delivery 誠實標成可重試的 `failed`。

## 啟動真實 Bot

在本資料夾 `.env` 填入獨立測試用 `STA_TELEGRAM_BOT_TOKEN`，不要沿用正式 Bot。若曾設 webhook：

```sh
./run telegram-delete-webhook
./run bot
```

使用者可使用 `/start`、`/list`、`/pending`、`/status`、`/history`、`/stop`。意見按鈕只顯示文字標籤；0／20／40／60／80／100 只在來源 API 內部轉換與計算，Telegram 訊息與 callback payload 都不含百分比。

測試完成可執行 `./run infra-down` 保留隔離資料；需要清空時才執行 `./run reset --yes`。
