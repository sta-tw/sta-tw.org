# Database migrations

`000001_initial.sql` 建立 Phase 2 的核心 PostgreSQL 結構。

後續 migration：

- `000002_auth_sessions.sql`：原生 session、CSRF、Email challenge、OAuth state／PKCE。
- `000003_ingestion.sql`：RabbitMQ 工作紀錄與簡章解析候選資料隔離區。
- `000004_portfolio.sql`：單一 admin role、備審專案、檔案版本／審核狀態與 append-only 上傳事件。
- `000005_results.sql`：官方榜單、意願值、兩輪詢問與匿名化查榜資料。
- `000006_content_chat.sql`：年度／校系論壇、心得 revision／審核，以及網站 canonical 的 Discord／Telegram 閒聊同步 outbox。
- `000007_verification.sql`：學校信箱白名單、學生驗證申請、私有在校證明與長期驗證結果。
- `000008_notifications.sql`：加密站內通知與 Email outbox。
- `000009_inquiry_notifications.sql`：查榜兩輪詢問的通知狀態與重試欄位。
- `000010_brochure_files.sql`：官方簡章最新檔案的上傳／審核／上架／下架歷程。
- `000011_school_master_seed.sql`：依教育部 114 學年度廣義大專校院名錄建立 149 筆學校主檔與三位數 STA 內碼。
- `000012_result_response_indexes.sql`：補上意願回覆輪次與通知 worker 的查詢索引。
- `000013_forum_space_bootstrap.sql`：建立論壇各層級的唯一性索引，並初始化公開全域論壇。
- `000014_candidate_number_uniqueness.sql`：限制同年度／校系的已確認申請不可共用准考證 lookup hash，避免查榜結果歧義匹配。
- `000024_brochure_discovery_roster.sql`：凍結簡章探索的 149 間學校名冊，避免學校主檔臨時增減改變年度排程。
- `000015_cross_check_willingness_array.sql`：建立校系純數值意願陣列、官方錄取資料的陣列位置，以及與官方資料列關聯的意願詢問／回覆欄位。
- `000016_support_tickets.sql`：通用客服 Ticket、canonical 對話、Discord／Email durable outbox 與客服事件紀錄。
- `000017_admission_sources.sql`：官方招生來源登錄、關聯證據與管理端審核狀態。
- `000018_support_attachments.sql`：客服 Ticket 私有附件 metadata 與限制。
- `000019_support_email_inbound.sql`：簽章 Email webhook 去重與 canonical 回覆來源。
- `000020_admin_mfa.sql`：管理員 TOTP MFA 加密 seed。
- `000021_rate_limit_buckets.sql`：跨 API 副本共用的固定視窗限流 bucket。
- `000022_brochure_discovery.sql`：以完整校院母表建立 115 學年度簡章探索佇列、五種作業狀態與 append-only 事件。
- `000023_brochure_discovery_cycles.sql`：將學年度改為可建立、啟動與結束的週期，同時支援人工確認該校無簡章。
- `000025_telegram_cross_check.sql`：**可選的 Telegram adapter 擴充**，提供 Telegram 身分對應、意願詢問 durable outbox、callback 冪等鍵與回覆來源紀錄；核心交叉查榜不依賴這個 migration。
- `000026_telegram_cross_check_reconciliation.sql`：**可選的 Telegram adapter 擴充**，在榜單取代與結果更正時封鎖過期 Telegram delivery；核心交叉查榜不依賴這個 migration。
- `000017_admission_sources.sql`：官方招生來源登錄、關聯證據與管理端審核狀態。
- `000018_support_attachments.sql`：客服 Ticket 私有附件 metadata 與限制。
- `000019_support_email_inbound.sql`：簽章 Email webhook 去重與 canonical 回覆來源。
- `000020_admin_mfa.sql`：管理員 TOTP MFA 加密 seed。

招生科系資料不另建管理專用表；管理端 API 直接以 transaction 維護 `academic_programs` 與 `program_exam_items`，並以 `review_status` 控制待審／公開狀態，異動寫入既有 append-only `audit_log`。

## 已鎖定的資料規則

- 校系識別碼與資料序號是 `學年度-學校編號-科系編號`，例如 `116-001-023`；資料庫另以三個欄位做關聯。
- 甲組、乙組、丙組使用不同的科系編號，獨立保存名額、考試項目與申請資料。
- 建檔時人工填寫學年度、學校編號、類組、頁碼與其他招生內容；`program_identifier` 與 `source_locator` 不接受人工輸入，由資料庫依欄位自動產生。
- 招生簡章以 `學年度-學校編號.pdf` 保存，每年度每校只保留最新版本；事件表保留歷次上傳紀錄。
- 簡章檔案 API、審核／上架／下架與事件紀錄格式見 [docs/brochure-file-api.md](../docs/brochure-file-api.md)。
- 備審資料專案、檔案版本、送審／審核、上下架與事件 API 見 [docs/portfolio-api.md](../docs/portfolio-api.md)。
- 查榜結果批次、准考證匹配、匿名化參考機率與兩輪意願詢問 API 見 [docs/results-api.md](../docs/results-api.md)。
- 定位編號由學校編號與頁碼產生；沒有頁碼時為 `NULL`，API 對外轉為 `-`。
- 未提供的日期保留為 typed `NULL`，API 對外轉為 `-`；文字欄位預設為 `-`。
- 使用者確認申請後由 `applications.status = 'confirmed'` 表示鎖定；追加申請必須建立客服單。
- 准考證號碼、Email、OAuth subject 只保存加密值或不可逆 lookup hash。
- `audit_log` 是 append-only；修改前後資料必須先由應用層移除個資與機密。
- 備審檔案上傳先是 `hidden`，送出後為 `pending_review`，審核通過為 `published`；作者可把已公開檔案改為 `unpublished` 或 `hidden`。
- 備審檔案本體只放 private object storage；資料庫保存 checksum、版本與狀態，不保存檔案內容。
- 學生證明檔、學校信箱與一次性驗證資料是年度可清除資料；`student_verifications` 只保存驗證結果與年度／學校／科系欄位。
- Email 與查榜提醒透過加密 payload 的 durable outbox；API 不直接連 SMTP。

## 套用方式

`cmd/migrate` 預設建立 core schema，會跳過 000025／000026 這兩個 optional Telegram adapter migration；若要讓 `cmd/api` 掛載 Telegram cross-check adapter，必須在該環境設定 `STA_TELEGRAM_CROSS_CHECK_TOKEN`，並使用 `cmd/migrate -include-telegram`。若資料庫已經套用它們，仍會驗證名稱與 SHA-256 checksum。migration runner 會建立 `schema_migrations`、以 advisory lock 防止並行套用，並驗證已套用檔案的名稱與 SHA-256 checksum。正式環境禁止直接手動修改已套用的 migration。
