-- An agent's review of a hive session's branch diff. One per session and
-- author: a new review from the same author replaces the old one.
CREATE TABLE diff_reviews (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    author TEXT NOT NULL,
    agent TEXT NOT NULL,
    head TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (session_id, author)
);

CREATE TABLE diff_review_comments (
    id TEXT PRIMARY KEY,
    review_id TEXT NOT NULL REFERENCES diff_reviews(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    start_side TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_side TEXT NOT NULL,
    end_line INTEGER NOT NULL,
    quote TEXT NOT NULL,
    text TEXT NOT NULL,
    severity TEXT NOT NULL
);

CREATE INDEX idx_diff_review_comments_review_id ON diff_review_comments(review_id);
