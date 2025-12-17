package ai

import (
	"os"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// GetKey returns the Anthropic API key for Support/Admin users only.
// This key is used by the AI Assistant feature in ModTools.
// @Router /ai/key [get]
// @Summary Get Anthropic API key
// @Description Returns the Anthropic API key for AI Assistant (Support/Admin only)
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]string "API key"
// @Failure 401 {object} fiber.Error "Authentication required"
// @Failure 403 {object} fiber.Error "Support or Admin role required"
// @Failure 500 {object} fiber.Error "API key not configured"
func GetKey(c *fiber.Ctx) error {
	// Extract JWT information including session ID
	userID, sessionID, _ := user.GetJWTFromRequest(c)
	if userID == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Authentication required")
	}

	db := database.DBConn

	// Validate that the user and session are still valid in the database
	var userInfo struct {
		ID         uint64 `json:"id"`
		Systemrole string `json:"systemrole"`
	}

	db.Raw("SELECT users.id, users.systemrole FROM sessions INNER JOIN users ON users.id = sessions.userid WHERE sessions.id = ? AND users.id = ? LIMIT 1", sessionID, userID).Scan(&userInfo)

	if userInfo.ID == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid session")
	}

	if userInfo.Systemrole != "Support" && userInfo.Systemrole != "Admin" {
		return fiber.NewError(fiber.StatusForbidden, "Support or Admin role required")
	}

	// Get the API key from environment variable
	apiKey := os.Getenv("CLAUDE_SUPPORT_QUERY_KEY")
	if apiKey == "" {
		return fiber.NewError(fiber.StatusInternalServerError, "API key not configured")
	}

	return c.JSON(fiber.Map{
		"key": apiKey,
	})
}
