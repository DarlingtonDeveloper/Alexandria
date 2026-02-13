-- Migration 003: Merge context-graph capabilities into Alexandria
-- Adds identity resolution, semantic clustering, provenance, and MCP support.

BEGIN;

-- =============================================================================
-- 2a. Upgrade vault_entities
-- =============================================================================

-- Add new columns
ALTER TABLE vault_entities ADD COLUMN IF NOT EXISTS key TEXT;
ALTER TABLE vault_entities ADD COLUMN IF NOT EXISTS display_name TEXT;
ALTER TABLE vault_entities ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
ALTER TABLE vault_entities ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Backfill key and display_name from existing data
UPDATE vault_entities SET key = entity_type || ':' || name WHERE key IS NULL;
UPDATE vault_entities SET display_name = name WHERE display_name IS NULL;

-- Make key NOT NULL after backfill
ALTER TABLE vault_entities ALTER COLUMN key SET NOT NULL;
ALTER TABLE vault_entities ALTER COLUMN display_name SET NOT NULL;
ALTER TABLE vault_entities ALTER COLUMN display_name SET DEFAULT '';

-- Convert entity_type from enum to TEXT if it's an enum
DO $$ BEGIN
    ALTER TABLE vault_entities ALTER COLUMN entity_type TYPE TEXT USING entity_type::TEXT;
EXCEPTION WHEN others THEN NULL;
END $$;

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_vault_entities_key ON vault_entities (key);
CREATE INDEX IF NOT EXISTS idx_vault_entities_not_deleted ON vault_entities (id) WHERE deleted_at IS NULL;

-- =============================================================================
-- 2b. Upgrade vault_relationships
-- =============================================================================

ALTER TABLE vault_relationships ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0;
ALTER TABLE vault_relationships ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT '';
ALTER TABLE vault_relationships ADD COLUMN IF NOT EXISTS valid_from TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE vault_relationships ADD COLUMN IF NOT EXISTS valid_to TIMESTAMPTZ;
ALTER TABLE vault_relationships ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

-- Backfill confidence from strength
UPDATE vault_relationships SET confidence = strength WHERE confidence = 1.0 AND strength != 1.0;

-- Partial unique index for semantic_similarity edge upsert
CREATE UNIQUE INDEX IF NOT EXISTS idx_vault_relationships_semantic_unique
    ON vault_relationships (source_entity_id, target_entity_id, relationship_type)
    WHERE relationship_type = 'semantic_similarity' AND valid_to IS NULL;

-- =============================================================================
-- 2c. New table: vault_aliases
-- =============================================================================

CREATE TABLE IF NOT EXISTS vault_aliases (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alias_type   TEXT NOT NULL,
    alias_value  TEXT NOT NULL,
    canonical_id UUID NOT NULL REFERENCES vault_entities(id) ON DELETE CASCADE,
    confidence   DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    source       TEXT NOT NULL DEFAULT '',
    reviewed     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(alias_type, alias_value)
);

CREATE INDEX IF NOT EXISTS idx_vault_aliases_canonical ON vault_aliases (canonical_id);
CREATE INDEX IF NOT EXISTS idx_vault_aliases_pending ON vault_aliases (id) WHERE reviewed = FALSE AND confidence < 0.9;

-- =============================================================================
-- 2d. New table: vault_provenance
-- =============================================================================

CREATE TABLE IF NOT EXISTS vault_provenance (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_id              UUID NOT NULL,
    target_type            TEXT NOT NULL,
    source_system          TEXT NOT NULL DEFAULT '',
    source_ref             TEXT NOT NULL DEFAULT '',
    source_idempotency_key TEXT UNIQUE,
    snippet                TEXT NOT NULL DEFAULT '',
    agent_id               UUID,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_vault_provenance_target ON vault_provenance (target_id, target_type);

-- =============================================================================
-- 2e. New table: vault_entity_embeddings
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS vault_entity_embeddings (
    entity_id    UUID PRIMARY KEY REFERENCES vault_entities(id) ON DELETE CASCADE,
    embedding    vector(384),
    model        TEXT NOT NULL DEFAULT '',
    text_hash    TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_vault_entity_embeddings_hnsw
    ON vault_entity_embeddings
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- =============================================================================
-- 2f. New tables: vault_semantic_clusters + vault_cluster_memberships
-- =============================================================================

CREATE TABLE IF NOT EXISTS vault_semantic_clusters (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label        TEXT NOT NULL DEFAULT '',
    centroid     vector(384),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    dissolved_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS vault_cluster_memberships (
    entity_id  UUID NOT NULL REFERENCES vault_entities(id) ON DELETE CASCADE,
    cluster_id UUID NOT NULL REFERENCES vault_semantic_clusters(id) ON DELETE CASCADE,
    distance   DOUBLE PRECISION NOT NULL DEFAULT 0,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at    TIMESTAMPTZ,
    PRIMARY KEY (entity_id, cluster_id, joined_at)
);

CREATE INDEX IF NOT EXISTS idx_vault_cluster_memberships_cluster ON vault_cluster_memberships (cluster_id) WHERE left_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_vault_cluster_memberships_entity ON vault_cluster_memberships (entity_id) WHERE left_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_vault_semantic_clusters_active ON vault_semantic_clusters (id) WHERE dissolved_at IS NULL;

-- =============================================================================
-- 2g. New table: vault_merge_proposals
-- =============================================================================

CREATE TABLE IF NOT EXISTS vault_merge_proposals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_a_id   UUID NOT NULL REFERENCES vault_entities(id) ON DELETE CASCADE,
    entity_b_id   UUID NOT NULL REFERENCES vault_entities(id) ON DELETE CASCADE,
    similarity    DOUBLE PRECISION NOT NULL,
    proposal_type TEXT NOT NULL DEFAULT 'entity',
    cluster_a_id  UUID REFERENCES vault_semantic_clusters(id),
    cluster_b_id  UUID REFERENCES vault_semantic_clusters(id),
    status        TEXT NOT NULL DEFAULT 'pending',
    reviewed_by   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at   TIMESTAMPTZ,
    UNIQUE(entity_a_id, entity_b_id)
);

CREATE INDEX IF NOT EXISTS idx_vault_merge_proposals_pending ON vault_merge_proposals (status) WHERE status = 'pending';

-- =============================================================================
-- 2h. New tables: vault_tasks + vault_decisions
-- =============================================================================

CREATE TABLE IF NOT EXISTS vault_tasks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id  UUID REFERENCES vault_entities(id) ON DELETE SET NULL,
    title      TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'open',
    priority   INT NOT NULL DEFAULT 0,
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_vault_tasks_entity ON vault_tasks (entity_id);
CREATE INDEX IF NOT EXISTS idx_vault_tasks_status ON vault_tasks (status);

CREATE TABLE IF NOT EXISTS vault_decisions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id   UUID REFERENCES vault_entities(id) ON DELETE SET NULL,
    title       TEXT NOT NULL,
    outcome     TEXT NOT NULL DEFAULT '',
    rationale   TEXT NOT NULL DEFAULT '',
    metadata    JSONB NOT NULL DEFAULT '{}',
    decided_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_vault_decisions_entity ON vault_decisions (entity_id);

-- =============================================================================
-- 2i. Extend vault_access_action enum (if it exists)
-- =============================================================================

DO $$ BEGIN
    ALTER TYPE vault_access_action ADD VALUE IF NOT EXISTS 'identity.resolve';
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TYPE vault_access_action ADD VALUE IF NOT EXISTS 'identity.merge';
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TYPE vault_access_action ADD VALUE IF NOT EXISTS 'semantic.read';
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

COMMIT;
