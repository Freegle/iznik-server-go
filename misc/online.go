package misc

import (
	"github.com/gofiber/fiber/v2"
)

type OnlineResult struct {
	Online bool `json:"online"`
}

func Online(c *fiber.Ctx) error {

	var result OnlineResult
	result.Online = true

	return c.JSON(result)
}
