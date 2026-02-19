package chat

import (
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// ReviewChatMessage represents a chat message in the review queue with associated room info.
type ReviewChatMessage struct {
	ID           uint64     `json:"id"`
	Chatid       uint64     `json:"chatid"`
	Userid       uint64     `json:"userid"`
	Type         string     `json:"type"`
	Message      string     `json:"message"`
	Date         *time.Time `json:"date"`
	Refmsgid     *uint64    `json:"refmsgid"`
	Reportreason *string    `json:"reportreason"`
	Chatroom     fiber.Map  `json:"chatroom"`
}

// GetReviewChatMessages handles GET /chatmessages for moderator review queue.
// When called without a roomid, returns messages pending review for the moderator's groups.
//
// @Summary Get chat messages for review
// @Tags chat
// @Produce json
// @Param roomid query integer false "Chat room ID (for room-specific messages)"
// @Param limit query integer false "Max messages to return"
// @Param context query integer false "Cursor for pagination (last message ID)"
// @Param groupid query integer false "Filter to specific group"
// @Success 200 {object} map[string]interface{}
// @Router /api/chatmessages [get]
func GetReviewChatMessages(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	roomid, _ := strconv.ParseUint(c.Query("roomid", "0"), 10, 64)

	// If roomid is provided, return messages for that specific room.
	if roomid > 0 {
		return getChatMessagesForRoom(c, myid, roomid)
	}

	// Otherwise, return the review queue for this moderator.
	return getReviewQueue(c, myid)
}

// getChatMessagesForRoom returns messages from a specific chat room (for MT viewing).
func getChatMessagesForRoom(c *fiber.Ctx, myid uint64, roomid uint64) error {
	db := database.DBConn

	// Verify user can access this chat (participant or moderator of group).
	type roomCheck struct {
		ID       uint64
		User1    uint64
		User2    uint64
		Groupid  uint64
		Chattype string
	}
	var room roomCheck
	db.Raw("SELECT id, user1, user2, COALESCE(groupid, 0) AS groupid, chattype FROM chat_rooms WHERE id = ?", roomid).Scan(&room)

	if room.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Chat not found"})
	}

	canSee := room.User1 == myid || room.User2 == myid
	if !canSee && room.Groupid > 0 {
		var modCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND role IN ('Moderator', 'Owner')",
			myid, room.Groupid).Scan(&modCount)
		canSee = modCount > 0
	}

	if !canSee {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	ctx, _ := strconv.ParseUint(c.Query("context", "0"), 10, 64)

	type msgRow struct {
		ID           uint64     `json:"id"`
		Chatid       uint64     `json:"chatid"`
		Userid       uint64     `json:"userid"`
		Type         string     `json:"type"`
		Message      string     `json:"message"`
		Date         *time.Time `json:"date"`
		Refmsgid     *uint64    `json:"refmsgid"`
		Reportreason *string    `json:"reportreason"`
	}

	ctxq := ""
	if ctx > 0 {
		ctxq = " AND chat_messages.id < " + strconv.FormatUint(ctx, 10)
	}

	var msgs []msgRow
	db.Raw("SELECT chat_messages.id, chat_messages.chatid, chat_messages.userid, "+
		"chat_messages.type, chat_messages.message, chat_messages.date, "+
		"chat_messages.refmsgid, chat_messages.reportreason "+
		"FROM chat_messages "+
		"INNER JOIN users ON users.id = chat_messages.userid "+
		"WHERE chat_messages.chatid = ? "+
		"AND (chat_messages.userid = ? OR (chat_messages.reviewrequired = 0 AND chat_messages.reviewrejected = 0 AND chat_messages.processingsuccessful = 1)) "+
		"AND users.deleted IS NULL"+ctxq+
		" ORDER BY chat_messages.id DESC LIMIT ?",
		roomid, myid, limit).Scan(&msgs)

	if msgs == nil {
		msgs = []msgRow{}
	}

	// Build context for pagination.
	newCtx := fiber.Map{}
	if len(msgs) > 0 {
		newCtx["id"] = msgs[len(msgs)-1].ID
	}

	return c.JSON(fiber.Map{
		"ret":          0,
		"status":       "Success",
		"chatmessages": msgs,
		"context":      newCtx,
	})
}

// getReviewQueue returns chat messages pending moderation review.
func getReviewQueue(c *fiber.Ctx, myid uint64) error {
	db := database.DBConn

	// Get groups where user is a moderator.
	var groupIDs []uint64
	db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&groupIDs)

	if len(groupIDs) == 0 {
		return c.JSON(fiber.Map{
			"ret":          0,
			"status":       "Success",
			"chatmessages": make([]interface{}, 0),
			"chatreports":  make([]interface{}, 0),
			"context":      fiber.Map{},
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	ctx, _ := strconv.ParseUint(c.Query("context", "0"), 10, 64)

	groupIDStr := make([]string, len(groupIDs))
	for i, gid := range groupIDs {
		groupIDStr[i] = strconv.FormatUint(gid, 10)
	}
	groupIDList := strings.Join(groupIDStr, ",")

	ctxq := ""
	if ctx > 0 {
		ctxq = " AND cm.id > " + strconv.FormatUint(ctx, 10)
	}

	// Find messages pending review where either participant is in the mod's groups,
	// or the chat is a User2Mod chat for one of the mod's groups.
	type reviewRow struct {
		ID           uint64     `json:"id"`
		Chatid       uint64     `json:"chatid"`
		Userid       uint64     `json:"userid"`
		Type         string     `json:"type"`
		Message      string     `json:"message"`
		Date         *time.Time `json:"date"`
		Refmsgid     *uint64    `json:"refmsgid"`
		Reportreason *string    `json:"reportreason"`
		RoomChattype string     `json:"-"`
		RoomUser1    uint64     `json:"-"`
		RoomUser2    uint64     `json:"-"`
		RoomGroupid  uint64     `json:"-"`
	}

	var msgs []reviewRow
	db.Raw("SELECT DISTINCT cm.id, cm.chatid, cm.userid, cm.type, cm.message, cm.date, "+
		"cm.refmsgid, cm.reportreason, "+
		"cr.chattype AS room_chattype, cr.user1 AS room_user1, cr.user2 AS room_user2, "+
		"COALESCE(cr.groupid, 0) AS room_groupid "+
		"FROM chat_messages cm "+
		"INNER JOIN chat_rooms cr ON cr.id = cm.chatid "+
		"INNER JOIN users ON users.id = cm.userid AND users.deleted IS NULL "+
		"WHERE cm.reviewrequired = 1 AND cm.reviewrejected = 0"+ctxq+
		" AND ("+
		"  (cr.chattype = ? AND cr.groupid IN ("+groupIDList+"))"+
		"  OR (cr.chattype = ? AND ("+
		"    EXISTS (SELECT 1 FROM memberships WHERE userid = cr.user1 AND groupid IN ("+groupIDList+"))"+
		"    OR EXISTS (SELECT 1 FROM memberships WHERE userid = cr.user2 AND groupid IN ("+groupIDList+"))"+
		"  ))"+
		") "+
		"ORDER BY cm.id ASC LIMIT ?",
		utils.CHAT_TYPE_USER2MOD, utils.CHAT_TYPE_USER2USER, limit).Scan(&msgs)

	if msgs == nil {
		msgs = []reviewRow{}
	}

	// Build response with inline chatroom info.
	result := make([]fiber.Map, 0, len(msgs))
	for _, m := range msgs {
		name := getChatName(db, m.RoomChattype, m.RoomGroupid, m.RoomUser1, m.RoomUser2, myid)

		msg := fiber.Map{
			"id":           m.ID,
			"chatid":       m.Chatid,
			"userid":       m.Userid,
			"type":         m.Type,
			"message":      m.Message,
			"date":         m.Date,
			"refmsgid":     m.Refmsgid,
			"reportreason": m.Reportreason,
			"chatroom": fiber.Map{
				"id":       m.Chatid,
				"chattype": m.RoomChattype,
				"user1":    m.RoomUser1,
				"user2":    m.RoomUser2,
				"groupid":  m.RoomGroupid,
				"name":     name,
			},
		}
		result = append(result, msg)
	}

	// Build context for pagination.
	newCtx := fiber.Map{}
	if len(msgs) > 0 {
		newCtx["id"] = msgs[len(msgs)-1].ID
	}

	return c.JSON(fiber.Map{
		"ret":          0,
		"status":       "Success",
		"chatmessages": result,
		"chatreports":  make([]interface{}, 0),
		"context":      newCtx,
	})
}
