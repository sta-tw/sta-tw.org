-- Different mail categories warrant different reply wording (e.g. 帳號問題
-- vs 簡章回報) — this is a per-route override of the global
-- app_settings.mail_reply_template. NULL means "use the global default",
-- matching how visible_role_ids' empty case already means "leave as-is".
ALTER TABLE mail_routes ADD COLUMN reply_template text;
