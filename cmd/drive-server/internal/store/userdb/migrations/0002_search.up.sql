ALTER TABLE documents ADD COLUMN title TEXT NOT NULL DEFAULT '';

-- The index keeps no copy of the text. It reads the documents row by rowid,
-- so a VACUUM that renumbers rowids needs the 'rebuild' command below.
CREATE VIRTUAL TABLE documents_fts USING fts5(
    title, path, body,
    content = 'documents',
    tokenize = 'porter unicode61'
);

CREATE TRIGGER documents_fts_insert AFTER INSERT ON documents BEGIN
    INSERT INTO documents_fts (rowid, title, path, body)
    VALUES (new.rowid, new.title, new.path, new.body);
END;

CREATE TRIGGER documents_fts_delete AFTER DELETE ON documents BEGIN
    INSERT INTO documents_fts (documents_fts, rowid, title, path, body)
    VALUES ('delete', old.rowid, old.title, old.path, old.body);
END;

CREATE TRIGGER documents_fts_update AFTER UPDATE ON documents BEGIN
    INSERT INTO documents_fts (documents_fts, rowid, title, path, body)
    VALUES ('delete', old.rowid, old.title, old.path, old.body);
    INSERT INTO documents_fts (rowid, title, path, body)
    VALUES (new.rowid, new.title, new.path, new.body);
END;

INSERT INTO documents_fts (documents_fts) VALUES ('rebuild');
