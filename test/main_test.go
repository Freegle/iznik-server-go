package test

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

var app *fiber.App

func init() {
	app = fiber.New()
	app.Use(user.NewAuthMiddleware(user.Config{}))
	database.InitDatabase()
	router.SetupRoutes(app)
}

func getApp() *fiber.App {
	// We use this so that we only initialise fiber once.
	return app
}
