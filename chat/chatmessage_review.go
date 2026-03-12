package chat

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// canSeeChatRoom checks if a user can view a chat room. Matches PHP ChatRoom::canSee().
// Allows: direct participants, moderators of the chat's group, and moderators of any group
// where either participant is a member (for User2User chats during review).
func canSeeChatRoom(myid uint64, user1, user2, groupid uint64) bool {
	if user1 == myid || user2 == myid {
		return true
	}

	db := database.DBConn

	if groupid > 0 {
		var modCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND role IN ('Moderator', 'Owner')",
			myid, groupid).Scan(&modCount)
		if modCount > 0 {
			return true
		}
	}

	// Fallback: check if mod of any group where either participant is a member.
	var modCount int64
	db.Raw("SELECT COUNT(*) FROM memberships m1 "+
		"INNER JOIN memberships m2 ON m1.groupid = m2.groupid "+
		"WHERE m1.userid = ? AND m1.role IN ('Moderator', 'Owner') "+
		"AND m2.userid IN (?, ?)",
		myid, user1, user2).Scan(&modCount)
	return modCount > 0
}

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
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"ret": 2, "status": "Chat not found"})
	}

	if !canSeeChatRoom(myid, room.User1, room.User2, room.Groupid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
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
		ctxq = " AND cm.id < " + strconv.FormatUint(ctx, 10)
	}

	// Find messages pending review where either participant is in the mod's groups,
	// or the chat is a User2Mod chat for one of the mod's groups.
	type reviewRow struct {
		ID              uint64          `json:"id"`
		Chatid          uint64          `json:"chatid"`
		Userid          uint64          `json:"userid"`
		Type            string          `json:"type"`
		Message         string          `json:"message"`
		Date            *time.Time      `json:"date"`
		Refmsgid        *uint64         `json:"refmsgid"`
		Reportreason    *string         `json:"reportreason"`
		Imageid         *uint64         `json:"-"`
		ImageArchived   int             `json:"-"`
		Imageuid        string          `json:"-"`
		Imagemods       json.RawMessage `json:"-"`
		RoomChattype    string          `json:"-"`
		RoomUser1       uint64          `json:"-"`
		RoomUser2       uint64          `json:"-"`
		RoomGroupid     uint64          `json:"-"`
		Widerchatreview int             `json:"-"`
		HeldBy          uint64          `json:"-"`
		HeldTimestamp   *time.Time      `json:"-"`
		Msgid           *uint64         `json:"-"`
		Groupid         uint64          `json:"-"`
		Groupidfrom     uint64          `json:"-"`
	}

	// Check if this user participates in wider chat review.
	widerReview := user.HasWiderReview(myid)

	// Base query: messages from mod's own groups.
	baseQuery := "SELECT DISTINCT cm.id, cm.chatid, cm.userid, cm.type, cm.message, cm.date, " +
		"cm.refmsgid, cm.reportreason, " +
		"cm.imageid, COALESCE(ci.archived, 0) AS image_archived, " +
		"COALESCE(ci.externaluid, '') AS imageuid, ci.externalmods AS imagemods, " +
		"cr.chattype AS room_chattype, cr.user1 AS room_user1, cr.user2 AS room_user2, " +
		"COALESCE(cr.groupid, 0) AS room_groupid, " +
		"0 AS widerchatreview, " +
		"COALESCE(cmh.userid, 0) AS held_by, cmh.timestamp AS held_timestamp, " +
		"cme.msgid, " +
		"COALESCE((SELECT m1.groupid FROM memberships m1 WHERE m1.userid = CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END AND m1.groupid IN (" + groupIDList + ") LIMIT 1), 0) AS groupid, " +
		"COALESCE((SELECT m2.groupid FROM memberships m2 WHERE m2.userid = cm.userid AND m2.groupid IN (" + groupIDList + ") LIMIT 1), 0) AS groupidfrom " +
		"FROM chat_messages cm " +
		"INNER JOIN chat_rooms cr ON cr.id = cm.chatid " +
		"INNER JOIN users ON users.id = cm.userid AND users.deleted IS NULL " +
		"LEFT JOIN chat_images ci ON ci.chatmsgid = cm.id " +
		"LEFT JOIN chat_messages_held cmh ON cmh.msgid = cm.id " +
		"LEFT JOIN chat_messages_byemail cme ON cme.chatmsgid = cm.id " +
		"WHERE cm.reviewrequired = 1 AND cm.reviewrejected = 0" + ctxq +
		" AND (" +
		"  (cr.chattype = ? AND cr.groupid IN (" + groupIDList + "))" +
		"  OR (cr.chattype = ? AND (" +
		"    EXISTS (SELECT 1 FROM memberships WHERE userid = cr.user1 AND groupid IN (" + groupIDList + "))" +
		"    OR EXISTS (SELECT 1 FROM memberships WHERE userid = cr.user2 AND groupid IN (" + groupIDList + "))" +
		"  ))" +
		")"

	var msgs []reviewRow

	if widerReview {
		// Add UNION for wider chat review: messages from any group with widerchatreview=1,
		// excluding held messages and user-reported spam. Matches PHP ChatRoom::getMessagesForReview().
		widerQuery := " UNION " +
			"SELECT DISTINCT cm.id, cm.chatid, cm.userid, cm.type, cm.message, cm.date, " +
			"cm.refmsgid, cm.reportreason, " +
			"cm.imageid, COALESCE(ci.archived, 0) AS image_archived, " +
			"COALESCE(ci.externaluid, '') AS imageuid, ci.externalmods AS imagemods, " +
			"cr.chattype AS room_chattype, cr.user1 AS room_user1, cr.user2 AS room_user2, " +
			"COALESCE(cr.groupid, 0) AS room_groupid, " +
			"1 AS widerchatreview, " +
			"COALESCE(cmh.userid, 0) AS held_by, cmh.timestamp AS held_timestamp, " +
			"cme.msgid, " +
			"m1.groupid AS groupid, " +
			"COALESCE(m2.groupid, 0) AS groupidfrom " +
			"FROM chat_messages cm " +
			"INNER JOIN chat_rooms cr ON cr.id = cm.chatid AND cm.reviewrequired = 1 AND cm.reviewrejected = 0 " +
			"INNER JOIN memberships m1 ON m1.userid = (CASE WHEN cm.userid = cr.user1 THEN cr.user2 ELSE cr.user1 END) " +
			"INNER JOIN `groups` g ON m1.groupid = g.id AND g.type = 'Freegle' " +
			"INNER JOIN users ON users.id = cm.userid AND users.deleted IS NULL " +
			"LEFT JOIN memberships m2 ON m2.userid = cm.userid " +
			"LEFT JOIN chat_images ci ON ci.chatmsgid = cm.id " +
			"LEFT JOIN chat_messages_held cmh ON cmh.msgid = cm.id " +
			"LEFT JOIN chat_messages_byemail cme ON cme.chatmsgid = cm.id " +
			"WHERE JSON_EXTRACT(g.settings, '$.widerchatreview') = 1 " +
			"AND cmh.id IS NULL " +
			"AND (cm.reportreason IS NULL OR cm.reportreason != 'User')" + ctxq

		db.Raw("SELECT * FROM ("+baseQuery+widerQuery+") combined ORDER BY id ASC LIMIT ?",
			utils.CHAT_TYPE_USER2MOD, utils.CHAT_TYPE_USER2USER, limit).Scan(&msgs)
	} else {
		db.Raw(baseQuery+" ORDER BY cm.id ASC LIMIT ?",
			utils.CHAT_TYPE_USER2MOD, utils.CHAT_TYPE_USER2USER, limit).Scan(&msgs)
	}

	if msgs == nil {
		msgs = []reviewRow{}
	}

	// Collect held-by user IDs for batch fetching.
	heldByUserIDs := make(map[uint64]bool)
	for _, m := range msgs {
		if m.HeldBy > 0 {
			heldByUserIDs[m.HeldBy] = true
		}
	}

	// Fetch held-by user details (name, email) if any.
	type heldUserInfo struct {
		ID    uint64
		Name  string
		Email string
	}
	heldUsers := make(map[uint64]heldUserInfo)
	if len(heldByUserIDs) > 0 {
		ids := make([]string, 0, len(heldByUserIDs))
		for id := range heldByUserIDs {
			ids = append(ids, strconv.FormatUint(id, 10))
		}
		var heldInfos []heldUserInfo
		db.Raw("SELECT u.id, u.fullname AS name, "+
			"(SELECT e.email FROM users_emails e WHERE e.userid = u.id AND e.preferred = 1 LIMIT 1) AS email "+
			"FROM users u WHERE u.id IN ("+strings.Join(ids, ",")+")").Scan(&heldInfos)
		for _, h := range heldInfos {
			heldUsers[h.ID] = h
		}
	}

	// Build response with inline chatroom info.
	result := make([]fiber.Map, 0, len(msgs))
	for _, m := range msgs {
		name := getChatName(db, m.RoomChattype, m.RoomGroupid, m.RoomUser1, m.RoomUser2, myid)

		// Determine fromuser (sender) and touser (other participant).
		fromuserid := m.Userid
		var touserid uint64
		if m.Userid == m.RoomUser1 {
			touserid = m.RoomUser2
		} else {
			touserid = m.RoomUser1
		}

		msg := fiber.Map{
			"id":              m.ID,
			"chatid":          m.Chatid,
			"userid":          m.Userid,
			"fromuserid":      fromuserid,
			"touserid":        touserid,
			"type":            m.Type,
			"message":         m.Message,
			"date":            m.Date,
			"refmsgid":        m.Refmsgid,
			"reviewreason":    m.Reportreason,
			"widerchatreview": m.Widerchatreview > 0,
			"groupid":         m.Groupid,
			"groupidfrom":     m.Groupidfrom,
			"chatroom": fiber.Map{
				"id":       m.Chatid,
				"chattype": m.RoomChattype,
				"user1":    m.RoomUser1,
				"user2":    m.RoomUser2,
				"groupid":  m.RoomGroupid,
				"name":     name,
			},
		}

		// Add image if the message has one.
		if m.Imageid != nil {
			var image *ChatAttachment
			if m.Imageuid != "" {
				image = &ChatAttachment{
					ID:           *m.Imageid,
					Ouruid:       m.Imageuid,
					Externalmods: m.Imagemods,
					Path:         misc.GetImageDeliveryUrl(m.Imageuid, string(m.Imagemods)),
					Paththumb:    misc.GetImageDeliveryUrl(m.Imageuid, string(m.Imagemods)),
				}
			} else if m.ImageArchived > 0 {
				image = &ChatAttachment{
					ID:        *m.Imageid,
					Path:      "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/mimg_" + strconv.FormatUint(*m.Imageid, 10) + ".jpg",
					Paththumb: "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tmimg_" + strconv.FormatUint(*m.Imageid, 10) + ".jpg",
				}
			} else {
				image = &ChatAttachment{
					ID:        *m.Imageid,
					Path:      "https://" + os.Getenv("IMAGE_DOMAIN") + "/mimg_" + strconv.FormatUint(*m.Imageid, 10) + ".jpg",
					Paththumb: "https://" + os.Getenv("IMAGE_DOMAIN") + "/tmimg_" + strconv.FormatUint(*m.Imageid, 10) + ".jpg",
				}
			}
			msg["image"] = image
			msg["imageid"] = *m.Imageid
		}

		// Add msgid if the message came via email.
		if m.Msgid != nil {
			msg["msgid"] = *m.Msgid
		}

		// Add held info if message is held by a moderator.
		if m.HeldBy > 0 {
			held := fiber.Map{
				"id": m.HeldBy,
			}
			if m.HeldTimestamp != nil {
				held["timestamp"] = m.HeldTimestamp
			}
			if h, ok := heldUsers[m.HeldBy]; ok {
				held["name"] = h.Name
				held["email"] = h.Email
			}
			msg["held"] = held
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
