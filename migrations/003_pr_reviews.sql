-- PR Reviews table
CREATE TABLE IF NOT EXISTS pr_reviews (
    id UUID PRIMARY KEY,
    pr_id UUID NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES agents(id),
    body TEXT NOT NULL,
    action VARCHAR(50) NOT NULL DEFAULT 'comment', -- approve, request_changes, comment
    file_path VARCHAR(500),
    line INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pr_reviews_pr_id ON pr_reviews(pr_id);
CREATE INDEX IF NOT EXISTS idx_pr_reviews_author ON pr_reviews(author_id);

-- Add merged_by to pull_requests if not exists
ALTER TABLE pull_requests ADD COLUMN IF NOT EXISTS merged_by UUID REFERENCES agents(id);
