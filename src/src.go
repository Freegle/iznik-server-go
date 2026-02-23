package src

import (
	"context"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type SourceRequest struct {
	Src string `json:"src" validate:"required,max=255"`
}

// RecordSource records the source parameter from URLs for tracking purposes
// @Summary Record traffic source
// @Description Records where a user came from (marketing campaigns, referrals, etc)
// @Tags tracking
// @Accept json
// @Produce json
// @Param source body SourceRequest true "Source tracking data"
// @Success 204 "No Content"
// @Failure 400 {object} fiber.Map "Bad Request"
// @Router /src [post]
func RecordSource(c *fiber.Ctx) error {
	var req SourceRequest

	// Parse request body
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request format",
		})
	}

	// Validate required field
	if req.Src == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Source parameter is required",
		})
	}

	// Get user ID if logged in (may be 0)
	userID, _, _ := user.GetJWTFromRequest(c)

	// Get session ID from header if available
	sessionID := c.Get("X-Session-ID", "")

	// Record in database synchronously for serverless compatibility
	// In AWS Lambda/Netlify Functions, goroutines that outlive the handler
	// may be terminated when the function execution ends
	_ = recordSource(req.Src, userID, sessionID)
	// Errors are intentionally ignored - we always return 204 for backward compatibility
	// with v1 behavior (FD doesn't check the response)

	// Return success
	return c.SendStatus(204)
}

func recordSource(src string, userID uint64, sessionID string) error {
	ctx := context.Background()
	db := database.DBConn

	// Insert into logs_src table
	// Using Exec since we don't need the result
	result := db.WithContext(ctx).Exec(`
		INSERT INTO logs_src (src, userid, session)
		VALUES (?, ?, ?)
	`, src, userID, sessionID)

	// Note: Errors are logged but not propagated - handler always returns 204
	if result.Error != nil {
		return result.Error
	}

	// If user is logged in, update their source field if not already set
	if userID > 0 {
		result = db.WithContext(ctx).Exec(`
			UPDATE users
			SET source = ?
			WHERE id = ? AND source IS NULL
		`, src, userID)

		// Note: UPDATE errors are also ignored by handler
		if result.Error != nil {
			return result.Error
		}
	}

	return nil
}