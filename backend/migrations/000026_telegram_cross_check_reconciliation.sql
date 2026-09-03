-- Keep Telegram delivery eligibility aligned with the canonical published
-- official-result state. Superseded lists and corrected non-eligible results
-- must never be reactivated by a later /start.

CREATE OR REPLACE FUNCTION enqueue_telegram_willingness_inquiry()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO telegram_willingness_outbox
        (inquiry_id, application_id, account_id, telegram_user_id, chat_id, status)
    SELECT NEW.id,
           NEW.application_id,
           application.account_id,
           link.telegram_user_id,
           link.private_chat_id,
           CASE
               WHEN link.notifications_enabled AND link.private_chat_id IS NOT NULL THEN 'pending'
               WHEN link.started_at IS NOT NULL THEN 'blocked'
               ELSE 'waiting_binding'
           END
    FROM applications application
    JOIN telegram_account_links link ON link.account_id = application.account_id
    JOIN official_results result
      ON result.id = NEW.official_result_id
     AND result.result_status IN ('admitted', 'waitlisted')
    JOIN official_result_batches batch
      ON batch.id = result.batch_id
     AND batch.status = 'published'
    WHERE application.id = NEW.application_id
    ON CONFLICT (inquiry_id) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION sync_telegram_link_outbox()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO telegram_willingness_outbox
        (inquiry_id, application_id, account_id, telegram_user_id, chat_id, status)
    SELECT inquiry.id,
           inquiry.application_id,
           NEW.account_id,
           NEW.telegram_user_id,
           NEW.private_chat_id,
           CASE
               WHEN NEW.notifications_enabled AND NEW.private_chat_id IS NOT NULL THEN 'pending'
               WHEN NEW.started_at IS NOT NULL THEN 'blocked'
               ELSE 'waiting_binding'
           END
    FROM willingness_inquiries inquiry
    JOIN applications application
      ON application.id = inquiry.application_id
     AND application.account_id = NEW.account_id
    JOIN official_results result
      ON result.id = inquiry.official_result_id
     AND result.result_status IN ('admitted', 'waitlisted')
    JOIN official_result_batches batch
      ON batch.id = result.batch_id
     AND batch.status = 'published'
    WHERE NOT EXISTS (
        SELECT 1
        FROM willingness_response_events event
        WHERE event.inquiry_id = inquiry.id
    )
    ON CONFLICT (inquiry_id) DO NOTHING;

    UPDATE telegram_willingness_outbox outbox
    SET chat_id = NEW.private_chat_id,
        status = CASE
            WHEN NEW.notifications_enabled
                 AND NEW.private_chat_id IS NOT NULL
                 AND outbox.status IN ('waiting_binding', 'blocked', 'failed') THEN 'pending'
            WHEN NOT NEW.notifications_enabled
                 AND NEW.started_at IS NOT NULL
                 AND outbox.status IN ('waiting_binding', 'pending', 'failed') THEN 'blocked'
            ELSE outbox.status
        END,
        available_at = CASE
            WHEN NEW.notifications_enabled
                 AND NEW.private_chat_id IS NOT NULL
                 AND outbox.status IN ('waiting_binding', 'blocked', 'failed') THEN CURRENT_TIMESTAMP
            ELSE outbox.available_at
        END,
        leased_at = CASE
            WHEN outbox.status IN ('waiting_binding', 'blocked', 'failed') THEN NULL
            ELSE outbox.leased_at
        END,
        last_error = CASE
            WHEN NEW.notifications_enabled AND NEW.private_chat_id IS NOT NULL THEN NULL
            ELSE outbox.last_error
        END
    WHERE outbox.telegram_user_id = NEW.telegram_user_id
      AND outbox.status NOT IN ('sent', 'responded', 'processing')
      AND EXISTS (
          SELECT 1
          FROM willingness_inquiries inquiry
          JOIN official_results result
            ON result.id = inquiry.official_result_id
           AND result.result_status IN ('admitted', 'waitlisted')
          JOIN official_result_batches batch
            ON batch.id = result.batch_id
           AND batch.status = 'published'
          WHERE inquiry.id = outbox.inquiry_id
            AND NOT EXISTS (
                SELECT 1 FROM willingness_response_events event WHERE event.inquiry_id = inquiry.id
            )
      );
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION reconcile_telegram_outbox_batch_status()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status = 'superseded' AND OLD.status IS DISTINCT FROM NEW.status THEN
        UPDATE telegram_willingness_outbox outbox
        SET status = 'blocked',
            leased_at = NULL,
            last_error = 'official result batch superseded'
        FROM willingness_inquiries inquiry
        WHERE inquiry.id = outbox.inquiry_id
          AND inquiry.result_batch_id = NEW.id
          AND outbox.status NOT IN ('sent', 'responded', 'blocked');
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER official_result_batches_reconcile_telegram_outbox
AFTER UPDATE OF status ON official_result_batches
FOR EACH ROW
EXECUTE FUNCTION reconcile_telegram_outbox_batch_status();

CREATE OR REPLACE FUNCTION reconcile_telegram_outbox_result_status()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.result_status NOT IN ('admitted', 'waitlisted')
       AND OLD.result_status IS DISTINCT FROM NEW.result_status THEN
        UPDATE telegram_willingness_outbox outbox
        SET status = 'blocked',
            leased_at = NULL,
            last_error = 'official result is no longer willingness-eligible'
        FROM willingness_inquiries inquiry
        WHERE inquiry.id = outbox.inquiry_id
          AND inquiry.official_result_id = NEW.id
          AND outbox.status NOT IN ('sent', 'responded', 'blocked');
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER official_results_reconcile_telegram_outbox
AFTER UPDATE OF result_status ON official_results
FOR EACH ROW
EXECUTE FUNCTION reconcile_telegram_outbox_result_status();

UPDATE telegram_willingness_outbox outbox
SET status = 'blocked',
    leased_at = NULL,
    last_error = CASE
        WHEN batch.status <> 'published' THEN 'official result batch superseded'
        ELSE 'official result is no longer willingness-eligible'
    END
FROM willingness_inquiries inquiry
JOIN official_results result ON result.id = inquiry.official_result_id
JOIN official_result_batches batch ON batch.id = result.batch_id
WHERE inquiry.id = outbox.inquiry_id
  AND (batch.status <> 'published' OR result.result_status NOT IN ('admitted', 'waitlisted'))
  AND outbox.status NOT IN ('sent', 'responded', 'blocked');

