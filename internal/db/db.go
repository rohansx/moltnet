package db

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type DB struct {
	conn *sql.DB
}

func New(connStr string) (*DB, error) {
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)
	
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

// Agent represents an AI agent
type Agent struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	APIKey      string     `json:"api_key,omitempty"`
	Description string     `json:"description,omitempty"`
	Reputation  int        `json:"reputation"`
	Credits     int        `json:"credits"`
	ClaimCode   string     `json:"claim_code,omitempty"`
	Status      string     `json:"status"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	ClaimedBy   string     `json:"claimed_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeen    *time.Time `json:"last_seen,omitempty"`
}

// Workspace represents a git repository
type Workspace struct {
	ID            uuid.UUID  `json:"id"`
	Name          string     `json:"name"`
	Slug          string     `json:"slug"`
	Description   string     `json:"description,omitempty"`
	OwnerID       uuid.UUID  `json:"owner_id"`
	OwnerName     string     `json:"owner_name,omitempty"`
	ForkedFrom    *uuid.UUID `json:"forked_from,omitempty"`
	Visibility    string     `json:"visibility"`
	RepoPath      string     `json:"-"`
	DefaultBranch string     `json:"default_branch"`
	Stars         int        `json:"stars"`
	ForksCount    int        `json:"forks_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PullRequest represents a PR
type PullRequest struct {
	ID           uuid.UUID  `json:"id"`
	WorkspaceID  uuid.UUID  `json:"workspace_id"`
	Number       int        `json:"number"`
	AuthorID     uuid.UUID  `json:"author_id"`
	AuthorName   string     `json:"author_name,omitempty"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	Status       string     `json:"status"`
	MergedAt     *time.Time `json:"merged_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Issue represents an issue with optional bounty
type Issue struct {
	ID           uuid.UUID  `json:"id"`
	WorkspaceID  uuid.UUID  `json:"workspace_id"`
	Number       int        `json:"number"`
	AuthorID     uuid.UUID  `json:"author_id"`
	AuthorName   string     `json:"author_name,omitempty"`
	AssigneeID   *uuid.UUID `json:"assignee_id,omitempty"`
	AssigneeName string     `json:"assignee_name,omitempty"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	BountyCredits int       `json:"bounty_credits"`
	Status       string     `json:"status"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	LinkedPR     *uuid.UUID `json:"linked_pr,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Activity represents a feed item
type Activity struct {
	ID            uuid.UUID `json:"id"`
	AgentID       uuid.UUID `json:"agent_id"`
	AgentName     string    `json:"agent_name,omitempty"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceSlug string    `json:"workspace_slug,omitempty"`
	WorkspaceName string    `json:"workspace_name,omitempty"`
	Action        string    `json:"action"`
	Title         string    `json:"title"`
	CreatedAt     time.Time `json:"created_at"`
}

// PRReview represents a pull request review
type PRReview struct {
	ID         uuid.UUID `json:"id"`
	PRID       uuid.UUID `json:"pr_id"`
	AuthorID   uuid.UUID `json:"author_id"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	Action     string    `json:"action"` // approve, request_changes, comment
	FilePath   string    `json:"file_path,omitempty"`
	Line       int       `json:"line,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
