package chat

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"strconv"
	"time"
)

type ChatRoomPostRequest struct {
	ID          uint64 `json:"id"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	Lastmsgseen uint64 `json:"lastmsgseen"`
	Allowback   bool   `json:"allowback"`
}

type RosterEntry struct {
	Userid uint64 `json:"userid"`
	Status string `json:"status"`
}

// PostChatRoom handles POST /chatrooms - roster updates, nudge, typing actions
func PostChatRoom(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req ChatRoomPostRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	switch req.Action {
	case "Nudge":
		return handleNudge(c, db, myid, req.ID)
	case "Typing":
		return handleTyping(c, db, myid, req.ID)
	default:
		if req.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "Chat ID required")
		}
		return handleRosterUpdate(c, db, myid, req)
	}
}

func handleNudge(c *fiber.Ctx, db *gorm.DB, myid uint64, chatid uint64) error {
	if chatid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Chat ID required")
	}

	// Verify user is a member of this chat
	var room ChatRoom
	db.Raw("SELECT id, chattype, user1, user2 FROM chat_rooms WHERE id = ?", chatid).Scan(&room)
	if room.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Chat not found")
	}
	if room.User1 != myid && room.User2 != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not a member of this chat")
	}

	// Check last message - only create nudge if it's not already a nudge from this user
	type lastMsg struct {
		Type   string
		Userid uint64
	}
	var last lastMsg
	db.Raw("SELECT type, userid FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatid).Scan(&last)

	if last.Type == utils.CHAT_MESSAGE_NUDGE && last.Userid == myid {
		// Already nudged - return the existing nudge ID
		var existingId uint64
		db.Raw("SELECT id FROM chat_messages WHERE chatid = ? AND type = ? AND userid = ? ORDER BY id DESC LIMIT 1",
			chatid, utils.CHAT_MESSAGE_NUDGE, myid).Scan(&existingId)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": existingId})
	}

	// Create nudge message
	now := time.Now()
	result := db.Exec(
		"INSERT INTO chat_messages (chatid, userid, type, date, message, replyexpected, reportreason, reviewrequired, reviewrejected, processingsuccessful) VALUES (?, ?, ?, ?, '', 1, NULL, 0, 0, 1)",
		chatid, myid, utils.CHAT_MESSAGE_NUDGE, now,
	)
	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create nudge")
	}

	var newId uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newId)

	// Update latestmessage on chat room
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatid)

	// Record nudge for analytics
	db.Exec("INSERT INTO users_nudges (fromuser, touser) VALUES (?, ?)",
		myid, getOtherUser(room, myid))

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newId})
}

func handleTyping(c *fiber.Ctx, db *gorm.DB, myid uint64, chatid uint64) error {
	if chatid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Chat ID required")
	}

	// Bump date on recent unmailed messages to delay email batching.
	// This batches multiple chat messages into a single email when user is actively typing.
	// PHP uses DELAY = 30 seconds.
	result := db.Exec("UPDATE chat_messages SET date = NOW() WHERE chatid = ? AND TIMESTAMPDIFF(SECOND, chat_messages.date, NOW()) < 30 AND mailedtoall = 0",
		chatid)
	count := result.RowsAffected

	// Record the last typing time in roster.
	db.Exec("UPDATE chat_roster SET lasttype = NOW() WHERE chatid = ? AND userid = ?", chatid, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "count": count})
}

func handleRosterUpdate(c *fiber.Ctx, db *gorm.DB, myid uint64, req ChatRoomPostRequest) error {
	// Verify user can see this chat and is a legitimate member
	var room ChatRoom
	db.Raw("SELECT id, chattype, user1, user2 FROM chat_rooms WHERE id = ?", req.ID).Scan(&room)
	if room.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": strconv.FormatUint(req.ID, 10) + " Not visible to you"})
	}

	// Permission check - prevent client bugs from inserting mods into User2User chats
	canUpdate := false
	switch room.Chattype {
	case utils.CHAT_TYPE_USER2MOD, utils.CHAT_TYPE_GROUP, utils.CHAT_TYPE_MOD2MOD:
		canUpdate = true
	case utils.CHAT_TYPE_USER2USER:
		canUpdate = (myid == room.User1 || myid == room.User2)
	}

	if !canUpdate {
		return c.JSON(fiber.Map{"ret": 2, "status": strconv.FormatUint(req.ID, 10) + " Not visible to you"})
	}

	// Determine status - default to Online if not specified
	status := req.Status
	if status == "" {
		status = utils.CHAT_STATUS_ONLINE
	}

	// Get user's IP for tracking
	ip := c.IP()

	// Insert or update roster entry
	if status == utils.CHAT_STATUS_BLOCKED {
		db.Exec(
			"INSERT INTO chat_roster (chatid, userid, status, lastip, date) VALUES (?, ?, ?, ?, NOW()) "+
				"ON DUPLICATE KEY UPDATE status = ?, lastip = ?, date = NOW()",
			req.ID, myid, status, ip, status, ip,
		)
	} else if status == utils.CHAT_STATUS_CLOSED {
		// Don't overwrite BLOCKED with CLOSED
		db.Exec(
			"INSERT INTO chat_roster (chatid, userid, status, lastip, date) VALUES (?, ?, ?, ?, NOW()) "+
				"ON DUPLICATE KEY UPDATE status = IF(status = '"+utils.CHAT_STATUS_BLOCKED+"', status, ?), lastip = ?, date = NOW()",
			req.ID, myid, status, ip, status, ip,
		)
	} else {
		db.Exec(
			"INSERT INTO chat_roster (chatid, userid, status, lastip, date) VALUES (?, ?, ?, ?, NOW()) "+
				"ON DUPLICATE KEY UPDATE status = ?, lastip = ?, date = NOW()",
			req.ID, myid, status, ip, status, ip,
		)
	}

	// Update lastmsgseen if provided
	if req.Lastmsgseen > 0 {
		if req.Allowback {
			db.Exec("UPDATE chat_roster SET lastmsgseen = ? WHERE chatid = ? AND userid = ?",
				req.Lastmsgseen, req.ID, myid)
		} else {
			// Only update forward (don't go backwards)
			db.Exec("UPDATE chat_roster SET lastmsgseen = ? WHERE chatid = ? AND userid = ? AND (lastmsgseen IS NULL OR lastmsgseen < ?)",
				req.Lastmsgseen, req.ID, myid, req.Lastmsgseen)
		}

		// Check if message has been seen by all roster members
		db.Exec(`UPDATE chat_messages SET seenbyall = 1
			WHERE chatid = ? AND id <= ? AND seenbyall = 0
			AND NOT EXISTS (
				SELECT 1 FROM chat_roster
				WHERE chatid = ? AND (lastmsgseen IS NULL OR lastmsgseen < chat_messages.id)
				AND userid != chat_messages.userid
			)`, req.ID, req.Lastmsgseen, req.ID)
	}

	// Get updated roster
	var roster []RosterEntry
	db.Raw("SELECT userid, status FROM chat_roster WHERE chatid = ?", req.ID).Scan(&roster)

	// Get unseen count
	var unseen int64
	db.Raw(`SELECT COUNT(*) FROM chat_messages
		WHERE chatid = ? AND userid != ?
		AND id > COALESCE((SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?), 0)
		AND reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1`,
		req.ID, myid, req.ID, myid).Scan(&unseen)

	if roster == nil {
		roster = make([]RosterEntry, 0)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"roster": roster,
		"unseen": unseen,
		"nolog":  true,
	})
}

func getOtherUser(room ChatRoom, myid uint64) uint64 {
	if room.User1 == myid {
		return room.User2
	}
	return room.User1
}
