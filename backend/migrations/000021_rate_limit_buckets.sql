-- STA Phase 12: cross-replica fixed-window rate limiting.

CREATE TABLE rate_limit_buckets (
    bucket_key TEXT PRIMARY KEY,
    window_started TIMESTAMPTZ NOT NULL,
    request_count INTEGER NOT NULL CHECK (request_count >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT rate_limit_bucket_key_not_blank CHECK (length(btrim(bucket_key)) > 0),
    CONSTRAINT rate_limit_bucket_window_valid CHECK (expires_at > window_started)
);

CREATE INDEX rate_limit_buckets_expiry_idx
ON rate_limit_buckets (expires_at);

COMMENT ON TABLE rate_limit_buckets IS '跨 API 副本共用的短期固定視窗限流 bucket；可安全清理過期列。';
