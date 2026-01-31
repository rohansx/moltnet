package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/moltnet/moltnet/internal/api"
	"github.com/moltnet/moltnet/internal/db"
	"github.com/moltnet/moltnet/internal/git"
)

func main() {
	// Config
	port := os.Getenv("PORT")
	if port == "" {
		port = "3456"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://moltnet:moltnet@localhost/moltnet?sslmode=disable"
	}

	reposPath := os.Getenv("REPOS_PATH")
	if reposPath == "" {
		reposPath = "./repos"
	}

	// Initialize database
	database, err := db.New(dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Initialize git service
	gitService := git.New(reposPath)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Moltnet API v1",
		ServerHeader: "Moltnet",
		ErrorHandler: api.ErrorHandler,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${status} ${method} ${path} ${latency}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-API-Key",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE, OPTIONS",
	}))

	// API routes
	apiHandler := api.New(database, gitService)
	apiHandler.SetupRoutes(app)

	// Clean URLs for pages (serve .html without extension)
	pages := []string{"docs", "explore", "claim", "feed", "workspace", "dashboard"}
	for _, page := range pages {
		p := page // capture for closure
		app.Get("/"+p, func(c *fiber.Ctx) error {
			return c.SendFile("./web/" + p + ".html")
		})
	}

	// Static files
	app.Static("/", "./web", fiber.Static{
		Index: "index.html",
	})

	// Start server
	log.Printf("🚀 Moltnet starting on :%s", port)
	log.Printf("📁 Repos path: %s", reposPath)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
