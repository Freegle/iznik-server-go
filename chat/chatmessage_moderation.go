package chat

import (
	"regexp"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type ModerationRequest struct {
	ID     uint64 `json:"id"`
	Action string `json:"action"`
}

// PostChatMessageModeration handles moderation actions on chat messages:
// Approve, ApproveAllFuture, Reject, Hold, Release, Redact
func PostChatMessageModeration(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req ModerationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	// Check caller is a moderator on at least one group
	var modCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&modCount)
	if modCount == 0 {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator")
	}

	switch req.Action {
	case "Approve":
		return approveChatMessage(c, db, myid, req.ID, false)
	case "ApproveAllFuture":
		return approveChatMessage(c, db, myid, req.ID, true)
	case "Reject":
		return rejectChatMessage(c, db, myid, req.ID)
	case "Hold":
		return holdChatMessage(c, db, myid, req.ID)
	case "Release":
		return releaseChatMessage(c, db, myid, req.ID)
	case "Redact":
		return redactChatMessage(c, db, myid, req.ID)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Invalid action: "+req.Action)
	}
}

type reviewMessage struct {
	ID     uint64 `gorm:"column:id"`
	Chatid uint64 `gorm:"column:chatid"`
	Userid uint64 `gorm:"column:userid"`
	Message string `gorm:"column:message"`
	HeldBy uint64 `gorm:"column:heldbyuser"`
}

func fetchReviewMessage(db *gorm.DB, msgID uint64) *reviewMessage {
	var msg reviewMessage
	db.Raw("SELECT chat_messages.id, chat_messages.chatid, chat_messages.userid, chat_messages.message, "+
		"COALESCE(chat_messages_held.userid, 0) AS heldbyuser "+
		"FROM chat_messages "+
		"LEFT JOIN chat_messages_held ON chat_messages_held.msgid = chat_messages.id "+
		"INNER JOIN chat_rooms ON chat_rooms.id = chat_messages.chatid "+
		"WHERE chat_messages.id = ? AND chat_messages.reviewrequired = 1",
		msgID).Scan(&msg)

	if msg.ID == 0 {
		return nil
	}
	return &msg
}

// checkHoldConflict returns true if the message is held by a different moderator.
func checkHoldConflict(msg *reviewMessage, myid uint64) bool {
	return msg.HeldBy != 0 && msg.HeldBy != myid
}

// autoApproveModmails approves any ModMail messages after the given message in the same chat.
func autoApproveModmails(db *gorm.DB, myid uint64, chatID uint64, afterMsgID uint64) {
	var modmailIDs []uint64
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? AND id > ? AND reviewrequired = 1 AND type = 'ModMail'",
		chatID, afterMsgID).Scan(&modmailIDs)

	for _, id := range modmailIDs {
		db.Exec("UPDATE chat_messages SET reviewrequired = 0, reviewedby = ? WHERE id = ?", myid, id)
	}
}

// updateMessageCounts recalculates valid/invalid message counts for a chat room.
func updateMessageCounts(db *gorm.DB, chatID uint64) {
	type countRow struct {
		Valid int
		Count int64
	}

	var counts []countRow
	db.Raw("SELECT CASE WHEN reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1 THEN 1 ELSE 0 END AS valid, "+
		"COUNT(*) AS count FROM chat_messages WHERE chatid = ? "+
		"GROUP BY CASE WHEN reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1 THEN 1 ELSE 0 END",
		chatID).Scan(&counts)

	var msgValid, msgInvalid int64
	for _, c := range counts {
		if c.Valid == 1 {
			msgValid = c.Count
		} else {
			msgInvalid = c.Count
		}
	}

	// For Mod2Mod chats, force msginvalid to 0
	var chattype string
	db.Raw("SELECT chattype FROM chat_rooms WHERE id = ?", chatID).Scan(&chattype)
	if chattype == "Mod2Mod" {
		msgInvalid = 0
	}

	db.Exec("UPDATE chat_rooms SET msgvalid = ?, msginvalid = ?, latestmessage = ? WHERE id = ?",
		msgValid, msgInvalid, time.Now(), chatID)
}

func approveChatMessage(c *fiber.Ctx, db *gorm.DB, myid uint64, msgID uint64, approveAllFuture bool) error {
	msg := fetchReviewMessage(db, msgID)
	if msg == nil {
		return fiber.NewError(fiber.StatusNotFound, "Message not found or not requiring review")
	}

	if checkHoldConflict(msg, myid) {
		return fiber.NewError(fiber.StatusConflict, "Message is held by another moderator")
	}

	// Approve the message
	db.Exec("UPDATE chat_messages SET reviewrequired = 0, reviewedby = ? WHERE id = ?", myid, msgID)

	// Auto-approve any ModMail messages after this one in the same chat
	autoApproveModmails(db, myid, msg.Chatid, msgID)

	// Update message counts
	updateMessageCounts(db, msg.Chatid)

	// Remove hold if it exists
	db.Exec("DELETE FROM chat_messages_held WHERE msgid = ?", msgID)

	if approveAllFuture {
		// Set user's chatmodstatus to Unmoderated
		db.Exec("UPDATE users SET chatmodstatus = 'Unmoderated' WHERE id = ?", msg.Userid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func rejectChatMessage(c *fiber.Ctx, db *gorm.DB, myid uint64, msgID uint64) error {
	msg := fetchReviewMessage(db, msgID)
	if msg == nil {
		return fiber.NewError(fiber.StatusNotFound, "Message not found or not requiring review")
	}

	// Reject the message
	db.Exec("UPDATE chat_messages SET reviewrequired = 0, reviewedby = ?, reviewrejected = 1 WHERE id = ?", myid, msgID)

	// Auto-approve any ModMail messages after this one
	autoApproveModmails(db, myid, msg.Chatid, msgID)

	// Find and reject all identical pending messages from last 24 hours (spam flood prevention)
	cutoff := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

	type dupMsg struct {
		ID     uint64
		Chatid uint64
	}
	var dups []dupMsg
	db.Raw("SELECT id, chatid FROM chat_messages WHERE date >= ? AND reviewrequired = 1 AND message = ? AND id != ?",
		cutoff, msg.Message, msgID).Scan(&dups)

	// Track affected chat IDs for count updates
	affectedChats := map[uint64]bool{msg.Chatid: true}

	for _, dup := range dups {
		db.Exec("UPDATE chat_messages SET reviewrequired = 0, reviewedby = ?, reviewrejected = 1 WHERE id = ?", myid, dup.ID)
		affectedChats[dup.Chatid] = true
	}

	// Update message counts for all affected chats
	for chatID := range affectedChats {
		updateMessageCounts(db, chatID)
	}

	// Remove hold if it exists
	db.Exec("DELETE FROM chat_messages_held WHERE msgid = ?", msgID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func holdChatMessage(c *fiber.Ctx, db *gorm.DB, myid uint64, msgID uint64) error {
	// Verify the message exists and requires review
	var reviewRequired int
	db.Raw("SELECT reviewrequired FROM chat_messages WHERE id = ?", msgID).Scan(&reviewRequired)
	if reviewRequired != 1 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found or not requiring review")
	}

	// REPLACE INTO handles the case where it's already held
	db.Exec("REPLACE INTO chat_messages_held (msgid, userid) VALUES (?, ?)", msgID, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func releaseChatMessage(c *fiber.Ctx, db *gorm.DB, myid uint64, msgID uint64) error {
	// Delete the hold record
	db.Exec("DELETE FROM chat_messages_held WHERE msgid = ?", msgID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// Email regex pattern matching PHP's Message::EMAIL_REGEXP
var emailRegexp = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

func redactChatMessage(c *fiber.Ctx, db *gorm.DB, myid uint64, msgID uint64) error {
	msg := fetchReviewMessage(db, msgID)
	if msg == nil {
		return fiber.NewError(fiber.StatusNotFound, "Message not found or not requiring review")
	}

	if checkHoldConflict(msg, myid) {
		return fiber.NewError(fiber.StatusConflict, "Message is held by another moderator")
	}

	// Replace email addresses with placeholder
	cleaned := emailRegexp.ReplaceAllString(msg.Message, "(email removed)")

	if cleaned != msg.Message {
		db.Exec("UPDATE chat_messages SET message = ? WHERE id = ?", cleaned, msgID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
