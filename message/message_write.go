package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"time"
)

// PostMessageRequest handles action-based POST to /message.
type PostMessageRequest struct {
	ID       uint64  `json:"id"`
	Action   string  `json:"action"`
	Userid   *uint64 `json:"userid"`
	Count    *int    `json:"count"`
	Outcome  string  `json:"outcome"`
	Happiness *string `json:"happiness"`
	Comment  *string `json:"comment"`
	Message  *string `json:"message"`
}

// PostMessage dispatches POST /message actions.
func PostMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	switch req.Action {
	case "Promise":
		return handlePromise(c, myid, req)
	case "Renege":
		return handleRenege(c, myid, req)
	case "OutcomeIntended":
		return handleOutcomeIntended(c, myid, req)
	case "Outcome":
		return handleOutcome(c, myid, req)
	case "AddBy":
		return handleAddBy(c, myid, req)
	case "RemoveBy":
		return handleRemoveBy(c, myid, req)
	case "View":
		return handleView(c, myid, req)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action")
	}
}

// handlePromise records a promise of an item to a user.
func handlePromise(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	// Verify message exists and is owned by the current user.
	var msgUserid uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", req.ID).Scan(&msgUserid)
	if msgUserid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
	if msgUserid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	promisedTo := myid
	if req.Userid != nil && *req.Userid > 0 {
		promisedTo = *req.Userid
	}

	// REPLACE INTO - idempotent.
	db.Exec("REPLACE INTO messages_promises (msgid, userid) VALUES (?, ?)", req.ID, promisedTo)

	// Create a chat message of type Promised if promising to another user.
	if req.Userid != nil && *req.Userid > 0 && *req.Userid != myid {
		createSystemChatMessage(db, myid, *req.Userid, req.ID, utils.CHAT_MESSAGE_PROMISED)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRenege removes a promise and records reliability data.
func handleRenege(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	var msgUserid uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", req.ID).Scan(&msgUserid)
	if msgUserid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
	if msgUserid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	promisedTo := myid
	if req.Userid != nil && *req.Userid > 0 {
		promisedTo = *req.Userid
	}

	// Record renege for reliability tracking (only if not reneging on self).
	if promisedTo != myid {
		db.Exec("INSERT INTO messages_reneged (userid, msgid) VALUES (?, ?)", promisedTo, req.ID)
	}

	// Delete the promise.
	db.Exec("DELETE FROM messages_promises WHERE msgid = ? AND userid = ?", req.ID, promisedTo)

	// Create a chat message of type Reneged if reneging on another user.
	if req.Userid != nil && *req.Userid > 0 && *req.Userid != myid {
		createSystemChatMessage(db, myid, *req.Userid, req.ID, utils.CHAT_MESSAGE_RENEGED)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleOutcomeIntended records an intended outcome.
func handleOutcomeIntended(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if req.Outcome == "" {
		return fiber.NewError(fiber.StatusBadRequest, "outcome is required")
	}

	// Verify valid outcome.
	if req.Outcome != utils.OUTCOME_TAKEN && req.Outcome != utils.OUTCOME_RECEIVED && req.Outcome != utils.OUTCOME_WITHDRAWN {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid outcome")
	}

	// Simple insert-or-update.
	db.Exec("INSERT INTO messages_outcomes_intended (msgid, outcome) VALUES (?, ?) ON DUPLICATE KEY UPDATE outcome = VALUES(outcome)",
		req.ID, req.Outcome)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleOutcome marks a message with an outcome (Taken, Received, Withdrawn).
// This has complex async side effects that PHP handles via background jobs.
// We record the outcome in the DB and queue background processing.
func handleOutcome(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if req.Outcome == "" {
		return fiber.NewError(fiber.StatusBadRequest, "outcome is required")
	}

	if req.Outcome != utils.OUTCOME_TAKEN && req.Outcome != utils.OUTCOME_RECEIVED && req.Outcome != utils.OUTCOME_WITHDRAWN {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid outcome")
	}

	// Verify message exists.
	var msgUserid uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", req.ID).Scan(&msgUserid)
	if msgUserid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	// Check for existing outcome (prevent duplicates unless expired).
	var existingOutcome string
	db.Raw("SELECT outcome FROM messages_outcomes WHERE msgid = ?", req.ID).Scan(&existingOutcome)
	if existingOutcome != "" && existingOutcome != utils.OUTCOME_EXPIRED {
		return fiber.NewError(fiber.StatusConflict, "Outcome already recorded")
	}

	// Clear any intended outcome.
	db.Exec("DELETE FROM messages_outcomes_intended WHERE msgid = ?", req.ID)

	// Clear any existing outcome (for expired overwrite).
	db.Exec("DELETE FROM messages_outcomes WHERE msgid = ?", req.ID)

	// Record the outcome.
	happiness := ""
	if req.Happiness != nil {
		happiness = *req.Happiness
	}
	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}

	if happiness != "" {
		db.Exec("INSERT INTO messages_outcomes (msgid, outcome, happiness, comments) VALUES (?, ?, ?, ?)",
			req.ID, req.Outcome, happiness, comment)
	} else {
		db.Exec("INSERT INTO messages_outcomes (msgid, outcome, comments) VALUES (?, ?, ?)",
			req.ID, req.Outcome, comment)
	}

	// Queue background processing for notifications/chat messages.
	// PHP's backgroundMark() handles: logging, chat notifications to interested users,
	// marking chats as up-to-date.
	messageForOthers := ""
	if req.Message != nil {
		messageForOthers = *req.Message
	}
	userid := uint64(0)
	if req.Userid != nil {
		userid = *req.Userid
	}

	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'outcome', ?, 'happiness', ?, 'comment', ?, 'userid', ?, 'byuser', ?, 'message', ?))",
		"message_outcome", req.ID, req.Outcome, happiness, comment, userid, myid, messageForOthers)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleAddBy records who is taking items from a message.
func handleAddBy(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	count := 1
	if req.Count != nil {
		count = *req.Count
	}

	userid := uint64(0)
	if req.Userid != nil {
		userid = *req.Userid
	}

	// Check if this user already has an entry.
	type byEntry struct {
		ID    uint64
		Count int
	}
	var existing byEntry
	db.Raw("SELECT id, count FROM messages_by WHERE msgid = ? AND userid = ?",
		req.ID, userid).Scan(&existing)
	existingID := existing.ID
	existingCount := existing.Count

	if existingID > 0 {
		// Restore old count before updating.
		db.Exec("UPDATE messages SET availablenow = LEAST(availableinitially, availablenow + ?) WHERE id = ?",
			existingCount, req.ID)
		db.Exec("UPDATE messages_by SET count = ? WHERE id = ?", count, existingID)
	} else {
		db.Exec("INSERT INTO messages_by (userid, msgid, count) VALUES (?, ?, ?)",
			userid, req.ID, count)
	}

	// Reduce available count.
	db.Exec("UPDATE messages SET availablenow = GREATEST(LEAST(availableinitially, availablenow - ?), 0) WHERE id = ?",
		count, req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRemoveBy removes a taker and restores available count.
func handleRemoveBy(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	userid := uint64(0)
	if req.Userid != nil {
		userid = *req.Userid
	}

	// Find the entry.
	type byEntry struct {
		ID    uint64
		Count int
	}
	var entry byEntry
	db.Raw("SELECT id, count FROM messages_by WHERE msgid = ? AND userid = ?",
		req.ID, userid).Scan(&entry)
	entryID := entry.ID
	entryCount := entry.Count

	if entryID > 0 {
		// Restore count and delete entry.
		db.Exec("UPDATE messages SET availablenow = LEAST(availableinitially, availablenow + ?) WHERE id = ?",
			entryCount, req.ID)
		db.Exec("DELETE FROM messages_by WHERE id = ?", entryID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleView records a message view, de-duplicating within 30 minutes.
func handleView(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	// Check for a recent view within 30 minutes to avoid redundant writes.
	var recentCount int64
	db.Raw("SELECT COUNT(*) FROM messages_likes WHERE msgid = ? AND userid = ? AND type = 'View' AND timestamp >= DATE_SUB(NOW(), INTERVAL 30 MINUTE)",
		req.ID, myid).Scan(&recentCount)

	if recentCount == 0 {
		db.Exec("INSERT INTO messages_likes (msgid, userid, type) VALUES (?, ?, 'View') ON DUPLICATE KEY UPDATE timestamp = NOW(), count = count + 1",
			req.ID, myid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// createSystemChatMessage creates a system chat message between two users for a message.
func createSystemChatMessage(db *gorm.DB, fromUser uint64, toUser uint64, refmsgid uint64, msgType string) {
	// Find or create chat room between these users about this message.
	var chatID uint64
	db.Raw("SELECT id FROM chat_rooms WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?) LIMIT 1",
		fromUser, toUser, toUser, fromUser).Scan(&chatID)

	if chatID == 0 {
		return
	}

	// Insert chat message.
	db.Exec("INSERT INTO chat_messages (chatid, userid, type, refmsgid, date, message, processingrequired) VALUES (?, ?, ?, ?, ?, '', 1)",
		chatID, fromUser, msgType, refmsgid, time.Now())
}
