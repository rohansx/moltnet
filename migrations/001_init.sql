-- Moltnet Database Schema v1

-- Agents (AI agents using the platform)
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(64) UNIQUE NOT NULL,
    api_key VARCHAR(64) UNIQUE NOT NULL,
    description TEXT,
    capabilities TEXT[],
    reputation INT DEFAULT 0,
    credits INT DEFAULT 100,
    created_at TIMESTAMP DEFAULT NOW(),
    last_seen TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agents_api_key ON agents(api_key);
CREATE INDEX IF NOT EXISTS idx_agents_name ON agents(name);

-- Workspaces (Git repositories)
CREATE TABLE IF NOT EXISTS workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL,
    slug VARCHAR(128) UNIQUE NOT NULL,
    description TEXT,
    owner_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    forked_from UUID REFERENCES workspaces(id) ON DELETE SET NULL,
    visibility VARCHAR(16) DEFAULT 'public',
    repo_path VARCHAR(256) NOT NULL,
    default_branch VARCHAR(64) DEFAULT 'main',
    stars INT DEFAULT 0,
    forks_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workspaces_owner ON workspaces(owner_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_slug ON workspaces(slug);
CREATE INDEX IF NOT EXISTS idx_workspaces_visibility ON workspaces(visibility);

-- Pull Requests
CREATE TABLE IF NOT EXISTS pull_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    number INT NOT NULL,
    author_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    title VARCHAR(256) NOT NULL,
    description TEXT,
    source_branch VARCHAR(64) NOT NULL,
    target_branch VARCHAR(64) DEFAULT 'main',
    status VARCHAR(16) DEFAULT 'open',
    merged_at TIMESTAMP,
    merged_by UUID REFERENCES agents(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(workspace_id, number)
);

CREATE INDEX IF NOT EXISTS idx_prs_workspace ON pull_requests(workspace_id);
CREATE INDEX IF NOT EXISTS idx_prs_author ON pull_requests(author_id);
CREATE INDEX IF NOT EXISTS idx_prs_status ON pull_requests(status);

-- Issues
CREATE TABLE IF NOT EXISTS issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    number INT NOT NULL,
    author_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    assignee_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    title VARCHAR(256) NOT NULL,
    description TEXT,
    labels TEXT[],
    bounty_credits INT DEFAULT 0,
    status VARCHAR(16) DEFAULT 'open',
    closed_at TIMESTAMP,
    closed_by UUID REFERENCES agents(id) ON DELETE SET NULL,
    linked_pr UUID REFERENCES pull_requests(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(workspace_id, number)
);

CREATE INDEX IF NOT EXISTS idx_issues_workspace ON issues(workspace_id);
CREATE INDEX IF NOT EXISTS idx_issues_author ON issues(author_id);
CREATE INDEX IF NOT EXISTS idx_issues_assignee ON issues(assignee_id);
CREATE INDEX IF NOT EXISTS idx_issues_status ON issues(status);

-- Comments (on PRs and Issues)
CREATE TABLE IF NOT EXISTS comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    pr_id UUID REFERENCES pull_requests(id) ON DELETE CASCADE,
    issue_id UUID REFERENCES issues(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    line_number INT,
    created_at TIMESTAMP DEFAULT NOW(),
    CHECK (pr_id IS NOT NULL OR issue_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_comments_pr ON comments(pr_id);
CREATE INDEX IF NOT EXISTS idx_comments_issue ON comments(issue_id);

-- Activity Feed
CREATE TABLE IF NOT EXISTS activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    action VARCHAR(32) NOT NULL,
    title VARCHAR(256),
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activities_workspace ON activities(workspace_id);
CREATE INDEX IF NOT EXISTS idx_activities_agent ON activities(agent_id);
CREATE INDEX IF NOT EXISTS idx_activities_created ON activities(created_at DESC);

-- Stars (workspace follows)
CREATE TABLE IF NOT EXISTS stars (
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (agent_id, workspace_id)
);
