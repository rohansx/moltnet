package api

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/moltnet/moltnet/internal/middleware"
)

type CreateIssueRequest struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	BountyCredits int    `json:"bounty_credits"`
}

type UpdateIssueRequest struct {
	Status   string `json:"status"`
	LinkedPR int    `json:"linked_pr"`
}

// ListIssues lists issues
func (h *Handler) ListIssues(c *fiber.Ctx) error {
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

	issues, err := h.db.ListIssues(ws.ID, status)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list issues"})
	}

	return c.JSON(fiber.Map{
		"issues": issues,
		"count":  len(issues),
	})
}

// CreateIssue creates a new issue
func (h *Handler) CreateIssue(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	var req CreateIssueRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Title required"})
	}

	// Check if agent has enough credits for bounty
	if req.BountyCredits > 0 && agent.Credits < req.BountyCredits {
		return c.Status(400).JSON(fiber.Map{
			"error":          "Insufficient credits for bounty",
			"credits":        agent.Credits,
			"bounty_requested": req.BountyCredits,
		})
	}

	issue, err := h.db.CreateIssue(ws.ID, agent.ID, req.Title, req.Description, req.BountyCredits)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create issue"})
	}

	// Log activity
	activityTitle := fmt.Sprintf("Opened issue #%d: %s", issue.Number, issue.Title)
	if req.BountyCredits > 0 {
		activityTitle += fmt.Sprintf(" [Bounty: %d credits]", req.BountyCredits)
	}
	h.db.LogActivity(agent.ID, ws.ID, "issue_open", activityTitle)

	return c.Status(201).JSON(issue)
}

// GetIssue gets an issue
func (h *Handler) GetIssue(c *fiber.Ctx) error {
	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid issue number"})
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

	issue, err := h.db.GetIssue(ws.ID, number)
	if err != nil || issue == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Issue not found"})
	}

	return c.JSON(issue)
}

// ClaimIssue claims an issue
func (h *Handler) ClaimIssue(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid issue number"})
	}

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	issue, err := h.db.GetIssue(ws.ID, number)
	if err != nil || issue == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Issue not found"})
	}

	if issue.Status != "open" {
		return c.Status(400).JSON(fiber.Map{"error": "Issue is not open"})
	}

	if issue.AssigneeID != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Issue already claimed"})
	}

	if err := h.db.ClaimIssue(issue.ID, agent.ID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to claim issue"})
	}

	h.db.LogActivity(agent.ID, ws.ID, "issue_claim", fmt.Sprintf("Claimed issue #%d: %s", issue.Number, issue.Title))

	return c.JSON(fiber.Map{
		"claimed":      true,
		"issue_number": issue.Number,
		"claimed_by":   agent.Name,
	})
}

// UpdateIssue updates an issue (close it)
func (h *Handler) UpdateIssue(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	numberStr := c.Params("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid issue number"})
	}

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	issue, err := h.db.GetIssue(ws.ID, number)
	if err != nil || issue == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Issue not found"})
	}

	// Only owner, author, or assignee can update
	if agent.ID != ws.OwnerID && agent.ID != issue.AuthorID && (issue.AssigneeID == nil || agent.ID != *issue.AssigneeID) {
		return c.Status(403).JSON(fiber.Map{"error": "Not authorized to update this issue"})
	}

	var req UpdateIssueRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Status == "closed" {
		var linkedPR *uuid.UUID
		if req.LinkedPR > 0 {
			pr, _ := h.db.GetPullRequest(ws.ID, req.LinkedPR)
			if pr != nil {
				linkedPR = &pr.ID
			}
		}

		if err := h.db.CloseIssue(issue.ID, agent.ID, linkedPR); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to close issue"})
		}

		// TODO: Transfer bounty credits to assignee
		h.db.LogActivity(agent.ID, ws.ID, "issue_close", fmt.Sprintf("Closed issue #%d: %s", issue.Number, issue.Title))

		return c.JSON(fiber.Map{
			"closed":       true,
			"issue_number": issue.Number,
			"closed_by":    agent.Name,
		})
	}

	return c.JSON(fiber.Map{"updated": true})
}

// GetFeed gets the activity feed
func (h *Handler) GetFeed(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}

	activities, err := h.db.GetFeed(limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get feed"})
	}

	return c.JSON(fiber.Map{
		"activities": activities,
		"count":      len(activities),
	})
}
