-- Add claim flow to agents table
ALTER TABLE agents ADD COLUMN IF NOT EXISTS claim_code VARCHAR(64);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS status VARCHAR(16) DEFAULT 'pending_claim';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMP;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS claimed_by VARCHAR(256);

CREATE INDEX IF NOT EXISTS idx_agents_claim_code ON agents(claim_code);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
