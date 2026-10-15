-- Large columns come last so metadata reads do not walk BLOB overflow pages.
CREATE TABLE documents (
    id          TEXT PRIMARY KEY,
    space       TEXT NOT NULL,
    path        TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL,
    sha256      TEXT NOT NULL,
    size        INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    body        BLOB NOT NULL,
    search_text TEXT NOT NULL DEFAULT '',
    UNIQUE (space, path)
);

-- The index keeps no copy of the text. It reads the documents row by rowid,
-- so a VACUUM that renumbers rowids needs the 'rebuild' command.
CREATE VIRTUAL TABLE documents_fts USING fts5(
    title, path, search_text,
    content = 'documents',
    tokenize = 'porter unicode61'
);

CREATE TRIGGER documents_fts_insert AFTER INSERT ON documents BEGIN
    INSERT INTO documents_fts (rowid, title, path, search_text)
    VALUES (new.rowid, new.title, new.path, new.search_text);
END;

CREATE TRIGGER documents_fts_delete AFTER DELETE ON documents BEGIN
    INSERT INTO documents_fts (documents_fts, rowid, title, path, search_text)
    VALUES ('delete', old.rowid, old.title, old.path, old.search_text);
END;

CREATE TRIGGER documents_fts_update AFTER UPDATE ON documents BEGIN
    INSERT INTO documents_fts (documents_fts, rowid, title, path, search_text)
    VALUES ('delete', old.rowid, old.title, old.path, old.search_text);
    INSERT INTO documents_fts (rowid, title, path, search_text)
    VALUES (new.rowid, new.title, new.path, new.search_text);
END;
