BEGIN;

CREATE TABLE users (
    id integer PRIMARY KEY,
    name text NOT NULL,
    email text NOT NULL UNIQUE,
    password_hash text,
    avatar_color text NOT NULL DEFAULT '#58a6ff',
    is_admin boolean NOT NULL DEFAULT false,
    chatwork_account_id text,
    chatwork_direct_room_id text
);

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id integer NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    remember_me boolean NOT NULL,
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > issued_at)
);

CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

COMMIT;
