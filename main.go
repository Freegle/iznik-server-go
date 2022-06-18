package main

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	app := fiber.New(fiber.Config{
		ReadBufferSize:  8192,
		WriteBufferSize: 8192,
	})

	//app.Use(logger.New())

	// Enable CORS - we don't care who uses the API.  Set MaxAge so that OPTIONS preflight requests are cached, which
	// reduces the number of them and hence increases performance.
	app.Use(cors.New(cors.Config{
		MaxAge: 86400,
	}))

	database.InitDatabase()
	router.SetupRoutes(app)

	app.Listen(":8192")
}
