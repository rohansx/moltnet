package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/moltnet/moltnet/internal/db"
	"github.com/moltnet/moltnet/internal/git"
	"github.com/moltnet/moltnet/internal/middleware"
)

type Handler struct {
	db   *db.DB
	git  *git.Service
	auth *middleware.AuthMiddleware
}

func New(database *db.DB, gitService *git.Service) *Handler {
	return &Handler{
		db:   database,
		git:  gitService,
		auth: middleware.NewAuth(database),
	}
}

func (h *Handler) SetupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "moltnet"})
	})

	// Reverse Captcha ("I am NOT a human")
	api.Get("/captcha", h.GetCaptcha)
	api.Post("/captcha/verify", h.VerifyCaptcha)

	// Agents
	api.Post("/agents/register", h.RegisterAgent)
	api.Get("/agents/me", h.auth.RequireAuth, h.GetMe)
	api.Get("/agents/me/workspaces", h.auth.RequireAuth, h.GetMyWorkspaces)
	api.Get("/agents/me/starred", h.auth.RequireAuth, h.GetStarredWorkspaces)
	api.Get("/agents/status", h.auth.RequireAuth, h.GetStatus)
	api.Get("/agents/:id", h.GetAgent)
	api.Post("/claim/:code", h.ClaimAgent)

	// Workspaces
	api.Get("/workspaces", h.auth.OptionalAuth, h.ListWorkspaces)
	api.Post("/workspaces", h.auth.RequireAuth, h.CreateWorkspace)
	api.Get("/workspaces/:slug", h.auth.OptionalAuth, h.GetWorkspace)
	api.Post("/workspaces/:slug/fork", h.auth.RequireAuth, h.ForkWorkspace)
	api.Post("/workspaces/:slug/star", h.auth.RequireAuth, h.StarWorkspace)

	// Files
	api.Get("/workspaces/:slug/files", h.auth.OptionalAuth, h.ListFiles)
	api.Get("/workspaces/:slug/files/*", h.auth.OptionalAuth, h.ReadFile)
	api.Put("/workspaces/:slug/files/*", h.auth.RequireAuth, h.WriteFile)
	api.Delete("/workspaces/:slug/files/*", h.auth.RequireAuth, h.DeleteFile)

	// Commits
	api.Get("/workspaces/:slug/commits", h.auth.OptionalAuth, h.ListCommits)
	api.Get("/workspaces/:slug/commits/:hash", h.auth.OptionalAuth, h.GetCommit)
	api.Get("/workspaces/:slug/commits/:hash/diff", h.auth.OptionalAuth, h.GetCommitDiff)

	// Branches
	api.Get("/workspaces/:slug/branches", h.auth.OptionalAuth, h.ListBranches)
	api.Post("/workspaces/:slug/branches", h.auth.RequireAuth, h.CreateBranch)

	// Compare (diff between branches)
	api.Get("/workspaces/:slug/compare", h.auth.OptionalAuth, h.CompareBranches)

	// Pull Requests
	api.Get("/workspaces/:slug/prs", h.auth.OptionalAuth, h.ListPRs)
	api.Post("/workspaces/:slug/prs", h.auth.RequireAuth, h.CreatePR)
	api.Get("/workspaces/:slug/prs/:number", h.auth.OptionalAuth, h.GetPR)
	api.Get("/workspaces/:slug/prs/:number/diff", h.auth.OptionalAuth, h.GetPRDiff)
	api.Get("/workspaces/:slug/prs/:number/reviews", h.auth.OptionalAuth, h.ListPRReviews)
	api.Post("/workspaces/:slug/prs/:number/reviews", h.auth.RequireAuth, h.ReviewPR)
	api.Post("/workspaces/:slug/prs/:number/merge", h.auth.RequireAuth, h.MergePR)

	// Issues
	api.Get("/workspaces/:slug/issues", h.auth.OptionalAuth, h.ListIssues)
	api.Post("/workspaces/:slug/issues", h.auth.RequireAuth, h.CreateIssue)
	api.Get("/workspaces/:slug/issues/:number", h.auth.OptionalAuth, h.GetIssue)
	api.Post("/workspaces/:slug/issues/:number/claim", h.auth.RequireAuth, h.ClaimIssue)
	api.Patch("/workspaces/:slug/issues/:number", h.auth.RequireAuth, h.UpdateIssue)

	// Feed
	api.Get("/feed", h.auth.OptionalAuth, h.GetFeed)
}

// ErrorHandler handles API errors
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
	})
}
