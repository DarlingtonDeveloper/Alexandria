-- Alexandria Access Control Migration
-- Adds proper ownership and access control for secrets and knowledge

-- People
CREATE TABLE vault_people (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    identifier TEXT UNIQUE NOT NULL, -- e.g. phone number, email
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TRIGGER vault_people_updated
    BEFORE UPDATE ON vault_people
    FOR EACH ROW EXECUTE FUNCTION vault_update_timestamp();

-- Devices  
CREATE TABLE vault_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    device_type TEXT NOT NULL, -- phone, server, laptop, agent
    owner_id UUID REFERENCES vault_people(id),
    identifier TEXT UNIQUE NOT NULL, -- hostname, device-id, agent-name
    metadata JSONB DEFAULT '{}',
    last_seen TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TRIGGER vault_devices_updated
    BEFORE UPDATE ON vault_devices
    FOR EACH ROW EXECUTE FUNCTION vault_update_timestamp();

-- Access grants (for secrets AND knowledge)
CREATE TABLE vault_access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type TEXT NOT NULL, -- 'secret' or 'knowledge'
    resource_id TEXT NOT NULL, -- secret name or knowledge UUID
    subject_type TEXT NOT NULL, -- 'person', 'device', 'agent'
    subject_id TEXT NOT NULL, -- person UUID, device UUID, or agent name
    permission TEXT NOT NULL DEFAULT 'read', -- 'read', 'write', 'admin'
    granted_by TEXT, -- who created the grant
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(resource_type, resource_id, subject_type, subject_id)
);

-- Indexes for access grants
CREATE INDEX vault_access_grants_resource_idx ON vault_access_grants(resource_type, resource_id);
CREATE INDEX vault_access_grants_subject_idx ON vault_access_grants(subject_type, subject_id);

-- Add owner fields to secrets
ALTER TABLE vault_secrets ADD COLUMN IF NOT EXISTS owner_type TEXT DEFAULT 'agent';
ALTER TABLE vault_secrets ADD COLUMN IF NOT EXISTS owner_id TEXT;

-- Add agent_id field to secrets for backward compatibility (if not exists)
ALTER TABLE vault_secrets ADD COLUMN IF NOT EXISTS agent_id TEXT;

-- Migrate existing: set owner_id = agent_id where not null
UPDATE vault_secrets SET owner_id = agent_id WHERE agent_id IS NOT NULL AND owner_id IS NULL;

-- Indexes for secrets ownership
CREATE INDEX vault_secrets_owner_idx ON vault_secrets(owner_type, owner_id);
