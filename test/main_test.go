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
