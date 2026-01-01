// Package amp provides AMP email endpoints for dynamic email content.
//
// This package implements the AMP for Email specification, allowing
// dynamic content in emails (e.g., chat messages, job listings) and
// inline actions (e.g., replying to messages).
//
// Security Model:
// - Single HMAC-SHA256 token for both read and write operations
// - Token is reusable within expiry period (default 7 days)
// - User must also be a chat member (verified server-side)
package amp

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/gofiber/fiber/v2"
)

// AMPChatMessage extends ChatMessageQuery with AMP-specific display fields.
// These fields are populated by enriching the base chat messages with user info.
type AMPChatMessage struct {
	chat.ChatMessageQuery
	FromUser  string `json:"fromUser"`
	FromImage string `json:"fromImage"`
	IsNew     bool   `json:"isNew"`
}

// AMPChatResponse wraps the enriched chat messages with AMP-specific metadata.
type AMPChatResponse struct {
	Items    []AMPChatMessage `json:"items"`
	ChatID   uint64           `json:"chatId"`
	SinceID  uint64           `json:"sinceId,omitempty"`
	CanReply bool             `json:"canReply"`
}

// ReplyResponse is the response for AMP chat reply submissions.
type ReplyResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// getAMPSecret returns the AMP secret from environment.
func getAMPSecret() string {
	secret := os.Getenv("AMP_SECRET")
	if secret == "" {
		// Fallback for development - should be set in production
		secret = os.Getenv("FREEGLE_AMP_SECRET")
	}
	if secret == "" {
		log.Printf("[AMP] WARNING: No AMP secret configured (checked AMP_SECRET and FREEGLE_AMP_SECRET)")
	} else {
		log.Printf("[AMP] Secret configured, length=%d", len(secret))
	}
	return secret
}

// computeHMAC generates an HMAC-SHA256 signature.
func computeHMAC(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateToken validates HMAC-based tokens for AMP operations (read and write).
// Tokens are reusable within their expiry period.
// Returns (userID, resourceID, error). On failure, returns (0, 0, nil) for graceful fallback.
func ValidateToken(c *fiber.Ctx) (uint64, uint64, error) {
	token := c.Query("rt")  // Token
	uid := c.Query("uid")   // User ID
	exp := c.Query("exp")   // Expiry timestamp
	resID := c.Params("id") // Resource ID (chat ID, etc.)

	log.Printf("[AMP] ValidateToken: resID=%s, uid=%s, exp=%s, token_len=%d", resID, uid, exp, len(token))

	// Check all required params present
	if token == "" || uid == "" || exp == "" || resID == "" {
		log.Printf("[AMP] Missing params: token=%v, uid=%v, exp=%v, resID=%v", token != "", uid != "", exp != "", resID != "")
		return 0, 0, nil
	}

	// Check expiry
	expTime, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > expTime {
		log.Printf("[AMP] Token expired or invalid expiry: exp=%s, now=%d", exp, time.Now().Unix())
		return 0, 0, nil
	}

	// Validate HMAC: "amp" + user_id + resource_id + expiry
	secret := getAMPSecret()
	if secret == "" {
		log.Printf("[AMP] No secret configured, failing validation")
		return 0, 0, nil
	}

	message := "amp" + uid + resID + exp
	expectedMAC := computeHMAC(message, secret)

	log.Printf("[AMP] HMAC check: message=%s, expected_len=%d, provided_len=%d", message, len(expectedMAC), len(token))

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(token), []byte(expectedMAC)) != 1 {
		tokenPreview := token
		if len(token) > 16 {
			tokenPreview = token[:16]
		}
		log.Printf("[AMP] HMAC mismatch: expected=%s, got=%s...", expectedMAC[:16]+"...", tokenPreview)
		return 0, 0, nil
	}

	userID, _ := strconv.ParseUint(uid, 10, 64)
	resourceID, _ := strconv.ParseUint(resID, 10, 64)

	// Verify user still exists
	db := database.DBConn
	var exists bool
	db.Raw("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&exists)
	if !exists {
		log.Printf("[AMP] User %d does not exist", userID)
		return 0, 0, nil
	}

	log.Printf("[AMP] Token validated successfully: userID=%d, resourceID=%d", userID, resourceID)
	return userID, resourceID, nil
}

// AMPCORSMiddleware handles both v1 and v2 AMP CORS requirements.
// See: https://amp.dev/documentation/guides-and-tutorials/learn/cors-in-email
func AMPCORSMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get AMP sender header (v2 spec)
		ampSender := c.Get("AMP-Email-Sender")

		// Get Origin header (v1 spec)
		origin := c.Get("Origin")
		sourceOrigin := c.Query("__amp_source_origin")

		if ampSender != "" {
			// Version 2: Just validate and echo back
			if !isAllowedSender(ampSender) {
				return fiber.NewError(fiber.StatusForbidden, "Sender not allowed")
			}
			c.Set("AMP-Email-Allow-Sender", ampSender)
			c.Set("Access-Control-Expose-Headers", "AMP-Email-Allow-Sender")
		} else if origin != "" && sourceOrigin != "" {
			// Version 1: Full CORS headers
			if !isAllowedSender(sourceOrigin) {
				return fiber.NewError(fiber.StatusForbidden, "Sender not allowed")
			}
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Access-Control-Expose-Headers", "AMP-Access-Control-Allow-Source-Origin")
			c.Set("AMP-Access-Control-Allow-Source-Origin", sourceOrigin)
		}

		// Handle preflight OPTIONS request
		if c.Method() == "OPTIONS" {
			c.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Set("Access-Control-Allow-Headers", "Content-Type, AMP-Email-Sender")
			c.Set("Access-Control-Max-Age", "86400")
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}

// isAllowedSender checks if the email sender is from an allowed domain.
func isAllowedSender(email string) bool {
	allowedDomains := []string{
		"@ilovefreegle.org",
		"@users.ilovefreegle.org",
		"@mail.ilovefreegle.org",
		"@gmail.dev", // Google AMP Playground for testing
	}
	for _, domain := range allowedDomains {
		if strings.HasSuffix(strings.ToLower(email), domain) {
			return true
		}
	}
	return false
}

// userInfo holds display information for a user.
type userInfo struct {
	ID       uint64
	Name     string
	ImageURL string
}

// getUserInfo fetches display name and profile image for a user.
// This matches the logic used in chat/chatroom.go for consistency.
func getUserInfo(userID uint64) userInfo {
	db := database.DBConn
	var result struct {
		ID        uint64
		Fullname  *string
		Firstname *string
		Lastname  *string
		ImageID   *uint64
		ImageURL  *string
		Email     *string
	}

	// Query user info with image and preferred email for Gravatar fallback.
	// Email is in users_emails table, not users table.
	db.Raw(`
		SELECT u.id, u.fullname, u.firstname, u.lastname,
		       ui.id AS imageid, ui.url AS imageurl,
		       ue.email AS email
		FROM users u
		LEFT JOIN users_images ui ON ui.userid = u.id
		LEFT JOIN users_emails ue ON ue.userid = u.id
		WHERE u.id = ?
		ORDER BY ui.default DESC, ui.id ASC, ue.preferred DESC
		LIMIT 1
	`, userID).Scan(&result)

	// Build display name - same logic as chat/chatroom.go
	var name string
	if result.Fullname != nil && *result.Fullname != "" {
		name = *result.Fullname
	} else {
		firstName := ""
		lastName := ""
		if result.Firstname != nil {
			firstName = *result.Firstname
		}
		if result.Lastname != nil {
			lastName = *result.Lastname
		}
		name = strings.TrimSpace(firstName + " " + lastName)
	}
	if name == "" {
		name = "Freegler"
	}

	// Build profile image URL - match Laravel AmpEmail trait logic
	imageDomain := os.Getenv("IMAGE_DOMAIN")
	if imageDomain == "" {
		imageDomain = "images.ilovefreegle.org"
	}
	deliveryDomain := os.Getenv("DELIVERY_DOMAIN")
	if deliveryDomain == "" {
		deliveryDomain = "delivery.ilovefreegle.org"
	}

	var imageURL string
	if result.ImageURL != nil && *result.ImageURL != "" {
		// External URL exists, use it directly
		imageURL = *result.ImageURL
	} else if result.ImageID != nil {
		// Build thumbnail URL from image ID
		imageURL = "https://" + imageDomain + "/tuimg_" + strconv.FormatUint(*result.ImageID, 10) + ".jpg"
	} else if result.Email != nil && *result.Email != "" {
		// No custom image - use Gravatar based on email MD5
		emailHash := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(*result.Email))))
		gravatarURL := "https://www.gravatar.com/avatar/" + hex.EncodeToString(emailHash[:]) + "?s=200&d=identicon&r=g"
		// Route through delivery service for resizing
		imageURL = "https://" + deliveryDomain + "/?url=" + url.QueryEscape(gravatarURL) + "&w=40"
	} else {
		// Fallback to default profile
		imageURL = "https://" + imageDomain + "/defaultprofile.png"
	}

	return userInfo{
		ID:       userID,
		Name:     name,
		ImageURL: imageURL,
	}
}

// GetChatMessages returns the last 5 messages for AMP email "Earlier conversation" section.
// Uses the shared FetchChatMessages function to return messages in the same format as the regular chat API,
// then enriches them with user display information for the AMP template.
// Excludes the triggering message (passed via 'exclude' param) to avoid duplication.
// @Summary Get chat messages for AMP email
// @Tags AMP
// @Produce json
// @Param id path int true "Chat ID"
// @Param rt query string true "Read token (HMAC)"
// @Param uid query int true "User ID"
// @Param exp query int true "Token expiry timestamp"
// @Param exclude query int false "Message ID to exclude (the one shown statically)"
// @Param since query int false "Message ID - messages newer than this are considered NEW"
// @Success 200 {object} AMPChatResponse
// @Router /amp/chat/{id} [get]
func GetChatMessages(c *fiber.Ctx) error {
	userID, chatID, err := ValidateToken(c)
	if err != nil || userID == 0 {
		// Return graceful fallback response - empty items triggers fallback UI
		return c.JSON(AMPChatResponse{
			Items:    []AMPChatMessage{},
			CanReply: false,
		})
	}

	// Message ID to exclude (the one shown statically in the email)
	excludeID, _ := strconv.ParseUint(c.Query("exclude", "0"), 10, 64)
	// Messages newer than this ID can be marked as "NEW" by the client
	sinceID, _ := strconv.ParseUint(c.Query("since", "0"), 10, 64)

	db := database.DBConn

	// Verify user is member of this chat
	var memberUserID uint64
	db.Raw(`SELECT userid FROM chat_roster WHERE chatid = ? AND userid = ?`, chatID, userID).Scan(&memberUserID)

	if memberUserID == 0 {
		return c.JSON(AMPChatResponse{Items: []AMPChatMessage{}, CanReply: false})
	}

	// Use the shared function to fetch messages
	// Limit to 5, exclude the triggering message, newest first (for "earlier conversation")
	messages := chat.FetchChatMessages(chatID, userID, 5, excludeID, true)

	// Reverse to show oldest first (chronological order for display)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	// Cache user info to avoid repeated lookups
	userCache := make(map[uint64]userInfo)

	// Enrich messages with user display info
	ampMessages := make([]AMPChatMessage, len(messages))
	for i, msg := range messages {
		// Get user info (from cache or fetch)
		info, ok := userCache[msg.Userid]
		if !ok {
			info = getUserInfo(msg.Userid)
			userCache[msg.Userid] = info
		}

		ampMessages[i] = AMPChatMessage{
			ChatMessageQuery: msg,
			FromUser:         info.Name,
			FromImage:        info.ImageURL,
			IsNew:            sinceID > 0 && msg.ID > sinceID,
		}
	}

	return c.JSON(AMPChatResponse{
		Items:    ampMessages,
		ChatID:   chatID,
		SinceID:  sinceID,
		CanReply: true,
	})
}

// PostChatReply handles inline replies from AMP email.
// @Summary Post reply from AMP email
// @Tags AMP
// @Accept json
// @Produce json
// @Param id path int true "Chat ID"
// @Param rt query string true "Token (HMAC)"
// @Param uid query int true "User ID"
// @Param exp query int true "Token expiry timestamp"
// @Param tid query int false "Email tracking ID for analytics"
// @Param body body object true "Message body with 'message' field"
// @Success 200 {object} ReplyResponse
// @Failure 400 {object} ReplyResponse
// @Router /amp/chat/{id}/reply [post]
func PostChatReply(c *fiber.Ctx) error {
	userID, chatID, err := ValidateToken(c)
	if err != nil || userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ReplyResponse{
			Success: false,
			Message: "Invalid token",
		})
	}

	// Get optional tracking ID from query param
	var emailTrackingID *uint64
	if tidStr := c.Query("tid"); tidStr != "" {
		if tid, err := strconv.ParseUint(tidStr, 10, 64); err == nil {
			emailTrackingID = &tid
		}
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Message) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ReplyResponse{
			Success: false,
			Message: "Please enter a message.",
		})
	}

	// Trim and validate message length
	message := strings.TrimSpace(body.Message)
	if len(message) > 10000 {
		return c.Status(fiber.StatusBadRequest).JSON(ReplyResponse{
			Success: false,
			Message: "Message is too long. Please keep it under 10,000 characters.",
		})
	}

	db := database.DBConn

	// Verify user is still member of chat
	var memberUserID uint64
	db.Raw(`
		SELECT userid FROM chat_roster
		WHERE chatid = ? AND userid = ?
	`, chatID, userID).Scan(&memberUserID)

	if memberUserID == 0 {
		return c.Status(fiber.StatusForbidden).JSON(ReplyResponse{
			Success: false,
			Message: "You are not a member of this conversation.",
		})
	}

	// Insert the message
	result := db.Exec(`
		INSERT INTO chat_messages (chatid, userid, message, type, date, processingsuccessful)
		VALUES (?, ?, ?, 'Default', NOW(), 1)
	`, chatID, userID, message)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ReplyResponse{
			Success: false,
			Message: "Failed to send message. Please try on Freegle.",
		})
	}

	// Get the inserted message ID for logging
	var messageID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&messageID)

	// Update chat room latest message time
	db.Exec(`UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?`, chatID)

	// Track the AMP reply if we have an email tracking ID
	if emailTrackingID != nil {
		// Record that this email resulted in an AMP reply
		db.Exec(`
			UPDATE email_tracking
			SET replied_at = NOW(),
			    replied_via = 'amp'
			WHERE id = ?
		`, *emailTrackingID)

		// Also insert a tracking click record for analytics
		db.Exec(`
			INSERT INTO email_tracking_clicks (email_tracking_id, link_url, link_position, action, clicked_at)
			VALUES (?, 'amp://reply', 'amp_reply_form', 'amp_reply', NOW())
		`, *emailTrackingID)
	}

	// Log to Loki for dashboard analytics
	misc.GetLoki().LogChatReply("amp", chatID, userID, &messageID, emailTrackingID)

	return c.JSON(ReplyResponse{
		Success: true,
		Message: "Message sent!",
	})
}
