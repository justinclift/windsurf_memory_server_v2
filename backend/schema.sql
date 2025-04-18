-- Memory table schema for versioning and archiving
CREATE TABLE IF NOT EXISTS memories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id TEXT NOT NULL,           -- descriptive title/heading
    version INTEGER NOT NULL,          -- version number, increments per memory_id
    content TEXT NOT NULL,             -- memory content
    archived BOOLEAN NOT NULL DEFAULT 0, -- true if archived, false if active
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memories_memory_id ON memories(memory_id);
CREATE INDEX IF NOT EXISTS idx_memories_archived ON memories(archived);
CREATE INDEX IF NOT EXISTS idx_memories_latest_active ON memories(memory_id, version, archived);
