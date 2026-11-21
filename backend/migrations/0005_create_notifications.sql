BEGIN;

CREATE TABLE notifications (
    id integer PRIMARY KEY,
    user_id integer NOT NULL,
    message text NOT NULL,
    created_at_ticks bigint NOT NULL CHECK (created_at_ticks BETWEEN 0 AND 3155378975999999999),
    target_date_ticks bigint NOT NULL CHECK (target_date_ticks BETWEEN 0 AND 3155378975999999999),
    is_read boolean NOT NULL DEFAULT false,
    type text NOT NULL,
    target_reservation_id integer,
    target_work_schedule_entry_id integer
);

CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at_ticks DESC);

COMMIT;
