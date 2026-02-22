CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS bot_config
(
  id   INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS facts
(
  id          SERIAL PRIMARY KEY,
  content     TEXT NOT NULL,
  tags        JSONB NOT NULL DEFAULT '[]',
  usernames   JSONB NOT NULL DEFAULT '[]',
  embedding   vector(1536),
  created_at  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_facts_embedding ON facts USING ivfflat (embedding vector_cosine_ops);
