-- STA: 000045 added the 'pending_verification' status value (21 chars) but
-- account_status was still VARCHAR(16), so every insert/update setting it
-- failed with "value too long for type character varying(16)". Widen it.

ALTER TABLE accounts ALTER COLUMN account_status TYPE VARCHAR(32);
