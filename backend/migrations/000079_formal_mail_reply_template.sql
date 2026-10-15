-- Upgrade only copies of the previous built-in template. Route-specific or
-- global templates that staff have customized remain untouched.
UPDATE app_settings
SET value = E'您好，感謝您的來信，回覆如下：\n\n[內容]\n\n如有其他問題，歡迎直接回覆此信。\n\n敬祝\n順心\n\n[簽名]'
WHERE key = 'mail_reply_template'
  AND value = E'您好，感謝您的來信，回覆如下：\n\n[內容]\n\n如有其他問題，歡迎再次與我們聯繫。\n\n[簽名]';

UPDATE mail_routes
SET reply_template = E'您好，感謝您的來信，回覆如下：\n\n[內容]\n\n如有其他問題，歡迎直接回覆此信。\n\n敬祝\n順心\n\n[簽名]'
WHERE reply_template = E'您好，感謝您的來信，回覆如下：\n\n[內容]\n\n如有其他問題，歡迎再次與我們聯繫。\n\n[簽名]';
