package main

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/message"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/jinzhu/gorm"
	"os"
)

func setupRoutes(app *fiber.App) {
	app.Get("/api/group/:id", group.GetGroup)
	app.Get("/api/message/:id", message.GetMessage)
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
	app := fiber.New()
	initDatabase()
	setupRoutes(app)
	app.Listen(":8192")
}
