package message

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
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

	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", req.ID)
	db.Exec("UPDATE messages SET deleted = NOW(), messageid = NULL WHERE id = ?", req.ID)

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

// handleBackToPending moves an approved message back to pending.
func handleBackToPending(c *fiber.Ctx, myid uint64, req PostMessageRequest) error {
	db := database.DBConn

	if !isModForMessage(db, myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator for this message")
	}

	// Move from Approved back to Pending.
	db.Exec("UPDATE messages_groups SET collection = 'Pending', approvedby = NULL, approvedat = NULL WHERE msgid = ? AND collection = 'Approved'",
		req.ID)

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
	db.Raw("SELECT id FROM partners_keys WHERE partner = ?", *req.Partner).Scan(&partnerID)
	if partnerID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Partner not found")
	}

	// Record consent in partners_messages.
	db.Exec("INSERT IGNORE INTO partners_messages (partnerid, msgid) VALUES (?, ?)", partnerID, req.ID)

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

	// Look up the existing draft message.
	type msgInfo struct {
		Fromuser uint64
		Type     string
	}
	var msg msgInfo
	db.Raw("SELECT fromuser, type FROM messages WHERE id = ?", req.ID).Scan(&msg)
	if msg.Fromuser == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
	if msg.Fromuser != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	// Find the group — from request, then messages_drafts, then messages_groups.
	groupid := uint64(0)
	if req.Groupid != nil && *req.Groupid > 0 {
		groupid = *req.Groupid
	} else {
		db.Raw("SELECT groupid FROM messages_drafts WHERE msgid = ? LIMIT 1", req.ID).Scan(&groupid)
	}
	if groupid == 0 {
		db.Raw("SELECT groupid FROM messages_groups WHERE msgid = ? LIMIT 1", req.ID).Scan(&groupid)
	}
	if groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	// Join group if not already a member.
	db.Exec("INSERT IGNORE INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Approved')",
		myid, groupid)

	// Submit: insert into messages_groups as Approved and clean up draft.
	db.Exec("INSERT IGNORE INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())",
		req.ID, groupid)
	db.Exec("DELETE FROM messages_drafts WHERE msgid = ?", req.ID)

	// Check if user has a password (to determine if they're a new user).
	var hasPassword int64
	db.Raw("SELECT COUNT(*) FROM users_logins WHERE userid = ? AND type = 'Native'", myid).Scan(&hasPassword)

	resp := fiber.Map{
		"ret":     0,
		"status":  "Success",
		"id":      req.ID,
		"groupid": groupid,
	}

	if hasPassword == 0 {
		// New user without a password — generate one and return it.
		password := utils.RandomHex(8)
		salt := auth.GetPasswordSalt()
		hashed := auth.HashPassword(password, salt)

		// uid must be the user ID (not email) so that VerifyPassword can find the row.
		db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?) ON DUPLICATE KEY UPDATE credentials = VALUES(credentials), salt = VALUES(salt)",
			myid, myid, hashed, salt)
		resp["newuser"] = true
		resp["newpassword"] = password
	}

	return c.JSON(resp)
}

// PatchMessage updates a message (PATCH /message).
//
// @Summary Update a message
// @Tags message
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/message [patch]
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
		Attachments  []uint64 `json:"attachments"`
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

	// Build a single UPDATE with all changed fields.
	setClauses := []string{}
	args := []interface{}{}

	if req.Subject != nil {
		setClauses = append(setClauses, "subject = ?")
		args = append(args, *req.Subject)
	}
	if req.Textbody != nil {
		setClauses = append(setClauses, "textbody = ?")
		args = append(args, *req.Textbody)
	}
	if req.Type != nil {
		setClauses = append(setClauses, "type = ?")
		args = append(args, *req.Type)
	}
	if req.Availablenow != nil {
		setClauses = append(setClauses, "availablenow = ?")
		args = append(args, *req.Availablenow)
	}
	if req.Locationid != nil {
		setClauses = append(setClauses, "locationid = ?")
		args = append(args, *req.Locationid)
	}

	if len(setClauses) > 0 {
		args = append(args, req.ID)
		db.Exec("UPDATE messages SET "+strings.Join(setClauses, ", ")+" WHERE id = ?", args...)
	}

	// Update attachment ordering if provided.
	if len(req.Attachments) > 0 {
		for i, attid := range req.Attachments {
			primary := i == 0
			db.Exec("UPDATE messages_attachments SET msgid = ?, `primary` = ? WHERE id = ?", req.ID, primary, attid)
		}

		// Delete any attachments for this message that are not in the new list.
		db.Exec("DELETE FROM messages_attachments WHERE msgid = ? AND id NOT IN (?)", req.ID, req.Attachments)
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
//
// @Summary Delete a message
// @Tags message
// @Produce json
// @Param id path integer true "Message ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/message/{id} [delete]
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

// findOrCreateUserForDraft looks up a user by email, or creates one if not found.
// Returns the user ID, JWT string, persistent token map, and any error.
// This supports the give/want flow where users post without signing up first.
//
// SECURITY: For existing users, we do NOT create a session/JWT. Knowing someone's
// email address must not grant authentication. A session is only created for
// brand-new users.
func findOrCreateUserForDraft(db *gorm.DB, email string) (uint64, string, fiber.Map, error) {
	email = strings.TrimSpace(email)

	// Basic email validation.
	if !strings.Contains(email, "@") || len(email) > 254 {
		return 0, "", nil, fmt.Errorf("invalid email address")
	}

	// Look up existing user by email.
	var existingUID uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? LIMIT 1", email).Scan(&existingUID)

	if existingUID > 0 {
		// Existing user — return their ID so the draft is linked to them,
		// but do NOT create a session.  The user must authenticate separately.
		return existingUID, "", nil, nil
	}

	// New user — create user, email, session, JWT.
	result := db.Exec("INSERT INTO users (added) VALUES (NOW())")
	if result.Error != nil {
		return 0, "", nil, result.Error
	}

	var newUserID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newUserID)
	if newUserID == 0 {
		return 0, "", nil, fmt.Errorf("failed to get new user ID")
	}

	// Add email.
	canon := user.CanonicalizeEmail(email)
	db.Exec("INSERT INTO users_emails (userid, email, preferred, validated, canon) VALUES (?, ?, 1, NOW(), ?)",
		newUserID, email, canon)

	// Create session.
	token := utils.RandomHex(16)
	db.Exec("INSERT INTO sessions (userid, series, token, lastactive) VALUES (?, ?, ?, NOW())",
		newUserID, newUserID, token)

	// Use token to find our specific session (avoids race with concurrent requests).
	var sessionID uint64
	db.Raw("SELECT id FROM sessions WHERE userid = ? AND token = ? ORDER BY id DESC LIMIT 1", newUserID, token).Scan(&sessionID)

	// Generate JWT.
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":        fmt.Sprint(newUserID),
		"sessionid": fmt.Sprint(sessionID),
		"exp":       time.Now().Unix() + 30*24*60*60,
	})
	jwtString, err := jwtToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return 0, "", nil, err
	}

	persistent := fiber.Map{
		"id":     sessionID,
		"series": newUserID,
		"token":  token,
		"userid": newUserID,
	}
	return newUserID, jwtString, persistent, nil
}

// PutMessage creates a new message draft (PUT /message).
// Accepts both authenticated and unauthenticated requests (with email).
// For unauthenticated requests, finds or creates the user by email.
//
// @Summary Create or update a message
// @Tags message
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/message [put]
func PutMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	type PutMessageRequest struct {
		Groupid            uint64   `json:"groupid"`
		Type               string   `json:"type"`
		Messagetype        string   `json:"messagetype"` // Client sends this; alias for Type.
		Subject            string   `json:"subject"`
		Item               string   `json:"item"`
		Textbody           string   `json:"textbody"`
		Collection         string   `json:"collection"` // Draft (default) or Pending.
		Locationid         *uint64  `json:"locationid"`
		Availableinitially *int     `json:"availableinitially"`
		Availablenow       *int     `json:"availablenow"`
		Attachments        []uint64 `json:"attachments"`
		Email              string   `json:"email"`
	}

	var req PutMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Handle messagetype alias from client.
	if req.Type == "" && req.Messagetype != "" {
		req.Type = req.Messagetype
	}

	// Generate subject from type + item if subject not provided.
	if req.Subject == "" && req.Item != "" {
		req.Subject = req.Type + ": " + req.Item
	}

	// Default to Draft collection (client compose flow creates drafts).
	if req.Collection == "" {
		req.Collection = "Draft"
	}

	db := database.DBConn

	// Handle unauthenticated user with email — find or create, then generate JWT.
	var jwtString string
	var persistent fiber.Map
	if myid == 0 && req.Email != "" {
		var err error
		myid, jwtString, persistent, err = findOrCreateUserForDraft(db, req.Email)
		if err != nil {
			if strings.Contains(err.Error(), "invalid email") {
				return fiber.NewError(fiber.StatusBadRequest, "Invalid email address")
			}
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create user")
		}
	}

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if req.Type != "Offer" && req.Type != "Wanted" {
		return fiber.NewError(fiber.StatusBadRequest, "type must be Offer or Wanted")
	}

	// For non-Draft, require group membership.
	if req.Collection != "Draft" && req.Groupid > 0 {
		var memberCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", myid, req.Groupid).Scan(&memberCount)
		if memberCount == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Not a member of this group")
		}
	}

	availInit := 1
	if req.Availableinitially != nil {
		availInit = *req.Availableinitially
	}
	availNow := availInit
	if req.Availablenow != nil {
		availNow = *req.Availablenow
	}

	// Create message.
	result := db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source, availableinitially, availablenow) VALUES (?, ?, ?, ?, NOW(), NOW(), 'Platform', ?, ?)",
		myid, req.Type, req.Subject, req.Textbody, availInit, availNow)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create message")
	}

	var newMsgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", myid).Scan(&newMsgID)

	if newMsgID == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve message ID")
	}

	// For Draft collection, store in messages_drafts (matching PHP behavior).
	// For other collections, add to messages_groups.
	if req.Collection == "Draft" {
		db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)",
			newMsgID, req.Groupid, myid)
	} else if req.Groupid > 0 {
		db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, ?, NOW())",
			newMsgID, req.Groupid, req.Collection)
	}

	// Link attachments.
	for _, attID := range req.Attachments {
		db.Exec("UPDATE messages_attachments SET msgid = ? WHERE id = ?", newMsgID, attID)
	}

	// Add spatial data if locationid is provided, and update the user's last known location
	// (matching PHP behavior so that GET /isochrone can auto-create an isochrone for the user).
	if req.Locationid != nil && *req.Locationid > 0 {
		db.Exec("UPDATE users SET lastlocation = ? WHERE id = ?", *req.Locationid, myid)

		var lat, lng float64
		db.Raw("SELECT lat, lng FROM locations WHERE id = ?", *req.Locationid).Row().Scan(&lat, &lng)
		if lat != 0 || lng != 0 {
			db.Exec("INSERT INTO messages_spatial (msgid, point, successful, groupid, msgtype) VALUES (?, ST_GeomFromText(CONCAT('POINT(', ?, ' ', ?, ')'), 3857), 1, ?, ?)",
				newMsgID, lng, lat, req.Groupid, req.Type)
		}
	}

	resp := fiber.Map{"ret": 0, "status": "Success", "id": newMsgID}
	if jwtString != "" {
		resp["jwt"] = jwtString
		resp["persistent"] = persistent
	}
	return c.JSON(resp)
}
