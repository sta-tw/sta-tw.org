-- STA: threaded follow-up correspondence on an account application. Outbound
-- mail related to an application (currently just the rejection notice) uses
-- a deterministic Message-ID (<account-application-<id>@<mail hostname>>) so
-- a reply's In-Reply-To/References can be matched straight back to the
-- application without a separate lookup table.

CREATE TABLE account_application_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES account_applications(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    body TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX account_application_messages_application_idx
    ON account_application_messages (application_id, created_at);
