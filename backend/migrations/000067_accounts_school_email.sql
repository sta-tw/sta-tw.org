-- The school email typed at registration was only ever used in-memory to
-- send the activation link (Register in internal/auth/service.go) and never
-- persisted anywhere on the account — so a mistyped address left the account
-- permanently stuck pending_verification with no record of what was typed,
-- and no way for an admin to correct it. Store it going forward so it can be
-- shown and corrected in the admin user-management UI. Nullable: accounts
-- created via CreateApprovedAccount/CreateAccountWithPassword/CreateBotAccount
-- never went through the school-email registration path and have none.
ALTER TABLE accounts ADD COLUMN school_email_ciphertext bytea;
ALTER TABLE accounts ADD COLUMN school_email_lookup_hash bytea;

-- Prevents the same school email being used to register a second account —
-- including after an admin corrects a mistyped address on an existing
-- pending account, which is exactly the "避免被重複註冊" case this is for.
CREATE UNIQUE INDEX accounts_school_email_lookup_hash_key
    ON accounts (school_email_lookup_hash)
    WHERE school_email_lookup_hash IS NOT NULL;
