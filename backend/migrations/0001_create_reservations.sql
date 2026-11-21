CREATE TABLE reservations (
    id integer PRIMARY KEY,
    user_id integer NOT NULL,
    name text NOT NULL,
    room text NOT NULL,
    number_of_people integer NOT NULL,
    type text NOT NULL,
    purpose text NOT NULL,
    series_id text,
    -- .NET DateTime ticks preserve timezone-free values to 100 ns.
    date_ticks bigint NOT NULL CHECK (date_ticks BETWEEN 0 AND 3155378975999999999),
    start_time_ticks bigint NOT NULL CHECK (start_time_ticks BETWEEN 0 AND 3155378975999999999),
    end_time_ticks bigint NOT NULL CHECK (end_time_ticks BETWEEN 0 AND 3155378975999999999),
    participant_ids integer[] NOT NULL DEFAULT '{}' CHECK (array_position(participant_ids, NULL) IS NULL),
    participants text NOT NULL,
    repeat_type text NOT NULL,
    repeat_until_ticks bigint CHECK (repeat_until_ticks BETWEEN 0 AND 3155378975999999999)
);
