package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// isModForMessage checks if the user is a system admin/support or a moderator/owner
// of any group the message is on.
func isModForMessage(db *gorm.DB, myid uint64, msgid uint64) bool {
	// Check system admin/support.
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	if systemrole == "Admin" || systemrole == "Support" {
		return true
	}

	// Check if mod of any group the message is on.
	var count int64
	db.Raw(`SELECT COUNT(*) FROM messages_groups mg
		JOIN memberships m ON m.groupid = mg.groupid
		WHERE mg.msgid = ? AND m.userid = ? AND m.role IN ('Moderator', 'Owner')`, msgid, myid).Scan(&count)
	return count > 0
}

// handleApprove approves a pending message.
func handleApprove(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Move to Approved.
	db.Exec("UPDATE messages_groups SET collection = 'Approved', approvedby = ?, approvedat = NOW() WHERE msgid = ? AND collection = 'Pending'",
		myid, req.ID)

	// Release any hold.
	db.Exec("UPDATE messages SET heldby = NULL WHERE id = ?", req.ID)

	// Queue email to poster.
	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'byuser', ?))",
		"email_message_approved", req.ID, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleReject rejects a pending message.
func handleReject(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	subject := ""
	if req.Subject != nil {
		subject = *req.Subject
	}
	body := ""
	if req.Body != nil {
		body = *req.Body
	}
	stdmsgid := uint64(0)
	if req.Stdmsgid != nil {
		stdmsgid = *req.Stdmsgid
	}

	// Delete from groups where pending.
	db.Exec("DELETE FROM messages_groups WHERE msgid = ? AND collection = 'Pending'", req.ID)

	// Queue rejection email.
	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?))",
		"email_message_rejected", req.ID, myid, subject, body, stdmsgid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleDeleteMessage deletes a message (mod action).
func handleDeleteMessage(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	db.Exec("UPDATE messages_groups SET deleted = 1 WHERE msgid = ?", req.ID)
	db.Exec("UPDATE messages SET deleted = NOW() WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleSpam marks a message as spam.
func handleSpam(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Record for spam training (matching PHP Message::spam).
	db.Exec("REPLACE INTO messages_spamham (msgid, spamham) VALUES (?, 'Spam')", req.ID)

	// Delete the message (matching PHP - spam() calls delete()).
	db.Exec("UPDATE messages_groups SET deleted = 1 WHERE msgid = ?", req.ID)
	db.Exec("UPDATE messages SET deleted = NOW() WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleHold holds a pending message (assigns heldby to the mod).
func handleHold(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	db.Exec("UPDATE messages SET heldby = ? WHERE id = ?", myid, req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRelease releases a held message.
func handleRelease(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	db.Exec("UPDATE messages SET heldby = NULL WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleApproveEdits approves pending edits on a message.
func handleApproveEdits(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Clear the editedby flag.
	db.Exec("UPDATE messages SET editedby = NULL WHERE id = ?", req.ID)

	// Find the latest pending edit.
	type editRecord struct {
		ID         uint64
		Newsubject *string
		Newtext    *string
	}
	var edit editRecord
	db.Raw("SELECT id, newsubject, newtext FROM messages_edits WHERE msgid = ? AND reviewrequired = 1 AND approvedat IS NULL AND revertedat IS NULL ORDER BY id DESC LIMIT 1",
		req.ID).Scan(&edit)

	if edit.ID > 0 {
		// Apply the edits.
		if edit.Newsubject != nil {
			db.Exec("UPDATE messages SET subject = ? WHERE id = ?", *edit.Newsubject, req.ID)
		}
		if edit.Newtext != nil {
			db.Exec("UPDATE messages SET textbody = ? WHERE id = ?", *edit.Newtext, req.ID)
		}
		// Mark as approved.
		db.Exec("UPDATE messages_edits SET approvedat = NOW() WHERE id = ?", edit.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleRevertEdits reverts pending edits on a message.
func handleRevertEdits(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Clear the editedby flag.
	db.Exec("UPDATE messages SET editedby = NULL WHERE id = ?", req.ID)

	// Mark all pending edits as reverted.
	db.Exec("UPDATE messages_edits SET revertedat = NOW() WHERE msgid = ? AND reviewrequired = 1 AND approvedat IS NULL AND revertedat IS NULL",
		req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handlePartnerConsent records partner consent on a message.
// Matches PHP Message.php:partnerConsent() - requires mod role and partner name.
func handlePartnerConsent(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	if req.Partner == nil || *req.Partner == "" {
		return fiber.NewError(fiber.StatusBadRequest, "partner is required")
	}

	// Look up partner in partners_keys.
	var partnerID uint64
	db.Raw("SELECT id FROM partners_keys WHERE partner LIKE ?", *req.Partner).Scan(&partnerID)
	if partnerID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Partner not found")
	}

	// Record consent in partners_messages.
	db.Exec("INSERT INTO partners_messages (msgid, partnerid) VALUES (?, ?) ON DUPLICATE KEY UPDATE msgid = ?", req.ID, partnerID, req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleReply queues a mod reply email to the message poster.
func handleReply(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	subject := ""
	if req.Subject != nil {
		subject = *req.Subject
	}
	body := ""
	if req.Body != nil {
		body = *req.Body
	}
	stdmsgid := uint64(0)
	if req.Stdmsgid != nil {
		stdmsgid = *req.Stdmsgid
	}

	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('msgid', ?, 'byuser', ?, 'subject', ?, 'body', ?, 'stdmsgid', ?))",
		"email_message_reply", req.ID, myid, subject, body, stdmsgid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleJoinAndPost joins a group and posts a message in one action.
func handleJoinAndPost(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if req.Groupid == nil || *req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}
	if req.Type == "" {
		return fiber.NewError(fiber.StatusBadRequest, "type is required (Offer or Wanted)")
	}
	if req.Type != "Offer" && req.Type != "Wanted" {
		return fiber.NewError(fiber.StatusBadRequest, "type must be Offer or Wanted")
	}

	item := ""
	if req.Item != nil {
		item = *req.Item
	}
	textbody := ""
	if req.Textbody != nil {
		textbody = *req.Textbody
	}

	subject := req.Type + ": " + item

	// Join group if not already a member.
	db.Exec("INSERT IGNORE INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Approved')",
		myid, *req.Groupid)

	// Create message.
	result := db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, ?, ?, ?, NOW(), NOW(), 'Platform')",
		myid, req.Type, subject, textbody)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create message")
	}

	var newMsgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", myid).Scan(&newMsgID)

	if newMsgID == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve message ID")
	}

	// Add to group.
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())",
		newMsgID, *req.Groupid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newMsgID})
}

// PatchMessage updates a message (PATCH /message).
func PatchMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type PatchMessageRequest struct {
		ID           uint64   `json:"id"`
		Subject      *string  `json:"subject"`
		Textbody     *string  `json:"textbody"`
		Type         *string  `json:"type"`
		Item         *string  `json:"item"`
		Availablenow *int     `json:"availablenow"`
		Lat          *float64 `json:"lat"`
		Lng          *float64 `json:"lng"`
		Locationid   *uint64  `json:"locationid"`
	}

	var req PatchMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	// Check ownership or mod permission.
	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", req.ID).Scan(&fromuser)
	if fromuser == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	isOwner := fromuser == myid
	isMod := isModForMessage(db, myid, req.ID)

	if !isOwner && !isMod {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to modify this message")
	}

	// Get old values for edit tracking.
	type msgValues struct {
		Subject  string
		Textbody string
	}
	var old msgValues
	db.Raw("SELECT subject, COALESCE(textbody, '') as textbody FROM messages WHERE id = ?", req.ID).Scan(&old)

	// Apply updates.
	if req.Subject != nil {
		db.Exec("UPDATE messages SET subject = ? WHERE id = ?", *req.Subject, req.ID)
	}
	if req.Textbody != nil {
		db.Exec("UPDATE messages SET textbody = ? WHERE id = ?", *req.Textbody, req.ID)
	}
	if req.Type != nil {
		db.Exec("UPDATE messages SET type = ? WHERE id = ?", *req.Type, req.ID)
	}
	if req.Availablenow != nil {
		db.Exec("UPDATE messages SET availablenow = ? WHERE id = ?", *req.Availablenow, req.ID)
	}
	if req.Locationid != nil {
		db.Exec("UPDATE messages SET locationid = ? WHERE id = ?", *req.Locationid, req.ID)
	}

	// If subject or textbody changed and user is not mod, create edit record for review.
	subjectChanged := req.Subject != nil && *req.Subject != old.Subject
	textChanged := req.Textbody != nil && *req.Textbody != old.Textbody

	if (subjectChanged || textChanged) && !isMod {
		newSubject := old.Subject
		if req.Subject != nil {
			newSubject = *req.Subject
		}
		newText := old.Textbody
		if req.Textbody != nil {
			newText = *req.Textbody
		}

		db.Exec("INSERT INTO messages_edits (msgid, byuser, oldsubject, newsubject, oldtext, newtext, reviewrequired) VALUES (?, ?, ?, ?, ?, ?, 1)",
			req.ID, myid, old.Subject, newSubject, old.Textbody, newText)
		db.Exec("UPDATE messages SET editedby = ? WHERE id = ?", myid, req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteMessageEndpoint handles DELETE /message/:id.
func DeleteMessageEndpoint(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid message ID")
	}
	msgid := uint64(id)

	db := database.DBConn

	// Check ownership.
	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", msgid).Scan(&fromuser)
	if fromuser == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	if fromuser != myid && !isModForMessage(db, myid, msgid) {
		return fiber.NewError(fiber.StatusForbidden, "Not allowed to delete this message")
	}

	db.Exec("UPDATE messages SET deleted = NOW() WHERE id = ?", msgid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// PutMessage creates a new message (PUT /message).
func PutMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type PutMessageRequest struct {
		Groupid            uint64  `json:"groupid"`
		Type               string  `json:"type"`
		Subject            string  `json:"subject"`
		Textbody           string  `json:"textbody"`
		Locationid         *uint64 `json:"locationid"`
		Item               string  `json:"item"`
		Availableinitially *int    `json:"availableinitially"`
	}

	var req PutMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}
	if req.Type != "Offer" && req.Type != "Wanted" {
		return fiber.NewError(fiber.StatusBadRequest, "type must be Offer or Wanted")
	}
	if req.Subject == "" {
		return fiber.NewError(fiber.StatusBadRequest, "subject is required")
	}

	db := database.DBConn

	// Check user is member of group.
	var memberCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", myid, req.Groupid).Scan(&memberCount)
	if memberCount == 0 {
		return fiber.NewError(fiber.StatusForbidden, "Not a member of this group")
	}

	availInit := 1
	if req.Availableinitially != nil {
		availInit = *req.Availableinitially
	}

	// Create message.
	result := db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source, availableinitially, availablenow) VALUES (?, ?, ?, ?, NOW(), NOW(), 'Platform', ?, ?)",
		myid, req.Type, req.Subject, req.Textbody, availInit, availInit)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create message")
	}

	var newMsgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", myid).Scan(&newMsgID)

	if newMsgID == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve message ID")
	}

	// Add to group as Pending.
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Pending', NOW())",
		newMsgID, req.Groupid)

	// Add spatial data if locationid is provided.
	if req.Locationid != nil && *req.Locationid > 0 {
		var lat, lng float64
		db.Raw("SELECT lat, lng FROM locations WHERE id = ?", *req.Locationid).Row().Scan(&lat, &lng)
		if lat != 0 || lng != 0 {
			db.Exec("INSERT INTO messages_spatial (msgid, point, successful, groupid, msgtype) VALUES (?, ST_GeomFromText(CONCAT('POINT(', ?, ' ', ?, ')'), 3857), 1, ?, ?)",
				newMsgID, lng, lat, req.Groupid, req.Type)
		}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newMsgID})
}
