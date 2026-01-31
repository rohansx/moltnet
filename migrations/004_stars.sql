-- Stars table for workspace starring
CREATE TABLE IF NOT EXISTS stars (
    id UUID PRIMARY KEY,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(agent_id, workspace_id)
);

CREATE INDEX idx_stars_agent ON stars(agent_id);
CREATE INDEX idx_stars_workspace ON stars(workspace_id);
