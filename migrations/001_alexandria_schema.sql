-- Alexandria (Vault) Schema Migration
-- All tables use vault_ prefix to coexist with other services in the shared Supabase project.

-- Enable pgvector if not already enabled
CREATE EXTENSION IF NOT EXISTS vector;

-- ============================================
-- Knowledge Store
-- ============================================

CREATE TYPE vault_knowledge_category AS ENUM (
  'discovery', 'lesson', 'preference', 'fact', 'event', 'decision', 'relationship'
);

CREATE TYPE vault_knowledge_scope AS ENUM (
  'public', 'private', 'shared'
);

CREATE TYPE vault_relevance_decay AS ENUM (
  'none', 'slow', 'fast', 'ephemeral'
);

CREATE TABLE vault_knowledge (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  content       TEXT NOT NULL,
  summary       TEXT,
  source_agent  TEXT NOT NULL,
  category      vault_knowledge_category NOT NULL DEFAULT 'discovery',
  scope         vault_knowledge_scope NOT NULL DEFAULT 'public',
  shared_with   TEXT[] DEFAULT '{}',
  tags          TEXT[] DEFAULT '{}',
  embedding     vector(1536),
  metadata      JSONB DEFAULT '{}',
  source_event_id TEXT,
  confidence    FLOAT DEFAULT 0.8 CHECK (confidence >= 0 AND confidence <= 1),
  relevance_decay vault_relevance_decay NOT NULL DEFAULT 'slow',
  expires_at    TIMESTAMPTZ,
  superseded_by UUID REFERENCES vault_knowledge(id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at    TIMESTAMPTZ
);

-- Indexes
CREATE INDEX vault_knowledge_source_agent_idx ON vault_knowledge(source_agent);
CREATE INDEX vault_knowledge_category_idx ON vault_knowledge(category);
CREATE INDEX vault_knowledge_scope_idx ON vault_knowledge(scope);
CREATE INDEX vault_knowledge_tags_idx ON vault_knowledge USING GIN(tags);
CREATE INDEX vault_knowledge_created_at_idx ON vault_knowledge(created_at DESC);
CREATE INDEX vault_knowledge_embedding_idx ON vault_knowledge
  USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Updated_at trigger
CREATE OR REPLACE FUNCTION vault_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER vault_knowledge_updated
  BEFORE UPDATE ON vault_knowledge
  FOR EACH ROW EXECUTE FUNCTION vault_update_timestamp();

-- ============================================
-- Knowledge Graph
-- ============================================

CREATE TYPE vault_entity_type AS ENUM (
  'person', 'agent', 'service', 'project', 'concept', 'credential'
);

CREATE TABLE vault_entities (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL,
  entity_type vault_entity_type NOT NULL,
  metadata    JSONB DEFAULT '{}',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(name, entity_type)
);

CREATE TRIGGER vault_entities_updated
  BEFORE UPDATE ON vault_entities
  FOR EACH ROW EXECUTE FUNCTION vault_update_timestamp();

CREATE TABLE vault_relationships (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_entity_id  UUID NOT NULL REFERENCES vault_entities(id) ON DELETE CASCADE,
  target_entity_id  UUID NOT NULL REFERENCES vault_entities(id) ON DELETE CASCADE,
  relationship_type TEXT NOT NULL,
  strength          FLOAT DEFAULT 1.0 CHECK (strength >= 0 AND strength <= 1),
  knowledge_id      UUID REFERENCES vault_knowledge(id),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_entity_id, target_entity_id, relationship_type)
);

CREATE INDEX vault_relationships_source_idx ON vault_relationships(source_entity_id);
CREATE INDEX vault_relationships_target_idx ON vault_relationships(target_entity_id);
CREATE INDEX vault_relationships_type_idx ON vault_relationships(relationship_type);

-- ============================================
-- Secret Management
-- ============================================

CREATE TABLE vault_secrets (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name                    TEXT NOT NULL UNIQUE,
  encrypted_value         TEXT NOT NULL,
  description             TEXT,
  scope                   TEXT[] DEFAULT '{}',
  rotation_interval_days  INT,
  last_rotated_at         TIMESTAMPTZ,
  expires_at              TIMESTAMPTZ,
  created_by              TEXT NOT NULL,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER vault_secrets_updated
  BEFORE UPDATE ON vault_secrets
  FOR EACH ROW EXECUTE FUNCTION vault_update_timestamp();

CREATE TABLE vault_secret_history (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  secret_id       UUID NOT NULL REFERENCES vault_secrets(id) ON DELETE CASCADE,
  encrypted_value TEXT NOT NULL,
  rotated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  rotated_by      TEXT NOT NULL
);

-- ============================================
-- Access Log (Audit Trail)
-- ============================================

CREATE TYPE vault_access_action AS ENUM (
  'knowledge.read', 'knowledge.write', 'knowledge.search', 'knowledge.delete',
  'secret.read', 'secret.write', 'secret.delete', 'secret.rotate',
  'briefing.generate',
  'graph.read', 'graph.write'
);

CREATE TABLE vault_access_log (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  action      vault_access_action NOT NULL,
  agent_id    TEXT NOT NULL,
  resource_id TEXT,
  ip_address  TEXT,
  success     BOOLEAN NOT NULL DEFAULT TRUE,
  metadata    JSONB DEFAULT '{}',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX vault_access_log_agent_idx ON vault_access_log(agent_id);
CREATE INDEX vault_access_log_action_idx ON vault_access_log(action);
CREATE INDEX vault_access_log_created_at_idx ON vault_access_log(created_at DESC);

-- ============================================
-- Semantic Search Helper Function
-- ============================================

CREATE OR REPLACE FUNCTION vault_semantic_search(
  query_embedding vector(1536),
  match_count INT DEFAULT 10,
  filter_scope vault_knowledge_scope DEFAULT NULL,
  filter_categories vault_knowledge_category[] DEFAULT NULL,
  filter_agent TEXT DEFAULT NULL,
  min_similarity FLOAT DEFAULT 0.5
)
RETURNS TABLE (
  id UUID,
  content TEXT,
  summary TEXT,
  source_agent TEXT,
  category vault_knowledge_category,
  tags TEXT[],
  similarity FLOAT,
  relevance_decay vault_relevance_decay,
  created_at TIMESTAMPTZ
) AS $$
BEGIN
  RETURN QUERY
  SELECT
    k.id,
    k.content,
    k.summary,
    k.source_agent,
    k.category,
    k.tags,
    (1 - (k.embedding <=> query_embedding))::FLOAT AS similarity,
    k.relevance_decay,
    k.created_at
  FROM vault_knowledge k
  WHERE k.deleted_at IS NULL
    AND (k.expires_at IS NULL OR k.expires_at > NOW())
    AND (k.superseded_by IS NULL)
    AND (filter_scope IS NULL OR k.scope = filter_scope)
    AND (filter_categories IS NULL OR k.category = ANY(filter_categories))
    AND (filter_agent IS NULL OR k.source_agent = filter_agent
         OR k.scope = 'public'
         OR (k.scope = 'shared' AND filter_agent = ANY(k.shared_with)))
    AND (1 - (k.embedding <=> query_embedding)) >= min_similarity
  ORDER BY k.embedding <=> query_embedding
  LIMIT match_count;
END;
$$ LANGUAGE plpgsql;
