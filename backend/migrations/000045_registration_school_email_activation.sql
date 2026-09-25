-- STA: registration now requires a school (*.edu.tw) email up front. The
-- account is created inactive ('pending_verification') and a password-set
-- link is emailed straight to that school address; consuming it both sets
-- the password and (via the new activates_account flag) flips the account
-- to active + identity_status='student' in the same transaction. People
-- without a school email never hit this path at all — they apply through
-- account_applications instead, which still creates an active account
-- directly on admin approval.

ALTER TABLE accounts DROP CONSTRAINT accounts_account_status_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_account_status_check
    CHECK (account_status IN ('pending_verification', 'active', 'suspended', 'deleted'));

ALTER TABLE password_reset_challenges
    ADD COLUMN activates_account BOOLEAN NOT NULL DEFAULT FALSE;
