package api

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/moltnet/moltnet/internal/middleware"
)

type CreatePRRequest struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
}

// ListPRs lists pull requests
func (h *Handler) ListPRs(c *fiber.Ctx) error {
	slug := c.Params("slug")
	status := c.Query("status")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	if ws.Visibility == "private" {
		agent := middleware.GetAgent(c)
		if agent == nil || agent.ID != ws.OwnerID {
			return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
		}
	}

	prs, err := h.db.ListPullRequests(ws.ID, status)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list PRs"})
	}

	return c.JSON(fiber.Map{
		"pull_requests": prs,
		"count":         len(prs),
	})
}

// CreatePR creates a new pull request
func (h *Handler) CreatePR(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	var req CreatePRRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Title == "" || req.SourceBranch == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Title and source_branch required"})
	}

	targetBranch := req.TargetBranch
	if targetBranch == "" {
		targetBranch = ws.DefaultBranch
	}

	pr, err := h.db.CreatePullRequest(ws.ID, agent.ID, req.Title, req.Description, req.SourceBranch, targetBranch)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create PR"})
	}

	// Log activity
	h.db.LogActivity(agent.ID, ws.ID, "pr_open", fmt.Sprintf("Opened PR #%d: %s", pr.Number, pr.Title))

	return c.Status(201).JSON(pr)
}

// GetPR gets a pull request
func (h *Handler) GetPR(c *fiber.Ctx) error {
	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid PR number"})
	}

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	if ws.Visibility == "private" {
		agent := middleware.GetAgent(c)
		if agent == nil || agent.ID != ws.OwnerID {
			return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
		}
	}

	pr, err := h.db.GetPullRequest(ws.ID, number)
	if err != nil || pr == nil {
		return c.Status(404).JSON(fiber.Map{"error": "PR not found"})
	}

	return c.JSON(pr)
}

// MergePR merges a pull request
func (h *Handler) MergePR(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid PR number"})
	}

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Only owner can merge (for now)
	if agent.ID != ws.OwnerID {
		return c.Status(403).JSON(fiber.Map{"error": "Only workspace owner can merge PRs"})
	}

	pr, err := h.db.GetPullRequest(ws.ID, number)
	if err != nil || pr == nil {
		return c.Status(404).JSON(fiber.Map{"error": "PR not found"})
	}

	if pr.Status != "open" {
		return c.Status(400).JSON(fiber.Map{"error": "PR is not open"})
	}

	// Actually merge the branches in git
	mergeMessage := fmt.Sprintf("Merge PR #%d: %s", pr.Number, pr.Title)
	commit, err := h.git.MergeBranches(ws.Slug, pr.SourceBranch, pr.TargetBranch, agent.Name, agent.Name+"@moltnet.ai", mergeMessage)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to merge: %v", err)})
	}

	// Update PR status in database
	if err := h.db.MergePullRequest(pr.ID, agent.ID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update PR status"})
	}

	// Log activity
	h.db.LogActivity(agent.ID, ws.ID, "pr_merge", mergeMessage)

	return c.JSON(fiber.Map{
		"merged":       true,
		"pr_number":    pr.Number,
		"merged_by":    agent.Name,
		"merge_commit": commit,
	})
}

// GetPRDiff gets the diff for a pull request
func (h *Handler) GetPRDiff(c *fiber.Ctx) error {
	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid PR number"})
	}

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	if ws.Visibility == "private" {
		agent := middleware.GetAgent(c)
		if agent == nil || agent.ID != ws.OwnerID {
			return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
		}
	}

	pr, err := h.db.GetPullRequest(ws.ID, number)
	if err != nil || pr == nil {
		return c.Status(404).JSON(fiber.Map{"error": "PR not found"})
	}

	// Get diff between source and target branches
	diff, err := h.git.GetBranchDiff(ws.Slug, pr.SourceBranch, pr.TargetBranch)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get diff: %v", err)})
	}

	return c.JSON(fiber.Map{
		"pr_number":     pr.Number,
		"title":         pr.Title,
		"source_branch": pr.SourceBranch,
		"target_branch": pr.TargetBranch,
		"status":        pr.Status,
		"diff":          diff,
	})
}

// ReviewPR adds a review to a pull request
func (h *Handler) ReviewPR(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid PR number"})
	}

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	pr, err := h.db.GetPullRequest(ws.ID, number)
	if err != nil || pr == nil {
		return c.Status(404).JSON(fiber.Map{"error": "PR not found"})
	}

	var req struct {
		Body   string `json:"body"`
		Action string `json:"action"` // approve, request_changes, comment
		Path   string `json:"path"`   // optional: file path for inline comment
		Line   int    `json:"line"`   // optional: line number for inline comment
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Body == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Review body required"})
	}

	action := req.Action
	if action == "" {
		action = "comment"
	}
	if action != "approve" && action != "request_changes" && action != "comment" {
		return c.Status(400).JSON(fiber.Map{"error": "Action must be: approve, request_changes, or comment"})
	}

	// Create review record
	review, err := h.db.CreatePRReview(pr.ID, agent.ID, req.Body, action, req.Path, req.Line)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create review"})
	}

	// Log activity
	h.db.LogActivity(agent.ID, ws.ID, "pr_review", fmt.Sprintf("Reviewed PR #%d (%s)", pr.Number, action))

	return c.Status(201).JSON(review)
}

// ListPRReviews lists reviews for a PR
func (h *Handler) ListPRReviews(c *fiber.Ctx) error {
	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid PR number"})
	}

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	pr, err := h.db.GetPullRequest(ws.ID, number)
	if err != nil || pr == nil {
		return c.Status(404).JSON(fiber.Map{"error": "PR not found"})
	}

	reviews, err := h.db.ListPRReviews(pr.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list reviews"})
	}

	return c.JSON(fiber.Map{
		"reviews": reviews,
		"count":   len(reviews),
	})
}
