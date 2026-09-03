# 管理員 MFA

正式環境預設要求管理員使用 TOTP MFA。MFA seed 以 `STA_EMAIL_ENCRYPTION_KEY` 加密後才寫入 `account_admin_mfa`；明文 seed 只在設定回應中出現一次，不寫入 log。

設定流程：

1. 已登入的管理員以 CSRF 或 Bearer mutation authorization 呼叫 `POST /api/v1/auth/admin-mfa/setup`。
2. 將回應的 `secret` 或 `otpauth_url` 加入驗證器 App。
3. 將目前六位數驗證碼送至 `POST /api/v1/auth/admin-mfa/enable`。
4. 後續所有 `/api/v1/admin/` 請求都要帶 `X-MFA-Code`；驗證碼允許前後一個 30 秒時間窗。

其他端點：

```http
GET  /api/v1/auth/admin-mfa/status
POST /api/v1/auth/admin-mfa/disable
```

設定 pending seed 15 分鐘後失效。正式環境可用 `STA_REQUIRE_ADMIN_MFA=false` 暫時關閉「尚未啟用 MFA 時的強制註冊」政策，但已啟用的管理員仍須提供 TOTP。
