CREATE TABLE IF NOT EXISTS bot_config
(
  id   INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS facts
(
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  content     TEXT NOT NULL,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  relevance   INTEGER  DEFAULT 50 CHECK (relevance BETWEEN 1 AND 100)
);

CREATE TABLE IF NOT EXISTS fact_tags
(
  fact_id INTEGER NOT NULL,
  tag     TEXT    NOT NULL,
  FOREIGN KEY (fact_id) REFERENCES facts (id) ON DELETE CASCADE,
  PRIMARY KEY (fact_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_fact_tags_tag ON fact_tags (tag);
CREATE INDEX IF NOT EXISTS idx_facts_relevance ON facts (relevance DESC);
