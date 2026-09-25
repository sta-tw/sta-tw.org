-- event_date/event_time -> structured start/end date+time; source_page dropped
-- (redundant with the program's own source_locator).
ALTER TABLE program_timeline_events
    RENAME COLUMN event_date TO start_date;

ALTER TABLE program_timeline_events
    RENAME COLUMN event_time TO start_time;

ALTER TABLE program_timeline_events
    ADD COLUMN end_date DATE,
    ADD COLUMN end_time TEXT NOT NULL DEFAULT '-';

ALTER TABLE program_timeline_events
    DROP COLUMN source_page;

COMMENT ON COLUMN program_timeline_events.start_time IS '時間（HH:MM，僅時分）；未提供時為 -。';
COMMENT ON COLUMN program_timeline_events.end_date IS '結束日期；留空代表單一天的事件。';
COMMENT ON COLUMN program_timeline_events.end_time IS '結束時間（HH:MM，僅時分）；未提供時為 -。';
