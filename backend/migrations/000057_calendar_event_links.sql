CREATE TABLE calendar_event_links (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    google_event_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, external_id),
    CONSTRAINT calendar_event_links_external_id_not_blank CHECK (length(btrim(external_id)) > 0),
    CONSTRAINT calendar_event_links_google_event_id_not_blank CHECK (length(btrim(google_event_id)) > 0)
);

COMMENT ON TABLE calendar_event_links IS
    '一筆使用者透過「新增至日曆」寫入自己 Google 日曆的事件記錄；external_id 由呼叫端提供（例如某個招生時程項目的穩定識別碼），用來之後對應刪除該筆 Google 日曆事件。';
