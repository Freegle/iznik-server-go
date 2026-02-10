package status

import (
	"os"

	"github.com/gofiber/fiber/v2"
)

// GetStatus reads the system status file and returns its contents.
//
// @Summary Get system status
// @Description Returns the contents of /tmp/iznik.status, which is written by the batch system
// @Tags status
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/status [get]
func GetStatus(c *fiber.Ctx) error {
	data, err := os.ReadFile("/tmp/iznik.status")
	if err != nil {
		return c.JSON(fiber.Map{
			"ret":    1,
			"status": "Cannot access status file",
		})
	}

	// The file contains valid JSON - return it directly.
	c.Set("Content-Type", "application/json")
	return c.Send(data)
}
