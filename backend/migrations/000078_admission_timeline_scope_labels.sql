-- Programme applicability is represented by the event's owning programme
-- row. Remove source-sheet scope notes from stored event names.
UPDATE program_timeline_events
SET event_name = regexp_replace(
    event_name,
    '^[（(](全系|全校|除[^）)]+|[^）)]*(系|學程|組)[^）)]*)[）)][[:space:]]*',
    ''
)
WHERE event_name ~ '^[（(](全系|全校|除[^）)]+|[^）)]*(系|學程|組)[^）)]*)[）)][[:space:]]*';
