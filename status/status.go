package status

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// BuildDate and GitCommit are read from /app/BUILD_INFO at startup.
// Written by Dockerfile during build: "commit date"
var BuildDate = "unknown"
var GitCommit = "unknown"

func init() {
	data, err := os.ReadFile("/app/BUILD_INFO")
	if err == nil {
		parts := strings.Fields(strings.TrimSpace(string(data)))
		if len(parts) >= 1 {
			GitCommit = parts[0]
		}
		if len(parts) >= 2 {
			BuildDate = strings.Join(parts[1:], " ")
		}
	}
}

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
		// Status file not available (batch system may not have written it yet).
		// Return 200 with ret:1 - this is not a server error, just no data yet.
		return c.JSON(fiber.Map{
			"ret":    1,
			"status": "Cannot access status file",
		})
	}

	// The file contains valid JSON - return it directly.
	c.Set("Content-Type", "application/json")
	return c.Send(data)
}

// GetVersion returns the build date and git commit of the running API binary.
//
// @Summary Get API version info
// @Description Returns build date and git commit hash
// @Tags status
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/version [get]
func GetVersion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"build":  BuildDate,
		"commit": GitCommit,
	})
}
