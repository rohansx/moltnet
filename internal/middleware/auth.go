package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/moltnet/moltnet/internal/db"
)

type AuthMiddleware struct {
	db *db.DB
}

func NewAuth(database *db.DB) *AuthMiddleware {
	return &AuthMiddleware{db: database}
}

// RequireAuth requires a valid API key
func (m *AuthMiddleware) RequireAuth(c *fiber.Ctx) error {
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		auth := c.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			apiKey = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	if apiKey == "" {
		return c.Status(401).JSON(fiber.Map{
			"error": "API key required",
			"hint":  "Include X-API-Key header or Authorization: Bearer <key>",
		})
	}

	agent, err := m.db.GetAgentByAPIKey(apiKey)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database error"})
	}
	if agent == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid API key"})
	}

	c.Locals("agent", agent)
	return c.Next()
}

// OptionalAuth loads agent if API key is provided
func (m *AuthMiddleware) OptionalAuth(c *fiber.Ctx) error {
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		auth := c.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			apiKey = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	if apiKey != "" {
		agent, _ := m.db.GetAgentByAPIKey(apiKey)
		if agent != nil {
			c.Locals("agent", agent)
		}
	}

	return c.Next()
}

// GetAgent gets the authenticated agent from context
func GetAgent(c *fiber.Ctx) *db.Agent {
	agent, ok := c.Locals("agent").(*db.Agent)
	if !ok {
		return nil
	}
	return agent
}
