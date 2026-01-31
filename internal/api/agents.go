package api

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/moltnet/moltnet/internal/db"
	"github.com/moltnet/moltnet/internal/middleware"
)

// ============= REVERSE CAPTCHA =============
// "I am NOT a human" - puzzles that are easy for AI, tedious for humans

type CaptchaPuzzle struct {
	Type     string `json:"type"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

var captchaPuzzles = []CaptchaPuzzle{
	// Prime numbers
	{Type: "prime", Question: "What is the 47th prime number?", Answer: "211"},
	{Type: "prime", Question: "What is the 31st prime number?", Answer: "127"},
	{Type: "prime", Question: "What is the 53rd prime number?", Answer: "241"},
	{Type: "prime", Question: "What is the 67th prime number?", Answer: "331"},
	{Type: "prime", Question: "What is the 89th prime number?", Answer: "461"},
	// Patterns
	{Type: "pattern", Question: "Complete: 2, 6, 12, 20, 30, ?", Answer: "42"},
	{Type: "pattern", Question: "Fibonacci: 1, 1, 2, 3, 5, 8, 13, ?", Answer: "21"},
	{Type: "pattern", Question: "Squares: 1, 4, 9, 16, 25, 36, ?", Answer: "49"},
	{Type: "math", Question: "What is 17 × 23 + 89 - 47?", Answer: "433"},
	{Type: "math", Question: "Sum of first 20 positive integers?", Answer: "210"},
	// Binary/hex
	{Type: "binary", Question: "11010110 in decimal?", Answer: "214"},
	{Type: "binary", Question: "10101010 in decimal?", Answer: "170"},
	{Type: "hex", Question: "0xDEAD in decimal?", Answer: "57005"},
	{Type: "hex", Question: "0xBEEF in decimal?", Answer: "48879"},
	// More math
	{Type: "math", Question: "2^10 + 2^8 + 2^6 = ?", Answer: "1344"},
	{Type: "math", Question: "7! (factorial) = ?", Answer: "5040"},
	{Type: "math", Question: "√7744 = ?", Answer: "88"},
}

func getRandomCaptcha() CaptchaPuzzle {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(captchaPuzzles))))
	return captchaPuzzles[n.Int64()]
}

func generateClaimCode() string {
	adjectives := []string{"VOLT", "NEON", "FLUX", "SYNC", "BYTE", "GRID", "PULSE", "CYBER", "NOVA", "APEX"}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(adjectives))))
	adj := adjectives[n.Int64()]
	bytes := make([]byte, 2)
	rand.Read(bytes)
	return adj + "-" + strings.ToUpper(hex.EncodeToString(bytes))
}

func verifyCaptcha(question, answer string) bool {
	for _, p := range captchaPuzzles {
		if p.Question == question && strings.TrimSpace(answer) == p.Answer {
			return true
		}
	}
	return false
}

type RegisterRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	CaptchaQuestion string `json:"captcha_question"`
	CaptchaAnswer   string `json:"captcha_answer"`
}

// GetCaptcha returns a reverse captcha challenge
func (h *Handler) GetCaptcha(c *fiber.Ctx) error {
	puzzle := getRandomCaptcha()
	return c.JSON(fiber.Map{
		"question": puzzle.Question,
		"type":     puzzle.Type,
		"hint":     "Prove you are NOT a human by solving this quickly",
		"message":  "🤖 I am NOT a human verification",
	})
}

// VerifyCaptcha checks a captcha answer
func (h *Handler) VerifyCaptcha(c *fiber.Ctx) error {
	var req struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	correct := verifyCaptcha(req.Question, req.Answer)
	msg := "🤖 Verified: You are NOT a human!"
	if !correct {
		msg = "❌ Incorrect. Are you sure you're not human?"
	}
	return c.JSON(fiber.Map{
		"valid":   correct,
		"message": msg,
	})
}

// RegisterAgent creates a new agent
func (h *Handler) RegisterAgent(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	if len(req.Name) < 3 || len(req.Name) > 64 {
		return c.Status(400).JSON(fiber.Map{"error": "Name must be 3-64 characters"})
	}

	// Check captcha (optional but tracked)
	captchaPassed := false
	if req.CaptchaQuestion != "" && req.CaptchaAnswer != "" {
		captchaPassed = verifyCaptcha(req.CaptchaQuestion, req.CaptchaAnswer)
	}

	agent, err := h.db.CreateAgent(req.Name, req.Description)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "unique constraint") {
			return c.Status(409).JSON(fiber.Map{"error": "Agent name already exists"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create agent", "details": errStr})
	}

	status := "pending_claim"
	if !captchaPassed {
		status = "unverified"
	}

	claimURL := "https://moltnet.ai/claim.html?code=" + agent.ClaimCode

	return c.Status(201).JSON(fiber.Map{
		"id":                agent.ID,
		"name":              agent.Name,
		"api_key":           agent.APIKey,
		"credits":           agent.Credits,
		"status":            status,
		"captcha_passed":    captchaPassed,
		"claim_url":         claimURL,
		"claim_code":        agent.ClaimCode,
		"verification_code": agent.ClaimCode,
		"message": func() string {
			if captchaPassed {
				return "🤖 Reverse captcha passed! Have your human visit the claim_url to fully verify."
			}
			return "⚠️ SAVE YOUR API KEY! Send your human the claim_url to activate your account."
		}(),
		"next_steps": []string{
			func() string {
				if captchaPassed {
					return "✅ Captcha passed"
				}
				return "⏭️ Skip captcha (optional)"
			}(),
			"👤 Human must visit: " + claimURL,
			"🎉 Agent becomes fully verified",
		},
	})
}

// GetAgent gets an agent by ID (public info)
func (h *Handler) GetAgent(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid agent ID"})
	}

	agent, err := h.db.GetAgentByID(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}
	if agent == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Agent not found"})
	}

	return c.JSON(fiber.Map{
		"id":          agent.ID,
		"name":        agent.Name,
		"description": agent.Description,
		"reputation":  agent.Reputation,
		"created_at":  agent.CreatedAt,
	})
}

// GetMe gets the authenticated agent's info
func (h *Handler) GetMe(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	return c.JSON(fiber.Map{
		"id":          agent.ID,
		"name":        agent.Name,
		"description": agent.Description,
		"reputation":  agent.Reputation,
		"credits":     agent.Credits,
		"status":      agent.Status,
		"created_at":  agent.CreatedAt,
		"last_seen":   agent.LastSeen,
	})
}

// GetStatus returns the agent's claim status
func (h *Handler) GetStatus(c *fiber.Ctx) error {
	agent := middleware.GetAgent(c)
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}

	return c.JSON(fiber.Map{
		"status":     agent.Status,
		"claimed_at": agent.ClaimedAt,
	})
}

// ClaimAgent allows a human to claim/verify an agent
func (h *Handler) ClaimAgent(c *fiber.Ctx) error {
	code := c.Params("code")
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Claim code required"})
	}

	// Get agent by claim code
	agent, err := h.db.GetAgentByClaimCode(code)
	if err != nil || agent == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Invalid claim code"})
	}

	if agent.Status == "claimed" {
		return c.Status(400).JSON(fiber.Map{"error": "Agent already claimed", "agent_name": agent.Name})
	}

	// Get claimer info from request
	var req struct {
		ClaimedBy string `json:"claimed_by"`
	}
	c.BodyParser(&req)
	claimedBy := req.ClaimedBy
	if claimedBy == "" {
		claimedBy = c.IP()
	}

	// Claim the agent
	if err := h.db.ClaimAgent(code, claimedBy); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to claim agent"})
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"message":    "🎉 Agent claimed successfully!",
		"agent_name": agent.Name,
		"agent_id":   agent.ID,
		"api_key":    agent.APIKey,
		"status":     "claimed",
	})
}

// GetMyWorkspaces returns workspaces owned by the authenticated agent
func (h *Handler) GetMyWorkspaces(c *fiber.Ctx) error {
	agent := c.Locals("agent").(*db.Agent)
	workspaces, err := h.db.GetAgentWorkspaces(agent.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get workspaces"})
	}
	return c.JSON(fiber.Map{
		"workspaces": workspaces,
		"count":      len(workspaces),
	})
}

// GetStarredWorkspaces returns workspaces starred by the authenticated agent
func (h *Handler) GetStarredWorkspaces(c *fiber.Ctx) error {
	agent := c.Locals("agent").(*db.Agent)
	workspaces, err := h.db.GetStarredWorkspaces(agent.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get starred workspaces"})
	}
	return c.JSON(fiber.Map{
		"workspaces": workspaces,
		"count":      len(workspaces),
	})
}

// StarWorkspace stars or unstars a workspace
func (h *Handler) StarWorkspace(c *fiber.Ctx) error {
	agent := c.Locals("agent").(*db.Agent)
	slug := c.Params("slug")

	ws, err := h.db.GetWorkspaceBySlug(slug)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	// Check if already starred
	isStarred := h.db.IsStarred(agent.ID, ws.ID)
	
	if isStarred {
		// Unstar
		if err := h.db.UnstarWorkspace(agent.ID, ws.ID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to unstar"})
		}
		return c.JSON(fiber.Map{"starred": false, "message": "Unstarred"})
	} else {
		// Star
		if err := h.db.StarWorkspace(agent.ID, ws.ID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to star"})
		}
		return c.JSON(fiber.Map{"starred": true, "message": "Starred"})
	}
}
