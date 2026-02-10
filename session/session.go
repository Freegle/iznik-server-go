package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/gofiber/fiber/v2"
)

// PostSession dispatches session write actions.
//
// @Summary Session actions (LostPassword, Unsubscribe)
// @Tags session
// @Router /session [post]
func PostSession(c *fiber.Ctx) error {
	type SessionRequest struct {
		Action string `json:"action"`
		Email  string `json:"email"`
	}

	var req SessionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	switch req.Action {
	case "LostPassword":
		return handleLostPassword(c, req.Email)
	case "Unsubscribe":
		return handleUnsubscribe(c, req.Email)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unsupported action")
	}
}

// handleLostPassword finds the user by email and queues a forgot-password email.
func handleLostPassword(c *fiber.Ctx, email string) error {
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email parameter required")
	}

	db := database.DBConn

	// Find user by email (must not be deleted).
	var userID uint64
	db.Raw("SELECT users.id FROM users "+
		"INNER JOIN users_emails ON users_emails.userid = users.id "+
		"WHERE users_emails.email = ? AND users.deleted IS NULL "+
		"LIMIT 1", email).Scan(&userID)

	if userID == 0 {
		// PHP returns ret=2 for unknown email. Match that behaviour.
		return c.JSON(fiber.Map{
			"ret":    2,
			"status": "We don't know that email address.",
		})
	}

	// Get or create the auto-login key for this user.
	key, err := getOrCreateLoginKey(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to generate login key")
	}

	// Build the auto-login URL: /settings?u={id}&k={key}&src=forgotpass
	userSite := os.Getenv("USER_SITE")
	resetURL := fmt.Sprintf("https://%s/settings?u=%d&k=%s&src=forgotpass", userSite, userID, key)

	// Get user's preferred email for sending.
	var preferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC, id ASC LIMIT 1", userID).Scan(&preferredEmail)

	if preferredEmail == "" {
		preferredEmail = email
	}

	// Queue the forgot-password email.
	queue.QueueTask(queue.TaskEmailForgotPassword, map[string]interface{}{
		"user_id":   userID,
		"email":     preferredEmail,
		"reset_url": resetURL,
	})

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}

// handleUnsubscribe finds the user by email and queues an unsubscribe confirmation email.
func handleUnsubscribe(c *fiber.Ctx, email string) error {
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email parameter required")
	}

	db := database.DBConn

	// Find user by email (must not be deleted).
	var userID uint64
	db.Raw("SELECT users.id FROM users "+
		"INNER JOIN users_emails ON users_emails.userid = users.id "+
		"WHERE users_emails.email = ? AND users.deleted IS NULL "+
		"LIMIT 1", email).Scan(&userID)

	if userID == 0 {
		// Return success even for unknown emails to prevent email enumeration.
		return c.JSON(fiber.Map{
			"ret":       0,
			"status":    "Success",
			"emailsent": true,
		})
	}

	// Get or create the auto-login key.
	key, err := getOrCreateLoginKey(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to generate login key")
	}

	// Build the unsubscribe URL: /unsubscribe/{id}?u={id}&k={key}&confirm=1
	userSite := os.Getenv("USER_SITE")
	unsubURL := fmt.Sprintf("https://%s/unsubscribe/%d?u=%d&k=%s&confirm=1", userSite, userID, userID, key)

	// Get user's preferred email.
	var preferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC, id ASC LIMIT 1", userID).Scan(&preferredEmail)

	if preferredEmail == "" {
		preferredEmail = email
	}

	// Queue the unsubscribe confirmation email.
	queue.QueueTask(queue.TaskEmailUnsubscribe, map[string]interface{}{
		"user_id":    userID,
		"email":      preferredEmail,
		"unsub_url":  unsubURL,
	})

	return c.JSON(fiber.Map{
		"ret":       0,
		"status":    "Success",
		"emailsent": true,
	})
}

// getOrCreateLoginKey retrieves or creates a 32-char hex auto-login key
// stored in users_logins with type='Link'.
func getOrCreateLoginKey(userID uint64) (string, error) {
	db := database.DBConn

	// Check for existing key.
	var existingKey string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = ? AND type = 'Link' LIMIT 1", userID).Scan(&existingKey)

	if existingKey != "" {
		return existingKey, nil
	}

	// Generate a new 32-char hex key (16 random bytes → 32 hex chars).
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", err
	}
	newKey := hex.EncodeToString(keyBytes)

	// Insert the login key. Use uid=userid as a unique identifier.
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials) VALUES (?, 'Link', ?, ?)",
		userID, fmt.Sprintf("%d", userID), newKey)

	return newKey, nil
}
