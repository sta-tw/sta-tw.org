# 簡章系統＋Telegram Bot 隔離測試環境

此資料夾只用於本機驗收簡章上傳／審核／上架、抽取 worker 與 Telegram smoke bot。它不讀取也不修改專案根目錄的 `.env`。

隔離邊界：

- `STA_ENV=test`，API 綁定 `127.0.0.1:58080`。
- PostgreSQL 使用獨立資料庫 `sta_brochure_tg_test` 與連接埠 `55432`。
- RabbitMQ 使用獨立 exchange／queue 與連接埠 `55672`。
- MinIO 使用獨立 bucket `sta-brochure-tg-test` 與連接埠 `59000`。
- ClamAV 使用測試連接埠 `53310`。
- Docker Compose project 固定為 `sta-brochure-tg-test`，volume 與其他專案分離。
- `run` 會先清除從目前 shell 繼承的所有 `STA_*`，再只載入本資料夾的 `.env`。
- `.env` 與 `.venv` 已被忽略，不會加入版本控制。

## 第一次啟動

目前工作區已建立專用 `.env`。請只把「獨立測試 Bot」token 填入：

```env
STA_TELEGRAM_BOT_TOKEN=<測試 Bot token>
```

不要使用正式 Bot token；若要限制 Bot，只填測試聊天室 ID 到 `STA_TELEGRAM_BOT_ALLOWED_CHAT_IDS`。新環境可從 `.env.example` 複製一份 `.env`，並重新產生兩組 32-byte key：

```sh
openssl rand -base64 32
openssl rand -base64 32
```

先檢查隔離護欄，再啟動基礎服務與 migration：

```sh
cd test-env/brochure-tg
./run env-check
./run infra-up
./run migrate
```

`infra-up` 第一次可能需要下載容器 image；ClamAV 第一次準備病毒碼通常最久。

## 啟動測試程序

每個常駐程序各開一個終端：

```sh
./run api
./run go-worker
./run python-install  # 只需第一次執行
./run python-worker
./run console
./run bot
```

控制台位於 <http://127.0.0.1:54173>。切換為 `Live API 模式`，API Base URL 填入 `http://127.0.0.1:58080`。

若只驗證 TG 讀取鏈路，必要程序是基礎服務、migration、API 與 Bot；資料庫中仍須至少有一份狀態為 `published` 的簡章。Python／Go extraction workers 只有在驗證 PDF 自動抽取時才必須啟動。

## 管理員與簡章驗收

1. 透過隔離 API 建立測試帳號。
2. 將該帳號升級為此測試資料庫的管理員：

   ```sh
   ./run bootstrap-admin <測試帳號>
   ```

3. 在控制台 Live API 模式填入該帳號的 opaque bearer token。
4. 上傳 PDF、人工核准並確認簡章狀態為 `published`。
5. 先執行 `./run check`，再到 Telegram 測試：

   ```text
   /id
   /health
   /brochure 116 001
   ```

若測試 Bot 原先有 webhook，long polling 前明確執行：

```sh
./run telegram-delete-webhook
```

這只可對 `.env` 中的測試 Bot 使用。

## 簡章探索（選配）

要驗證 149 校探索流程，先確認 `./run infra-up` 已啟動隔離 SearXNG（`STA_SEARXNG_URL=http://127.0.0.1:58888`，設定檔已開啟 JSON 結果格式），設定 API 與 worker 共用至少 32 字元的 `STA_BROCHURE_DISCOVERY_AGENT_TOKEN` 後，啟動週期並執行：

```sh
./run discovery
```

`certs/tw-edu-intermediates.pem` 已由 `STA_BROCHURE_DISCOVERY_CA_BUNDLE` 掛上，補足部分 `.edu.tw` 站台未附的 TWCA 中繼 CA。探索使用本地規則，不需要 `STA_AI_*` 或搜尋 API 金鑰；若 PDF 文字不足以確認，會保守回報無候選。其他簡章上傳、審核、上架與 TG 查詢流程不受影響。

## 檢查與清理

```sh
./run infra-status
./run infra-logs
./run test
./run infra-down
```

`infra-down` 保留測試資料。只有確定要刪除隔離資料庫、bucket、queue 與病毒碼 volume 時才執行：

```sh
./run reset --yes
```

`reset` 的 Compose project 名稱固定，只會鎖定此測試環境的 resources；正式環境不在其範圍內。
