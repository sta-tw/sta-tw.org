-- STA Phase 9: indexes for inquiry response privacy checks and notification claiming.

CREATE INDEX willingness_response_events_inquiry_idx
ON willingness_response_events (inquiry_id);

