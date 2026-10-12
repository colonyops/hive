CREATE TABLE accounts (
  email       TEXT PRIMARY KEY,
  encryption  INTEGER NOT NULL DEFAULT 0,
  bytes_used  INTEGER NOT NULL DEFAULT 0,
  created_at  INTEGER NOT NULL
);
CREATE TABLE tokens (
  id           TEXT PRIMARY KEY,
  email        TEXT NOT NULL REFERENCES accounts(email) ON DELETE CASCADE,
  hash         TEXT NOT NULL UNIQUE,
  kind         TEXT NOT NULL,
  device       TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER
);
CREATE TABLE sources (
  id           TEXT PRIMARY KEY,
  email        TEXT NOT NULL REFERENCES accounts(email) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  ingress_hash TEXT NOT NULL UNIQUE,
  secret_hash  TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL
);
CREATE INDEX sources_email ON sources(email);
CREATE TABLE deliveries (
  id          TEXT PRIMARY KEY,
  source_id   TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  received_at INTEGER NOT NULL,
  enc         INTEGER NOT NULL DEFAULT 0,
  size        INTEGER NOT NULL,
  record      BLOB NOT NULL
);
CREATE INDEX deliveries_source_id ON deliveries(source_id, id);
