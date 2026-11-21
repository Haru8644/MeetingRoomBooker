BEGIN;

CREATE TABLE work_schedule_entries (
    id integer PRIMARY KEY,
    created_by_user_id integer NOT NULL,
    type integer NOT NULL,
    title text NOT NULL,
    series_id text,
    date_ticks bigint NOT NULL CHECK (date_ticks BETWEEN 0 AND 3155378975999999999),
    start_time_ticks bigint CHECK (start_time_ticks BETWEEN 0 AND 3155378975999999999),
    end_time_ticks bigint CHECK (end_time_ticks BETWEEN 0 AND 3155378975999999999),
    repeat_type text,
    repeat_until_ticks bigint CHECK (repeat_until_ticks BETWEEN 0 AND 3155378975999999999),
    leave_period integer NOT NULL,
    participant_ids integer[] NOT NULL DEFAULT '{}' CHECK (array_position(participant_ids, NULL) IS NULL),
    participants text NOT NULL,
    created_at_ticks bigint NOT NULL CHECK (created_at_ticks BETWEEN 0 AND 3155378975999999999),
    updated_at_ticks bigint NOT NULL CHECK (updated_at_ticks BETWEEN 0 AND 3155378975999999999)
);

CREATE INDEX work_schedule_entries_date_idx ON work_schedule_entries (date_ticks);

COMMIT;
