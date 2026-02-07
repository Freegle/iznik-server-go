package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// MarkSeenRequest is the request body for marking messages as seen.
type MarkSeenRequest struct {
	IDs []uint64 `json:"ids"`
}

// MarkSeen marks messages as seen (viewed) by the authenticated user.
// @Summary Mark messages as seen
// @Description Records that the authenticated user has viewed the specified messages.
// @Tags messages
// @Accept json
// @Produce json
// @Param ids body []int true "Array of message IDs to mark as seen"
// @Success 200 {object} map[string]interface{} "Success response"
// @Failure 400 {object} map[string]string "Invalid request or empty IDs"
// @Failure 401 {object} map[string]string "Not logged in"
// @Router /messages/markseen [post]
func MarkSeen(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req MarkSeenRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if len(req.IDs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Message IDs required")
	}

	db := database.DBConn

	// Insert a View record for each message. ON DUPLICATE KEY UPDATE
	// increments the count and updates the timestamp, matching the PHP
	// Message::like() behaviour.
	for _, msgID := range req.IDs {
		db.Exec("INSERT INTO messages_likes (msgid, userid, type) VALUES (?, ?, 'View') "+
			"ON DUPLICATE KEY UPDATE timestamp = NOW(), count = count + 1",
			msgID, myid)
	}

	return c.JSON(fiber.Map{
		"success": true,
	})
}
