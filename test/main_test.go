package test

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"os"
)

var app *fiber.App

func init() {
	// Set environment variables needed for tests
	os.Setenv("LOVEJUNK_PARTNER_KEY", "testkey123")

	app = fiber.New()
	app.Use(user.NewAuthMiddleware(user.Config{}))
	database.InitDatabase()

	// Set up swagger routes BEFORE other API routes (same as main.go)
	// Handle swagger redirect - redirect exact /swagger path to /swagger/index.html
	app.Get("/swagger", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html", 302)
	})

	// Serve swagger static files from swagger directory
	// Use absolute path to ensure it works regardless of where tests are run from
	swaggerPath := "/app/swagger"
	if _, err := os.Stat(swaggerPath); os.IsNotExist(err) {
		// Fallback to relative path if absolute doesn't exist (for local development)
		swaggerPath = "../swagger"
	}
	app.Static("/swagger", swaggerPath, fiber.Static{
		Index: "index.html",
	})

	// Set up all other API routes
	router.SetupRoutes(app)

	// Set up comprehensive test environment
	if err := SetupTestEnvironment(); err != nil {
		panic("Failed to setup test environment: " + err.Error())
	}
}

func getApp() *fiber.App {
	// We use this so that we only initialise fiber once.
	return app
}
