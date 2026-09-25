-- return_to lets the OAuth login/bind flow send the browser back to the
-- frontend page it started from (e.g. a brochure detail page waiting to
-- retry an "add to calendar" request) instead of always landing on a fixed
-- default. It must be a same-origin relative path; the handler validates
-- that before ever storing or redirecting to it.
ALTER TABLE oauth_states
    ADD COLUMN return_to TEXT;

-- One row per account that has granted Google Calendar write access. Google
-- only issues a refresh_token on the *first* consent for a given
-- client+scope pair (or when the authorization request forces
-- prompt=consent), so it is captured once here and reused to mint short-
-- lived access tokens on demand — the user is never asked to re-authorize
-- unless they revoke access on Google's side, which surfaces as an
-- invalid_grant error and clears the row.
CREATE TABLE oauth_calendar_grants (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    provider VARCHAR(16) NOT NULL
        CHECK (provider = 'google'),
    refresh_token_ciphertext BYTEA NOT NULL,
    scope TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT oauth_calendar_grants_refresh_token_not_empty CHECK (octet_length(refresh_token_ciphertext) > 0),
    CONSTRAINT oauth_calendar_grants_scope_not_blank CHECK (length(btrim(scope)) > 0)
);

CREATE TRIGGER oauth_calendar_grants_set_updated_at
BEFORE UPDATE ON oauth_calendar_grants
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
