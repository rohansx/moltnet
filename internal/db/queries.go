package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

func generateAPIKey() string {
	b := make([]byte, 28)
	rand.Read(b)
	return "mlt_" + hex.EncodeToString(b)[:56]
}

func generateClaimCode() string {
	words := []string{"volt", "spark", "node", "link", "byte", "flux", "core", "sync", "data", "wave"}
	b := make([]byte, 2)
	rand.Read(b)
	word := words[int(b[0])%len(words)]
	code := fmt.Sprintf("%s-%X%X", word, b[0], b[1])
	return strings.ToUpper(code)
}

func slugify(name string) string {
	s := strings.ToLower(name)
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "-")
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// CreateAgent creates a new agent
func (d *DB) CreateAgent(name, description string) (*Agent, error) {
	agent := &Agent{
		ID:          uuid.New(),
		Name:        name,
		APIKey:      generateAPIKey(),
		Description: description,
		Reputation:  0,
		Credits:     100,
		ClaimCode:   generateClaimCode(),
		Status:      "pending_claim",
		CreatedAt:   time.Now(),
	}

	_, err := d.conn.Exec(`
		INSERT INTO agents (id, name, api_key, description, reputation, credits, claim_code, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, agent.ID, agent.Name, agent.APIKey, agent.Description, agent.Reputation, agent.Credits, agent.ClaimCode, agent.Status, agent.CreatedAt)

	if err != nil {
		return nil, err
	}
	return agent, nil
}

// GetAgentByAPIKey looks up agent by API key
func (d *DB) GetAgentByAPIKey(apiKey string) (*Agent, error) {
	agent := &Agent{}
	err := d.conn.QueryRow(`
		SELECT id, name, api_key, description, reputation, credits, claim_code, status, claimed_at, created_at, last_seen
		FROM agents WHERE api_key = $1
	`, apiKey).Scan(&agent.ID, &agent.Name, &agent.APIKey, &agent.Description,
		&agent.Reputation, &agent.Credits, &agent.ClaimCode, &agent.Status, &agent.ClaimedAt, &agent.CreatedAt, &agent.LastSeen)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Update last seen
	d.conn.Exec("UPDATE agents SET last_seen = $1 WHERE id = $2", time.Now(), agent.ID)
	return agent, nil
}

// GetAgentByID gets agent by ID
func (d *DB) GetAgentByID(id uuid.UUID) (*Agent, error) {
	agent := &Agent{}
	err := d.conn.QueryRow(`
		SELECT id, name, description, reputation, credits, status, created_at, last_seen
		FROM agents WHERE id = $1
	`, id).Scan(&agent.ID, &agent.Name, &agent.Description,
		&agent.Reputation, &agent.Credits, &agent.Status, &agent.CreatedAt, &agent.LastSeen)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return agent, err
}

// GetAgentByClaimCode gets agent by claim code
func (d *DB) GetAgentByClaimCode(code string) (*Agent, error) {
	agent := &Agent{}
	err := d.conn.QueryRow(`
		SELECT id, name, description, status, claim_code, api_key, created_at
		FROM agents WHERE claim_code = $1
	`, code).Scan(&agent.ID, &agent.Name, &agent.Description, &agent.Status, &agent.ClaimCode, &agent.APIKey, &agent.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return agent, err
}

// ClaimAgent marks an agent as claimed
func (d *DB) ClaimAgent(code, claimedBy string) error {
	_, err := d.conn.Exec(`
		UPDATE agents SET status = 'claimed', claimed_at = $1, claimed_by = $2 
		WHERE claim_code = $3 AND status = 'pending_claim'
	`, time.Now(), claimedBy, code)
	return err
}

// CreateWorkspace creates a new workspace
func (d *DB) CreateWorkspace(ownerID uuid.UUID, name, description, visibility, repoPath string, forkedFrom *uuid.UUID) (*Workspace, error) {
	ws := &Workspace{
		ID:            uuid.New(),
		Name:          name,
		Slug:          slugify(name),
		Description:   description,
		OwnerID:       ownerID,
		ForkedFrom:    forkedFrom,
		Visibility:    visibility,
		RepoPath:      repoPath,
		DefaultBranch: "main",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Ensure unique slug
	var exists bool
	baseSlug := ws.Slug
	counter := 1
	for {
		d.conn.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE slug = $1)", ws.Slug).Scan(&exists)
		if !exists {
			break
		}
		ws.Slug = fmt.Sprintf("%s-%d", baseSlug, counter)
		counter++
	}

	_, err := d.conn.Exec(`
		INSERT INTO workspaces (id, name, slug, description, owner_id, forked_from, visibility, repo_path, default_branch, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, ws.ID, ws.Name, ws.Slug, ws.Description, ws.OwnerID, ws.ForkedFrom, ws.Visibility, ws.RepoPath, ws.DefaultBranch, ws.CreatedAt, ws.UpdatedAt)

	if err != nil {
		return nil, err
	}

	// Increment fork count on parent
	if forkedFrom != nil {
		d.conn.Exec("UPDATE workspaces SET forks_count = forks_count + 1 WHERE id = $1", *forkedFrom)
	}

	return ws, nil
}

// GetWorkspaceBySlug gets workspace by slug
func (d *DB) GetWorkspaceBySlug(slug string) (*Workspace, error) {
	ws := &Workspace{}
	err := d.conn.QueryRow(`
		SELECT w.id, w.name, w.slug, w.description, w.owner_id, a.name, w.forked_from, 
		       w.visibility, w.repo_path, w.default_branch, w.stars, w.forks_count, w.created_at, w.updated_at
		FROM workspaces w
		JOIN agents a ON w.owner_id = a.id
		WHERE w.slug = $1
	`, slug).Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.OwnerID, &ws.OwnerName,
		&ws.ForkedFrom, &ws.Visibility, &ws.RepoPath, &ws.DefaultBranch, &ws.Stars, &ws.ForksCount, &ws.CreatedAt, &ws.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ws, err
}

// GetWorkspaceByID gets workspace by ID
func (d *DB) GetWorkspaceByID(id uuid.UUID) (*Workspace, error) {
	ws := &Workspace{}
	err := d.conn.QueryRow(`
		SELECT w.id, w.name, w.slug, w.description, w.owner_id, a.name, w.forked_from, 
		       w.visibility, w.repo_path, w.default_branch, w.stars, w.forks_count, w.created_at, w.updated_at
		FROM workspaces w
		JOIN agents a ON w.owner_id = a.id
		WHERE w.id = $1
	`, id).Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.OwnerID, &ws.OwnerName,
		&ws.ForkedFrom, &ws.Visibility, &ws.RepoPath, &ws.DefaultBranch, &ws.Stars, &ws.ForksCount, &ws.CreatedAt, &ws.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ws, err
}

// ListWorkspaces lists public workspaces with optional search
func (d *DB) ListWorkspaces(query string, limit int) ([]Workspace, error) {
	var rows *sql.Rows
	var err error

	if query != "" {
		rows, err = d.conn.Query(`
			SELECT w.id, w.name, w.slug, w.description, w.owner_id, a.name, w.forked_from,
			       w.visibility, w.repo_path, w.default_branch, w.stars, w.forks_count, w.created_at, w.updated_at
			FROM workspaces w
			JOIN agents a ON w.owner_id = a.id
			WHERE w.visibility = 'public' AND (w.name ILIKE $1 OR w.description ILIKE $1)
			ORDER BY w.stars DESC, w.created_at DESC
			LIMIT $2
		`, "%"+query+"%", limit)
	} else {
		rows, err = d.conn.Query(`
			SELECT w.id, w.name, w.slug, w.description, w.owner_id, a.name, w.forked_from,
			       w.visibility, w.repo_path, w.default_branch, w.stars, w.forks_count, w.created_at, w.updated_at
			FROM workspaces w
			JOIN agents a ON w.owner_id = a.id
			WHERE w.visibility = 'public'
			ORDER BY w.stars DESC, w.created_at DESC
			LIMIT $1
		`, limit)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		err := rows.Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.OwnerID, &ws.OwnerName,
			&ws.ForkedFrom, &ws.Visibility, &ws.RepoPath, &ws.DefaultBranch, &ws.Stars, &ws.ForksCount, &ws.CreatedAt, &ws.UpdatedAt)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}

// UpdateWorkspace updates workspace metadata
func (d *DB) UpdateWorkspace(id uuid.UUID, name, description string) error {
	_, err := d.conn.Exec(`
		UPDATE workspaces SET name = $1, description = $2, updated_at = $3 WHERE id = $4
	`, name, description, time.Now(), id)
	return err
}

// CreatePullRequest creates a new PR
func (d *DB) CreatePullRequest(workspaceID, authorID uuid.UUID, title, description, sourceBranch, targetBranch string) (*PullRequest, error) {
	// Get next PR number
	var maxNum sql.NullInt64
	d.conn.QueryRow("SELECT MAX(number) FROM pull_requests WHERE workspace_id = $1", workspaceID).Scan(&maxNum)
	number := 1
	if maxNum.Valid {
		number = int(maxNum.Int64) + 1
	}

	pr := &PullRequest{
		ID:           uuid.New(),
		WorkspaceID:  workspaceID,
		Number:       number,
		AuthorID:     authorID,
		Title:        title,
		Description:  description,
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		Status:       "open",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err := d.conn.Exec(`
		INSERT INTO pull_requests (id, workspace_id, number, author_id, title, description, source_branch, target_branch, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, pr.ID, pr.WorkspaceID, pr.Number, pr.AuthorID, pr.Title, pr.Description, pr.SourceBranch, pr.TargetBranch, pr.Status, pr.CreatedAt, pr.UpdatedAt)

	return pr, err
}

// GetPullRequest gets a PR by workspace and number
func (d *DB) GetPullRequest(workspaceID uuid.UUID, number int) (*PullRequest, error) {
	pr := &PullRequest{}
	err := d.conn.QueryRow(`
		SELECT pr.id, pr.workspace_id, pr.number, pr.author_id, a.name, pr.title, pr.description,
		       pr.source_branch, pr.target_branch, pr.status, pr.merged_at, pr.created_at, pr.updated_at
		FROM pull_requests pr
		JOIN agents a ON pr.author_id = a.id
		WHERE pr.workspace_id = $1 AND pr.number = $2
	`, workspaceID, number).Scan(&pr.ID, &pr.WorkspaceID, &pr.Number, &pr.AuthorID, &pr.AuthorName,
		&pr.Title, &pr.Description, &pr.SourceBranch, &pr.TargetBranch, &pr.Status, &pr.MergedAt, &pr.CreatedAt, &pr.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return pr, err
}

// ListPullRequests lists PRs for a workspace
func (d *DB) ListPullRequests(workspaceID uuid.UUID, status string) ([]PullRequest, error) {
	query := `
		SELECT pr.id, pr.workspace_id, pr.number, pr.author_id, a.name, pr.title, pr.description,
		       pr.source_branch, pr.target_branch, pr.status, pr.merged_at, pr.created_at, pr.updated_at
		FROM pull_requests pr
		JOIN agents a ON pr.author_id = a.id
		WHERE pr.workspace_id = $1
	`
	args := []interface{}{workspaceID}

	if status != "" {
		query += " AND pr.status = $2"
		args = append(args, status)
	}
	query += " ORDER BY pr.created_at DESC"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []PullRequest
	for rows.Next() {
		var pr PullRequest
		err := rows.Scan(&pr.ID, &pr.WorkspaceID, &pr.Number, &pr.AuthorID, &pr.AuthorName,
			&pr.Title, &pr.Description, &pr.SourceBranch, &pr.TargetBranch, &pr.Status, &pr.MergedAt, &pr.CreatedAt, &pr.UpdatedAt)
		if err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

// MergePullRequest marks a PR as merged
func (d *DB) MergePullRequest(id, mergedBy uuid.UUID) error {
	_, err := d.conn.Exec(`
		UPDATE pull_requests SET status = 'merged', merged_at = $1, merged_by = $2, updated_at = $1 WHERE id = $3
	`, time.Now(), mergedBy, id)
	return err
}

// CreateIssue creates a new issue
func (d *DB) CreateIssue(workspaceID, authorID uuid.UUID, title, description string, bountyCredits int) (*Issue, error) {
	// Get next issue number
	var maxNum sql.NullInt64
	d.conn.QueryRow("SELECT MAX(number) FROM issues WHERE workspace_id = $1", workspaceID).Scan(&maxNum)
	number := 1
	if maxNum.Valid {
		number = int(maxNum.Int64) + 1
	}

	issue := &Issue{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		Number:        number,
		AuthorID:      authorID,
		Title:         title,
		Description:   description,
		BountyCredits: bountyCredits,
		Status:        "open",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err := d.conn.Exec(`
		INSERT INTO issues (id, workspace_id, number, author_id, title, description, bounty_credits, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, issue.ID, issue.WorkspaceID, issue.Number, issue.AuthorID, issue.Title, issue.Description, issue.BountyCredits, issue.Status, issue.CreatedAt, issue.UpdatedAt)

	return issue, err
}

// GetIssue gets an issue by workspace and number
func (d *DB) GetIssue(workspaceID uuid.UUID, number int) (*Issue, error) {
	issue := &Issue{}
	err := d.conn.QueryRow(`
		SELECT i.id, i.workspace_id, i.number, i.author_id, a.name, i.assignee_id,
		       i.title, i.description, i.bounty_credits, i.status, i.closed_at, i.linked_pr, i.created_at, i.updated_at
		FROM issues i
		JOIN agents a ON i.author_id = a.id
		WHERE i.workspace_id = $1 AND i.number = $2
	`, workspaceID, number).Scan(&issue.ID, &issue.WorkspaceID, &issue.Number, &issue.AuthorID, &issue.AuthorName,
		&issue.AssigneeID, &issue.Title, &issue.Description, &issue.BountyCredits, &issue.Status, &issue.ClosedAt, &issue.LinkedPR, &issue.CreatedAt, &issue.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return issue, err
}

// ListIssues lists issues for a workspace
func (d *DB) ListIssues(workspaceID uuid.UUID, status string) ([]Issue, error) {
	query := `
		SELECT i.id, i.workspace_id, i.number, i.author_id, a.name, i.assignee_id,
		       i.title, i.description, i.bounty_credits, i.status, i.closed_at, i.linked_pr, i.created_at, i.updated_at
		FROM issues i
		JOIN agents a ON i.author_id = a.id
		WHERE i.workspace_id = $1
	`
	args := []interface{}{workspaceID}

	if status != "" {
		query += " AND i.status = $2"
		args = append(args, status)
	}
	query += " ORDER BY i.bounty_credits DESC, i.created_at DESC"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []Issue
	for rows.Next() {
		var issue Issue
		err := rows.Scan(&issue.ID, &issue.WorkspaceID, &issue.Number, &issue.AuthorID, &issue.AuthorName,
			&issue.AssigneeID, &issue.Title, &issue.Description, &issue.BountyCredits, &issue.Status, &issue.ClosedAt, &issue.LinkedPR, &issue.CreatedAt, &issue.UpdatedAt)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

// ClaimIssue assigns an issue to an agent
func (d *DB) ClaimIssue(issueID, agentID uuid.UUID) error {
	_, err := d.conn.Exec(`
		UPDATE issues SET assignee_id = $1, updated_at = $2 WHERE id = $3 AND assignee_id IS NULL
	`, agentID, time.Now(), issueID)
	return err
}

// CloseIssue closes an issue
func (d *DB) CloseIssue(issueID, closedBy uuid.UUID, linkedPR *uuid.UUID) error {
	_, err := d.conn.Exec(`
		UPDATE issues SET status = 'closed', closed_at = $1, closed_by = $2, linked_pr = $3, updated_at = $1 WHERE id = $4
	`, time.Now(), closedBy, linkedPR, issueID)
	return err
}

// LogActivity logs an activity
func (d *DB) LogActivity(agentID, workspaceID uuid.UUID, action, title string) error {
	_, err := d.conn.Exec(`
		INSERT INTO activities (id, agent_id, workspace_id, action, title, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.New(), agentID, workspaceID, action, title, time.Now())
	return err
}

// GetFeed gets recent activity
func (d *DB) GetFeed(limit int) ([]Activity, error) {
	rows, err := d.conn.Query(`
		SELECT act.id, act.agent_id, a.name, act.workspace_id, w.slug, w.name, act.action, act.title, act.created_at
		FROM activities act
		JOIN agents a ON act.agent_id = a.id
		JOIN workspaces w ON act.workspace_id = w.id
		ORDER BY act.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var act Activity
		err := rows.Scan(&act.ID, &act.AgentID, &act.AgentName, &act.WorkspaceID, &act.WorkspaceSlug, &act.WorkspaceName, &act.Action, &act.Title, &act.CreatedAt)
		if err != nil {
			return nil, err
		}
		activities = append(activities, act)
	}
	return activities, nil
}

// CreatePRReview creates a new PR review
func (d *DB) CreatePRReview(prID, authorID uuid.UUID, body, action, filePath string, line int) (*PRReview, error) {
	review := &PRReview{
		ID:        uuid.New(),
		PRID:      prID,
		AuthorID:  authorID,
		Body:      body,
		Action:    action,
		FilePath:  filePath,
		Line:      line,
		CreatedAt: time.Now(),
	}

	_, err := d.conn.Exec(`
		INSERT INTO pr_reviews (id, pr_id, author_id, body, action, file_path, line, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, review.ID, review.PRID, review.AuthorID, review.Body, review.Action, review.FilePath, review.Line, review.CreatedAt)

	if err != nil {
		return nil, err
	}
	return review, nil
}

// ListPRReviews lists reviews for a PR
func (d *DB) ListPRReviews(prID uuid.UUID) ([]PRReview, error) {
	rows, err := d.conn.Query(`
		SELECT r.id, r.pr_id, r.author_id, a.name, r.body, r.action, r.file_path, r.line, r.created_at
		FROM pr_reviews r
		JOIN agents a ON r.author_id = a.id
		WHERE r.pr_id = $1
		ORDER BY r.created_at ASC
	`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []PRReview
	for rows.Next() {
		var review PRReview
		err := rows.Scan(&review.ID, &review.PRID, &review.AuthorID, &review.AuthorName,
			&review.Body, &review.Action, &review.FilePath, &review.Line, &review.CreatedAt)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

// StarWorkspace stars a workspace for an agent
func (d *DB) StarWorkspace(agentID, workspaceID uuid.UUID) error {
	_, err := d.conn.Exec(`
		INSERT INTO stars (id, agent_id, workspace_id, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (agent_id, workspace_id) DO NOTHING
	`, uuid.New(), agentID, workspaceID, time.Now())
	if err != nil {
		return err
	}
	// Update star count
	_, err = d.conn.Exec(`UPDATE workspaces SET stars = stars + 1 WHERE id = $1`, workspaceID)
	return err
}

// UnstarWorkspace removes a star
func (d *DB) UnstarWorkspace(agentID, workspaceID uuid.UUID) error {
	result, err := d.conn.Exec(`DELETE FROM stars WHERE agent_id = $1 AND workspace_id = $2`, agentID, workspaceID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		_, err = d.conn.Exec(`UPDATE workspaces SET stars = GREATEST(stars - 1, 0) WHERE id = $1`, workspaceID)
	}
	return err
}

// IsStarred checks if an agent has starred a workspace
func (d *DB) IsStarred(agentID, workspaceID uuid.UUID) bool {
	var exists bool
	d.conn.QueryRow(`SELECT EXISTS(SELECT 1 FROM stars WHERE agent_id = $1 AND workspace_id = $2)`, agentID, workspaceID).Scan(&exists)
	return exists
}

// GetStarredWorkspaces gets workspaces starred by an agent
func (d *DB) GetStarredWorkspaces(agentID uuid.UUID) ([]Workspace, error) {
	rows, err := d.conn.Query(`
		SELECT w.id, w.owner_id, a.name, w.name, w.slug, w.description, w.visibility, w.stars, w.forks_count, w.forked_from, w.created_at, w.updated_at
		FROM workspaces w
		JOIN stars s ON w.id = s.workspace_id
		JOIN agents a ON w.owner_id = a.id
		WHERE s.agent_id = $1
		ORDER BY s.created_at DESC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		err := rows.Scan(&ws.ID, &ws.OwnerID, &ws.OwnerName, &ws.Name, &ws.Slug, &ws.Description, &ws.Visibility, &ws.Stars, &ws.ForksCount, &ws.ForkedFrom, &ws.CreatedAt, &ws.UpdatedAt)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}

// GetAgentWorkspaces gets workspaces owned by an agent
func (d *DB) GetAgentWorkspaces(agentID uuid.UUID) ([]Workspace, error) {
	rows, err := d.conn.Query(`
		SELECT w.id, w.owner_id, a.name, w.name, w.slug, w.description, w.visibility, w.stars, w.forks_count, w.forked_from, w.created_at, w.updated_at
		FROM workspaces w
		JOIN agents a ON w.owner_id = a.id
		WHERE w.owner_id = $1
		ORDER BY w.updated_at DESC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		err := rows.Scan(&ws.ID, &ws.OwnerID, &ws.OwnerName, &ws.Name, &ws.Slug, &ws.Description, &ws.Visibility, &ws.Stars, &ws.ForksCount, &ws.ForkedFrom, &ws.CreatedAt, &ws.UpdatedAt)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}
