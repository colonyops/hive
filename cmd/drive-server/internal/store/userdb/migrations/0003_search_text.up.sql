ALTER TABLE documents ADD COLUMN search_text TEXT NOT NULL DEFAULT '';

-- Markdown is its own plain text. HTML rows get their text from Go, which
-- strips the markup after the migration (see backfillSearchText).
UPDATE documents SET search_text = CAST(body AS TEXT) WHERE kind = 'markdown';

DROP TRIGGER documents_fts_insert;
DROP TRIGGER documents_fts_delete;
DROP TRIGGER documents_fts_update;
DROP TABLE documents_fts;

-- The index reads search_text instead of body so that markup is not indexed.
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

INSERT INTO documents_fts (documents_fts) VALUES ('rebuild');
