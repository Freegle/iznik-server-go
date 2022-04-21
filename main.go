package main

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/message"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/jinzhu/gorm"
	"os"
)

func setupRoutes(app *fiber.App) {
	// TODO Can we avoid duplicating routes?
	api := app.Group("/api")
	api.Get("/group/:id", group.GetGroup)
	api.Get("/group/:id/message", group.GetGroupMessages)
	api.Get("/message/isochrones", message.Isochrones)
	api.Get("/message/:id", message.GetMessage)

	apiv2 := app.Group("/apiv2")
	apiv2.Get("/group/:id", group.GetGroup)
	api.Get("/group/:id/message", group.GetGroupMessages)
	apiv2.Get("/message/isochrones", message.Isochrones)
	apiv2.Get("/message/:id", message.GetMessage)
}

func initDatabase() {
	var err error
	mysqlCredentials := fmt.Sprintf(
		"%s:%s@%s(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local",
		os.Getenv("MYSQL_USER"),
		os.Getenv("MYSQL_PASSWORD"),
		os.Getenv("MYSQL_PROTOCOL"),
		os.Getenv("MYSQL_HOST"),
		os.Getenv("MYSQL_PORT"),
		os.Getenv("MYSQL_DBNAME"),
	)

	database.DBConn, err = gorm.Open("mysql", mysqlCredentials)

	if err != nil {
		panic("failed to connect database")
	}
}

func main() {
	app := fiber.New(fiber.Config{
		ReadBufferSize:  8192,
		WriteBufferSize: 8192,
	})
	app.Use(logger.New())
	initDatabase()
	setupRoutes(app)
	app.Listen(":8192")
}
