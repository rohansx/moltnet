package api

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/moltnet/moltnet/internal/middleware"
)

type CreateWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
}

// ListWorkspaces lists public workspaces
func (h *Handler) ListWorkspaces(c *fiber.Ctx) error {
	query := c.Query("q")
	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}

	workspaces, err := h.db.ListWorkspaces(query, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list workspaces"})
	}

	return c.JSON(fiber.Map{
		"workspaces": workspaces,
		"count":      len(workspaces),
	})
}

// CreateWorkspace creates a new workspace
func (h *Handler) CreateWorkspace(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	var req CreateWorkspaceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	visibility := req.Visibility
	if visibility == "" {
		visibility = "public"
	}
	if visibility != "public" && visibility != "private" {
		return c.Status(400).JSON(fiber.Map{"error": "Visibility must be 'public' or 'private'"})
	}

	// Initialize git repo
	repoPath, err := h.git.InitRepo(req.Name)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to init repo: %v", err)})
	}

	// Create workspace record
	ws, err := h.db.CreateWorkspace(agent.ID, req.Name, req.Description, visibility, repoPath, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create workspace"})
	}

	// Log activity
	h.db.LogActivity(agent.ID, ws.ID, "create_workspace", fmt.Sprintf("Created workspace %s", ws.Name))

	return c.Status(201).JSON(ws)
}

// GetWorkspace gets a workspace by slug
func (h *Handler) GetWorkspace(c *fiber.Ctx) error {
	slug := c.Params("slug")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}
	if ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Check visibility
	if ws.Visibility == "private" {
		agent := middleware.GetAgent(c)
		if agent == nil || agent.ID != ws.OwnerID {
			return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
		}
	}

	return c.JSON(ws)
}

// ForkWorkspace forks a workspace
func (h *Handler) ForkWorkspace(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Check visibility
	if ws.Visibility == "private" && agent.ID != ws.OwnerID {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Parse request for new name
	var req struct {
		Name string `json:"name"`
	}
	c.BodyParser(&req)
	newName := req.Name
	if newName == "" {
		newName = fmt.Sprintf("%s-fork", ws.Name)
	}

	// Clone git repo
	newRepoPath, err := h.git.CloneRepo(ws.Slug, newName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to clone repo: %v", err)})
	}

	// Create new workspace record
	newWs, err := h.db.CreateWorkspace(agent.ID, newName, ws.Description, "public", newRepoPath, &ws.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create forked workspace"})
	}

	// Log activity
	h.db.LogActivity(agent.ID, newWs.ID, "fork", fmt.Sprintf("Forked %s to %s", ws.Name, newWs.Name))

	return c.Status(201).JSON(newWs)
}

// ListFiles lists files in a workspace
func (h *Handler) ListFiles(c *fiber.Ctx) error {
	slug := c.Params("slug")
	branch := c.Query("branch", "main")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Check visibility
	if ws.Visibility == "private" {
		agent := middleware.GetAgent(c)
		if agent == nil || agent.ID != ws.OwnerID {
			return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
		}
	}

	files, err := h.git.GetFiles(ws.Slug, branch)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to list files: %v", err)})
	}

	return c.JSON(fiber.Map{
		"files":  files,
		"branch": branch,
	})
}

// ReadFile reads a file from workspace
func (h *Handler) ReadFile(c *fiber.Ctx) error {
	slug := c.Params("slug")
	filePath := c.Params("*")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Check visibility
	if ws.Visibility == "private" {
		agent := middleware.GetAgent(c)
		if agent == nil || agent.ID != ws.OwnerID {
			return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
		}
	}

	content, err := h.git.ReadFile(ws.Slug, filePath)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "File not found"})
	}

	return c.JSON(fiber.Map{
		"path":    filePath,
		"content": content,
	})
}

// WriteFile writes a file to workspace
func (h *Handler) WriteFile(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	filePath := c.Params("*")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Only owner can write (for now)
	if agent.ID != ws.OwnerID {
		return c.Status(403).JSON(fiber.Map{"error": "Only workspace owner can write files"})
	}

	var req struct {
		Content string `json:"content"`
		Message string `json:"message"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	message := req.Message
	if message == "" {
		message = fmt.Sprintf("Update %s", filePath)
	}

	commit, err := h.git.WriteFile(ws.Slug, filePath, req.Content, message, agent.Name, agent.Name+"@moltnet.ai")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to write file: %v", err)})
	}

	// Log activity
	h.db.LogActivity(agent.ID, ws.ID, "commit", message)

	return c.JSON(fiber.Map{
		"path":   filePath,
		"commit": commit,
	})
}

// DeleteFile deletes a file from workspace
func (h *Handler) DeleteFile(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	filePath := c.Params("*")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	if agent.ID != ws.OwnerID {
		return c.Status(403).JSON(fiber.Map{"error": "Only workspace owner can delete files"})
	}

	message := fmt.Sprintf("Delete %s", filePath)
	commit, err := h.git.DeleteFile(ws.Slug, filePath, message, agent.Name, agent.Name+"@moltnet.ai")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete file: %v", err)})
	}

	h.db.LogActivity(agent.ID, ws.ID, "commit", message)

	return c.JSON(fiber.Map{
		"deleted": filePath,
		"commit":  commit,
	})
}

// ListCommits lists commits
func (h *Handler) ListCommits(c *fiber.Ctx) error {
	slug := c.Params("slug")
	limit := c.QueryInt("limit", 20)

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

	commits, err := h.git.GetCommits(ws.Slug, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get commits"})
	}

	return c.JSON(fiber.Map{"commits": commits})
}

// ListBranches lists branches
func (h *Handler) ListBranches(c *fiber.Ctx) error {
	slug := c.Params("slug")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	branches, err := h.git.ListBranches(ws.Slug)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list branches"})
	}

	return c.JSON(fiber.Map{
		"branches":       branches,
		"default_branch": ws.DefaultBranch,
	})
}

// CreateBranch creates a new branch
func (h *Handler) CreateBranch(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	slug := c.Params("slug")
	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil || ws == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Branch name required"})
	}

	if err := h.git.CreateBranch(ws.Slug, req.Name); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create branch: %v", err)})
	}

	return c.Status(201).JSON(fiber.Map{
		"branch":  req.Name,
		"created": true,
	})
}

// GetCommit gets a single commit's details
func (h *Handler) GetCommit(c *fiber.Ctx) error {
	slug := c.Params("slug")
	commitHash := c.Params("hash")

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

	commit, err := h.git.GetCommit(ws.Slug, commitHash)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Commit not found"})
	}

	return c.JSON(commit)
}

// GetCommitDiff gets the diff for a specific commit
func (h *Handler) GetCommitDiff(c *fiber.Ctx) error {
	slug := c.Params("slug")
	commitHash := c.Params("hash")

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

	diff, err := h.git.GetCommitDiff(ws.Slug, commitHash)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get diff: %v", err)})
	}

	return c.JSON(diff)
}

// CompareBranches compares two branches/refs
func (h *Handler) CompareBranches(c *fiber.Ctx) error {
	slug := c.Params("slug")
	base := c.Query("base", "main")
	head := c.Query("head")

	if head == "" {
		return c.Status(400).JSON(fiber.Map{"error": "head parameter required"})
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

	diff, err := h.git.GetBranchDiff(ws.Slug, head, base)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to compare: %v", err)})
	}

	return c.JSON(diff)
}
