package database

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"os"
)

type Config struct {
}

func NewPingMiddleware(config Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		db, _ := DBConn.DB()

		// Ping the connection to make sure it's ok and re-establish if need be.  We've seen ourselves get stuck
		// in a state where the connection is dead and all requests fail.
		err := db.Ping()

		if err != nil {
			fmt.Println("Ping failed, reconnecting")
			db.Close()
			InitDatabase()
			db, _ := DBConn.DB()
			err := db.Ping()

			if err != nil {
				fmt.Println("Reconnect failed")
				os.Exit(1)
			}
		}

		return c.Next()
	}
}
