CREATE TABLE recipients (
  id         TEXT PRIMARY KEY,
  email      TEXT NOT NULL REFERENCES accounts(email) ON DELETE CASCADE,
  recipient  TEXT NOT NULL,
  device     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (email, recipient)
);
CREATE INDEX recipients_email ON recipients(email);
