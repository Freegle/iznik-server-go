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
	"os"
	"strconv"
	"strings"
	"time"

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

// ChatMessage represents a message in the AMP chat response.
type ChatMessage struct {
	ID        uint64 `json:"id"`
	Message   string `json:"message"`
	FromUser  string `json:"fromUser"`
	FromImage string `json:"fromImage,omitempty"`
	Date      string `json:"date"`
	IsNew     bool   `json:"isNew"`
	IsMine    bool   `json:"isMine"`
}

// ChatResponse is the response for AMP chat message requests.
type ChatResponse struct {
	Items         []ChatMessage `json:"items"`
	ChatID        uint64        `json:"chatId"`
	OtherUserName string        `json:"otherUserName,omitempty"`
	ItemSubject   string        `json:"itemSubject,omitempty"`
	ItemAvailable bool          `json:"itemAvailable"`
	CanReply      bool          `json:"canReply"`
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

	// Check all required params present
	if token == "" || uid == "" || exp == "" || resID == "" {
		return 0, 0, nil
	}

	// Check expiry
	expTime, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > expTime {
		return 0, 0, nil
	}

	// Validate HMAC: "read" + user_id + resource_id + expiry
	secret := getAMPSecret()
	if secret == "" {
		return 0, 0, nil
	}

	message := "read" + uid + resID + exp
	expectedMAC := computeHMAC(message, secret)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(token), []byte(expectedMAC)) != 1 {
		return 0, 0, nil
	}

	userID, _ := strconv.ParseUint(uid, 10, 64)
	resourceID, _ := strconv.ParseUint(resID, 10, 64)

	// Verify user still exists
	db := database.DBConn
	var exists bool
	db.Raw("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&exists)
	if !exists {
		return 0, 0, nil
	}

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

// GetChatMessages returns the last 5 messages for AMP email "Earlier conversation" section.
// Excludes the triggering message (passed via 'exclude' param) to avoid duplication.
// @Summary Get chat messages for AMP email
// @Tags AMP
// @Produce json
// @Param id path int true "Chat ID"
// @Param rt query string true "Read token (HMAC)"
// @Param uid query int true "User ID"
// @Param exp query int true "Token expiry timestamp"
// @Param exclude query int false "Message ID to exclude (the one shown statically)"
// @Param since query int false "Message ID - newer messages marked as NEW"
// @Success 200 {object} ChatResponse
// @Router /amp/chat/{id} [get]
func GetChatMessages(c *fiber.Ctx) error {
	userID, chatID, err := ValidateReadToken(c)
	if err != nil || userID == 0 {
		// Return graceful fallback response - empty items triggers fallback UI
		return c.JSON(ChatResponse{
			Items:    []ChatMessage{},
			CanReply: false,
		})
	}

	// Message ID to exclude (the one shown statically in the email)
	excludeID, _ := strconv.ParseUint(c.Query("exclude", "0"), 10, 64)
	// Messages newer than this ID are marked as "NEW"
	sinceID, _ := strconv.ParseUint(c.Query("since", "0"), 10, 64)

	db := database.DBConn

	// Verify user is member of this chat
	var membership struct {
		UserID uint64
	}
	db.Raw(`
		SELECT userid FROM chat_roster
		WHERE chatid = ? AND userid = ?
	`, chatID, userID).Scan(&membership)

	if membership.UserID == 0 {
		return c.JSON(ChatResponse{Items: []ChatMessage{}, CanReply: false})
	}

	// Fetch last 5 messages (excluding the triggering message)
	var messages []struct {
		ID          uint64
		Message     string
		UserID      uint64
		DisplayName string
		ProfileURL  *string
		Date        string
	}

	db.Raw(`
		SELECT
			cm.id,
			cm.message,
			cm.userid,
			COALESCE(u.displayname, u.fullname, 'A Freegler') as displayname,
			u.profile as profileurl,
			DATE_FORMAT(cm.date, '%d %b, %l:%i%p') as date
		FROM chat_messages cm
		LEFT JOIN users u ON u.id = cm.userid
		WHERE cm.chatid = ?
		  AND cm.id != ?
		  AND cm.reviewrequired = 0
		  AND cm.processingsuccessful = 1
		ORDER BY cm.date DESC
		LIMIT 5
	`, chatID, excludeID).Scan(&messages)

	// Build response - reverse to show oldest first (chronological order)
	items := make([]ChatMessage, len(messages))
	for i, m := range messages {
		// Reverse index: put newest (index 0) at end
		reverseIdx := len(messages) - 1 - i

		profileURL := ""
		if m.ProfileURL != nil {
			profileURL = *m.ProfileURL
		}

		items[reverseIdx] = ChatMessage{
			ID:        m.ID,
			Message:   m.Message,
			FromUser:  m.DisplayName,
			FromImage: profileURL,
			Date:      m.Date,
			IsNew:     sinceID > 0 && m.ID > sinceID,
			IsMine:    m.UserID == userID,
		}
	}

	// Get other user's name
	var otherUser struct {
		DisplayName string
	}
	db.Raw(`
		SELECT COALESCE(u.displayname, u.fullname, 'A Freegler') as displayname
		FROM chat_roster cr
		INNER JOIN users u ON u.id = cr.userid
		WHERE cr.chatid = ? AND cr.userid != ?
		LIMIT 1
	`, chatID, userID).Scan(&otherUser)

	// Check if related item is still available
	var itemInfo struct {
		Subject   string
		Available bool
	}
	db.Raw(`
		SELECT
			m.subject,
			(m.type = 'Offer' AND m.availablenow = 1) as available
		FROM chat_rooms cr
		INNER JOIN messages m ON m.id = cr.refmsgid
		WHERE cr.id = ?
	`, chatID).Scan(&itemInfo)

	return c.JSON(ChatResponse{
		Items:         items,
		ChatID:        chatID,
		OtherUserName: otherUser.DisplayName,
		ItemSubject:   itemInfo.Subject,
		ItemAvailable: itemInfo.Available,
		CanReply:      true,
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
	misc.GetLokiClient().LogChatReply("amp", chatID, userID, &messageID, tokenResult.EmailTrackingID)

	return c.JSON(ReplyResponse{
		Success: true,
		Message: "Message sent!",
	})
}
