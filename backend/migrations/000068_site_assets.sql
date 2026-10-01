-- Small operator-managed binary assets that are served publicly by a stable
-- API path. The static frontend asset remains the fallback when no row exists.
CREATE TABLE site_assets (
    asset_key TEXT PRIMARY KEY,
    content_type TEXT NOT NULL,
    content BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by UUID REFERENCES accounts(id) ON DELETE SET NULL,
    CONSTRAINT site_assets_content_not_empty CHECK (octet_length(content) > 0)
);
