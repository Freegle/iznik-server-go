// Package amp provides AMP email endpoints for dynamic email content.
//
// This package implements the AMP for Email specification, allowing
// dynamic content in emails (e.g., chat messages, job listings) and
// inline actions (e.g., replying to messages).
//
// Security Model:
// - READ tokens: HMAC-SHA256 based, reusable within expiry period
// - WRITE tokens: Database-stored nonce, one-time use only
//
// This protects against email forwarding attacks:
// - If forwarded, recipient can view messages (privacy leak, acceptable)
// - If forwarded, recipient can only send ONE reply (then token is invalidated)
package amp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/gofiber/fiber/v2"
)

// AmpWriteToken represents a one-time use write token for AMP forms.
type AmpWriteToken struct {
	ID              uint64     `json:"id" gorm:"primaryKey"`
	Nonce           string     `json:"nonce" gorm:"column:nonce"`
	UserID          uint64     `json:"user_id" gorm:"column:user_id"`
	ChatID          uint64     `json:"chat_id" gorm:"column:chat_id"`
	EmailTrackingID *uint64    `json:"email_tracking_id" gorm:"column:email_tracking_id"`
	ExpiresAt       time.Time  `json:"expires_at" gorm:"column:expires_at"`
	UsedAt          *time.Time `json:"used_at" gorm:"column:used_at"`
	CreatedAt       time.Time  `json:"created_at" gorm:"column:created_at"`
}

func (AmpWriteToken) TableName() string {
	return "amp_write_tokens"
}

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

// ValidateReadToken validates HMAC-based read tokens for amp-list.
// Read tokens are reusable within their expiry period.
// Returns (userID, resourceID, error). On failure, returns (0, 0, nil) for graceful fallback.
func ValidateReadToken(c *fiber.Ctx) (uint64, uint64, error) {
	token := c.Query("rt")  // Read token
	uid := c.Query("uid")   // User ID
	exp := c.Query("exp")   // Expiry timestamp
	resID := c.Params("id") // Resource ID (chat ID, etc.)

	log.Printf("[AMP] ValidateReadToken: resID=%s, uid=%s, exp=%s, token_len=%d", resID, uid, exp, len(token))

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

	// Validate HMAC: "read" + user_id + resource_id + expiry
	secret := getAMPSecret()
	if secret == "" {
		log.Printf("[AMP] No secret configured, failing validation")
		return 0, 0, nil
	}

	message := "read" + uid + resID + exp
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

// WriteTokenResult contains the validated write token information.
type WriteTokenResult struct {
	UserID          uint64
	ChatID          uint64
	EmailTrackingID *uint64
}

// ValidateWriteToken validates one-time-use write tokens for amp-form.
// Write tokens can only be used ONCE and are stored in the database.
// Returns WriteTokenResult and error. On failure, returns an error.
func ValidateWriteToken(c *fiber.Ctx) (*WriteTokenResult, error) {
	nonce := c.Query("wt")  // Write token (nonce)
	reqChatID := c.Params("id")

	if nonce == "" || reqChatID == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Missing token")
	}

	chatIDUint, err := strconv.ParseUint(reqChatID, 10, 64)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Invalid chat ID")
	}

	db := database.DBConn

	// Look up token - use transaction for atomic read-and-mark
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var token AmpWriteToken
	result := tx.Raw(`
		SELECT id, user_id, chat_id, email_tracking_id, used_at, expires_at
		FROM amp_write_tokens
		WHERE nonce = ?
		FOR UPDATE
	`, nonce).Scan(&token)

	if result.Error != nil || token.ID == 0 {
		tx.Rollback()
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid token")
	}

	// Check not already used
	if token.UsedAt != nil {
		tx.Rollback()
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Token already used")
	}

	// Check not expired
	if time.Now().After(token.ExpiresAt) {
		tx.Rollback()
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Token expired")
	}

	// Check chat ID matches
	if token.ChatID != chatIDUint {
		tx.Rollback()
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Token mismatch")
	}

	// IMMEDIATELY mark as used - do this BEFORE any other operation
	updateResult := tx.Exec(`UPDATE amp_write_tokens SET used_at = NOW() WHERE id = ? AND used_at IS NULL`, token.ID)
	if updateResult.RowsAffected == 0 {
		tx.Rollback()
		return nil, fiber.NewError(fiber.StatusConflict, "Token already used")
	}

	tx.Commit()

	return &WriteTokenResult{
		UserID:          token.UserID,
		ChatID:          token.ChatID,
		EmailTrackingID: token.EmailTrackingID,
	}, nil
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
func getUserInfo(userID uint64) userInfo {
	db := database.DBConn
	var result struct {
		ID        uint64
		Fullname  *string
		Firstname *string
		Lastname  *string
		ImageUID  *string
	}

	db.Raw(`
		SELECT u.id, u.fullname, u.firstname, u.lastname, ui.externaluid AS imageuid
		FROM users u
		LEFT JOIN users_images ui ON ui.userid = u.id
		WHERE u.id = ?
		ORDER BY ui.id DESC
		LIMIT 1
	`, userID).Scan(&result)

	// Build display name
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

	// Build profile image URL
	imageURL := "https://www.ilovefreegle.org/defaultprofile.png"
	if result.ImageUID != nil && *result.ImageUID != "" {
		imageURL = misc.GetImageDeliveryUrl(*result.ImageUID, "")
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
	userID, chatID, err := ValidateReadToken(c)
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
// @Param wt query string true "Write token (one-time nonce)"
// @Param body body object true "Message body with 'message' field"
// @Success 200 {object} ReplyResponse
// @Failure 400 {object} ReplyResponse
// @Router /amp/chat/{id}/reply [post]
func PostChatReply(c *fiber.Ctx) error {
	tokenResult, err := ValidateWriteToken(c)
	if err != nil {
		// Include specific error for debugging
		errMsg := "Unable to send reply"
		if fiberErr, ok := err.(*fiber.Error); ok {
			errMsg = fiberErr.Message
		}
		return c.Status(fiber.StatusBadRequest).JSON(ReplyResponse{
			Success: false,
			Message: errMsg,
		})
	}

	userID := tokenResult.UserID
	chatID := tokenResult.ChatID

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
	if tokenResult.EmailTrackingID != nil {
		// Record that this email resulted in an AMP reply
		db.Exec(`
			UPDATE email_tracking
			SET replied_at = NOW(),
			    replied_via = 'amp'
			WHERE id = ?
		`, *tokenResult.EmailTrackingID)

		// Also insert a tracking click record for analytics
		db.Exec(`
			INSERT INTO email_tracking_clicks (email_tracking_id, link_url, link_position, action, clicked_at)
			VALUES (?, 'amp://reply', 'amp_reply_form', 'amp_reply', NOW())
		`, *tokenResult.EmailTrackingID)
	}

	// Log to Loki for dashboard analytics
	misc.GetLoki().LogChatReply("amp", chatID, userID, &messageID, tokenResult.EmailTrackingID)

	return c.JSON(ReplyResponse{
		Success: true,
		Message: "Message sent!",
	})
}
