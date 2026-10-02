-- Notifies on the existing sta_events Postgres NOTIFY channel whenever a
-- program_timeline_events row's start/end date or time changes via an
-- UPDATE statement — including a direct SQL fix run by hand, which the
-- application-level calendar sync hook (admissions.PostgresRepository's
-- replaceTimelineEvents DELETE+INSERT path) never sees. The API process
-- listens for this on the "admissions.timeline_changed" topic and pushes
-- the corrected date to every visitor who already added that event to
-- their Google Calendar (see internal/calendar's SyncEventDates).
--
-- Deliberately AFTER UPDATE only, not INSERT: the app's normal save path
-- (replaceTimelineEvents) deletes and re-inserts every row for a program on
-- every edit, even ones whose dates didn't change — an INSERT trigger would
-- fire for all of them on every unrelated field edit, duplicating the
-- precise diff the app-level hook already computes. A direct SQL fix (this
-- session's typical "UPDATE program_timeline_events SET ... WHERE ...")
-- only ever uses UPDATE, which is exactly the gap this closes.
CREATE OR REPLACE FUNCTION notify_timeline_event_change() RETURNS trigger AS $$
BEGIN
    IF NEW.start_date IS DISTINCT FROM OLD.start_date
        OR NEW.start_time IS DISTINCT FROM OLD.start_time
        OR NEW.end_date IS DISTINCT FROM OLD.end_date
        OR NEW.end_time IS DISTINCT FROM OLD.end_time THEN
        PERFORM pg_notify('sta_events', json_build_object(
            'topic', 'admissions.timeline_changed',
            'kind', 'updated',
            'data', json_build_object(
                'academic_year', NEW.academic_year,
                'school_code', NEW.school_code,
                'program_code', NEW.program_code,
                'sort_order', NEW.sort_order,
                'start_date', NEW.start_date,
                'end_date', NEW.end_date
            )
        )::text);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS program_timeline_events_notify_change ON program_timeline_events;
CREATE TRIGGER program_timeline_events_notify_change
    AFTER UPDATE ON program_timeline_events
    FOR EACH ROW
    EXECUTE FUNCTION notify_timeline_event_change();
