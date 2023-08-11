package misc

import (
	"github.com/gofiber/fiber/v2"
)

func Online(c *fiber.Ctx) error {
	return c.JSON(true)
}
