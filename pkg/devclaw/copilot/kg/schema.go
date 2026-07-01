package kg

// KgSchema contains all CREATE TABLE and CREATE INDEX statements for the
// Knowledge Graph bitemporal store. All statements are idempotent (IF NOT EXISTS).
const KgSchema = `
-- Entity registry
CREATE TABLE IF NOT EXISTS kg_entities (
    entity_id INTEGER PRIMARY KEY AUTOINCREMENT,
    canonical_name TEXT UNIQUE NOT NULL,
    embedding BLOB NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_kg_entities_canonical
    ON kg_entities(canonical_name);

-- Alias table for case/accent-insensitive entity resolution
CREATE TABLE IF NOT EXISTS kg_entity_aliases (
    alias_id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_id INTEGER NOT NULL REFERENCES kg_entities(entity_id) ON DELETE CASCADE,
    alias_name TEXT UNIQUE NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_kg_aliases_name
    ON kg_entity_aliases(alias_name);

CREATE INDEX IF NOT EXISTS idx_kg_aliases_entity
    ON kg_entity_aliases(entity_id);

-- Predicate metadata
CREATE TABLE IF NOT EXISTS kg_predicates (
    predicate_id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    is_functional BOOLEAN NOT NULL DEFAULT 0,
    description TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_kg_predicates_name
    ON kg_predicates(name);

-- SPO triples with bitemporal tracking
CREATE TABLE IF NOT EXISTS kg_triples (
    triple_id INTEGER PRIMARY KEY AUTOINCREMENT,
    subject_entity_id INTEGER NOT NULL REFERENCES kg_entities(entity_id) ON DELETE SET NULL,
    predicate_id INTEGER NOT NULL REFERENCES kg_predicates(predicate_id) ON DELETE CASCADE,
    object_text TEXT NOT NULL,
    object_entity_id INTEGER NULL REFERENCES kg_entities(entity_id) ON DELETE SET NULL,

    -- Valid time: when the fact is true in the real world
    valid_from    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_until   DATETIME NULL,

    -- Transaction time: when the row exists in the database
    txn_from      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    txn_until     DATETIME NULL,

    -- Metadata
    confidence REAL NOT NULL DEFAULT 0.5,
    source_memory_id TEXT NOT NULL DEFAULT '',
    wing TEXT NULL,
    raw_text TEXT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP

    -- INVARIANT: For functional predicates, only one triple per (subject, predicate)
    -- may have valid_until IS NULL. Enforced in Go transaction layer (Room 3.2).
    -- INVARIANT: object_text is always populated. object_entity_id is set only for entity refs.
    -- INVARIANT: wing IS NULL is neutral — never filtered, never penalized.
);

-- Prevent duplicate current triples (partial unique index)
CREATE UNIQUE INDEX IF NOT EXISTS idx_kg_triples_active_unique
    ON kg_triples(subject_entity_id, predicate_id, object_text)
    WHERE valid_until IS NULL AND txn_until IS NULL;

-- Primary query: current facts about an entity (covering partial index)
CREATE INDEX IF NOT EXISTS idx_kg_triples_subject_current
    ON kg_triples(subject_entity_id, predicate_id)
    WHERE valid_until IS NULL AND txn_until IS NULL;

-- Historical queries
CREATE INDEX IF NOT EXISTS idx_kg_triples_subject
    ON kg_triples(subject_entity_id, predicate_id, valid_until);

CREATE INDEX IF NOT EXISTS idx_kg_triples_object
    ON kg_triples(object_entity_id, valid_until);

CREATE INDEX IF NOT EXISTS idx_kg_triples_valid_time
    ON kg_triples(valid_from, valid_until);

CREATE INDEX IF NOT EXISTS idx_kg_triples_source
    ON kg_triples(source_memory_id);
`

// KgDropSchema reverses the KG migration. Drops KG tables and indexes.
// Safe to call on any database — uses IF EXISTS.
const KgDropSchema = `
DROP INDEX IF EXISTS idx_kg_triples_source;
DROP INDEX IF EXISTS idx_kg_triples_valid_time;
DROP INDEX IF EXISTS idx_kg_triples_object;
DROP INDEX IF EXISTS idx_kg_triples_subject;
DROP INDEX IF EXISTS idx_kg_triples_subject_current;
DROP INDEX IF EXISTS idx_kg_triples_active_unique;
DROP TABLE IF EXISTS kg_triples;
DROP INDEX IF EXISTS idx_kg_predicates_name;
DROP TABLE IF EXISTS kg_predicates;
DROP INDEX IF EXISTS idx_kg_aliases_entity;
DROP INDEX IF EXISTS idx_kg_aliases_name;
DROP TABLE IF EXISTS kg_entity_aliases;
DROP INDEX IF EXISTS idx_kg_entities_canonical;
DROP TABLE IF EXISTS kg_entities;
`
