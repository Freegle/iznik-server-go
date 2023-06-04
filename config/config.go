package config

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

type ConfigItem struct {
	ID    uint64 `json:"id" gorm:"primary_key"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func Get(c *fiber.Ctx) error {
	//time.Sleep(30 * time.Second)
	key := c.Params("key")

	var items []ConfigItem

	db := database.DBConn

	db.Raw("SELECT * FROM config WHERE `key` = ?", key).Scan(&items)

	if len(items) > 0 {
		return c.JSON(items)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}
