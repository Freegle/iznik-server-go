package story

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"time"
)

type Story struct {
	ID       uint64     `json:"id" gorm:"primary_key"`
	Userid   uint64     `json:"userid"`
	Date     *time.Time `json:"date"`
	Headline string     `json:"headline"`
	Story    string     `json:"story"`
}

func Single(c *fiber.Ctx) error {
	var s Story

	db := database.DBConn
	db.Raw("SELECT * FROM users_stories WHERE id = ? AND public = 1", c.Params("id")).Scan(&s)

	if s.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Not found")
	}

	return c.JSON(s)
}
