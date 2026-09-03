# 限流

API 使用兩層固定視窗限流：單機記憶體做快速拒絕，PostgreSQL 的 `rate_limit_buckets` 做跨副本的原子計數。登入、註冊、Email 驗證、聊天室、客服訊息與學生信箱驗證寄送都已接入共用限流器。

資料庫限流器故障時相關寫入請求會回傳 `503 rate_limit_unavailable`，不會為了可用性而繞過資安限制。過期 bucket 可由維護排程清理：

```sql
DELETE FROM rate_limit_buckets WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '1 day';
```

這項清理只能在受控維護工作中執行，不能由公網 API 觸發。
