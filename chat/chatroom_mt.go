package chat

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const chatActiveLimitMT = 365

// parseChattypes extracts chattypes from query parameters.
// Handles array format (?chattypes[]=X&chattypes[]=Y) and single value.
func parseChattypes(c *fiber.Ctx) []string {
	// Try array format first (chattypes[]=X&chattypes[]=Y)
	vals := c.Context().QueryArgs().PeekMulti("chattypes[]")
	if len(vals) > 0 {
		result := make([]string, len(vals))
		for i, v := range vals {
			result[i] = string(v)
		}
		return result
	}

	// Try single value or comma-separated
	ct := c.Query("chattypes", "")
	if ct != "" {
		return strings.Split(ct, ",")
	}

	return []string{utils.CHAT_TYPE_USER2USER}
}

// GetChatRoomsMT handles GET /chatrooms for moderator operations.
//
// @Summary Get chatrooms for moderator
// @Tags chat
// @Produce json
// @Param count query boolean false "Return unseen count only"
// @Param id query integer false "Single chatroom ID"
// @Param chattypes query string false "Chat types"
// @Success 200 {object} map[string]interface{}
// @Router /api/chatrooms [get]
func GetChatRoomsMT(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	countMode := c.QueryBool("count", false)
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	chattypes := parseChattypes(c)

	if countMode {
		return countUnseenMT(c, myid, chattypes)
	}

	if id > 0 {
		return fetchSingleChatMT(c, myid, id)
	}

	// Listing is handled by ListForUserMT via /chat/rooms.
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"ret": 3, "status": "Use /chat/rooms for listing"})
}

// countUnseenMT returns the count of unseen messages across moderator chats.
func countUnseenMT(c *fiber.Ctx, myid uint64, chattypes []string) error {
	db := database.DBConn

	chatIDs := getModeratorChatIDs(db, myid, chattypes, "", 0)
	if len(chatIDs) == 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "count": 0})
	}

	idlist := joinIDs(chatIDs)
	activeSince := time.Now().AddDate(0, 0, -chatActiveLimitMT).Format("2006-01-02")

	var count int64
	db.Raw("SELECT COUNT(*) FROM chat_messages "+
		"INNER JOIN users ON users.id = chat_messages.userid "+
		"LEFT JOIN chat_roster ON chat_roster.chatid = chat_messages.chatid AND chat_roster.userid = ? "+
		"WHERE chat_messages.chatid IN ("+idlist+") "+
		"AND chat_messages.userid != ? "+
		"AND chat_messages.reviewrequired = 0 "+
		"AND chat_messages.reviewrejected = 0 "+
		"AND chat_messages.processingsuccessful = 1 "+
		"AND chat_messages.id > COALESCE(chat_roster.lastmsgseen, 0) "+
		"AND chat_messages.date >= ? "+
		"AND users.deleted IS NULL",
		myid, myid, activeSince).Scan(&count)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "count": count})
}

// fetchSingleChatMT returns a single chatroom with unseen count.
func fetchSingleChatMT(c *fiber.Ctx, myid uint64, id uint64) error {
	db := database.DBConn

	type roomInfo struct {
		ID            uint64     `json:"id"`
		Chattype      string     `json:"chattype"`
		User1         uint64     `json:"user1"`
		User2         uint64     `json:"user2"`
		Groupid       uint64     `json:"groupid"`
		Latestmessage *time.Time `json:"latestmessage"`
	}

	var room roomInfo
	db.Raw("SELECT id, chattype, user1, user2, COALESCE(groupid, 0) AS groupid, latestmessage FROM chat_rooms WHERE id = ?", id).Scan(&room)

	if room.ID == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"ret": 2, "status": "Chat not found"})
	}

	// Check permissions: participant or moderator of the group.
	canSee := room.User1 == myid || room.User2 == myid
	if !canSee && room.Groupid > 0 {
		var modCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND role IN ('Moderator', 'Owner')",
			myid, room.Groupid).Scan(&modCount)
		canSee = modCount > 0
	}

	if !canSee {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	// Get unseen count and last seen in parallel.
	var unseen int64
	var lastmsgseen *uint64

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		db.Raw("SELECT COUNT(*) FROM chat_messages "+
			"WHERE chatid = ? AND userid != ? "+
			"AND id > COALESCE((SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?), 0) "+
			"AND reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1",
			id, myid, id, myid).Scan(&unseen)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?", id, myid).Scan(&lastmsgseen)
	}()
	wg.Wait()

	// Get name based on chat type.
	name := getChatName(db, room.Chattype, room.Groupid, room.User1, room.User2, myid)

	otheruid := uint64(0)
	if room.User1 == myid {
		otheruid = room.User2
	} else if room.User2 == myid {
		otheruid = room.User1
	}

	chatroom := fiber.Map{
		"id":          room.ID,
		"chattype":    room.Chattype,
		"groupid":     room.Groupid,
		"user1":       room.User1,
		"user2":       room.User2,
		"unseen":      unseen,
		"lastmsgseen": lastmsgseen,
		"name":        name,
		"otheruid":    otheruid,
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "chatroom": chatroom})
}

// getModeratorChatIDs returns chat room IDs visible to a moderator for the given chat types.
func getModeratorChatIDs(db *gorm.DB, myid uint64, chattypes []string, search string, age int) []uint64 {
	activeDays := chatActiveLimitMT
	if age > 0 {
		activeDays = age
	}
	activeSince := time.Now().AddDate(0, 0, -activeDays).Format("2006-01-02")

	var allIDs []uint64

	for _, ct := range chattypes {
		var ids []uint64

		switch ct {
		case utils.CHAT_TYPE_MOD2MOD:
			db.Raw("SELECT DISTINCT chat_rooms.id FROM chat_rooms "+
				"INNER JOIN memberships ON chat_rooms.groupid = memberships.groupid "+
				"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid "+
				"WHERE memberships.userid = ? AND memberships.role IN ('Moderator', 'Owner') "+
				"AND chat_rooms.chattype = ? "+
				"AND (chat_roster.status IS NULL OR chat_roster.status != ?) "+
				"AND chat_rooms.latestmessage >= ?",
				myid, myid, utils.CHAT_TYPE_MOD2MOD, utils.CHAT_STATUS_CLOSED, activeSince).Scan(&ids)

		case utils.CHAT_TYPE_USER2MOD:
			db.Raw("SELECT DISTINCT chat_rooms.id FROM chat_rooms "+
				"INNER JOIN memberships ON chat_rooms.groupid = memberships.groupid "+
				"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid "+
				"WHERE (memberships.userid = ? AND memberships.role IN ('Moderator', 'Owner') OR chat_rooms.user1 = ?) "+
				"AND chat_rooms.chattype = ? "+
				"AND (chat_roster.status IS NULL OR chat_roster.status != ?) "+
				"AND chat_rooms.latestmessage >= ?",
				myid, myid, myid, utils.CHAT_TYPE_USER2MOD, utils.CHAT_STATUS_CLOSED, activeSince).Scan(&ids)
		}

		allIDs = append(allIDs, ids...)
	}

	// Apply search filter if provided.
	if search != "" && len(allIDs) > 0 {
		idlist := joinIDs(allIDs)
		var filteredIDs []uint64
		db.Raw("SELECT DISTINCT chat_rooms.id FROM chat_rooms "+
			"LEFT JOIN chat_messages ON chat_messages.chatid = chat_rooms.id "+
			"LEFT JOIN users u1 ON u1.id = chat_rooms.user1 "+
			"LEFT JOIN users u2 ON u2.id = chat_rooms.user2 "+
			"WHERE chat_rooms.id IN ("+idlist+") "+
			"AND (chat_messages.message LIKE ? OR u1.fullname LIKE ? OR u2.fullname LIKE ? "+
			"OR u1.firstname LIKE ? OR u2.firstname LIKE ?)",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%").Scan(&filteredIDs)
		return filteredIDs
	}

	return allIDs
}

// getChatName returns a display name for a chat room based on type.
// For User2Mod chats, returns the member's name (not the group name) so mods
// can see who they're chatting with in the ModTools chat list.
func getChatName(db *gorm.DB, chattype string, groupid uint64, user1 uint64, user2 uint64, myid uint64) string {
	switch chattype {
	case utils.CHAT_TYPE_USER2MOD:
		// Show the member's name (user1) to the moderator.
		if user1 > 0 {
			var fullname string
			db.Raw("SELECT fullname FROM users WHERE id = ?", user1).Scan(&fullname)
			if fullname != "" {
				return fullname
			}
		}
	case utils.CHAT_TYPE_MOD2MOD:
		if groupid > 0 {
			var nameshort string
			db.Raw("SELECT nameshort FROM `groups` WHERE id = ?", groupid).Scan(&nameshort)
			if nameshort != "" {
				return nameshort + " Mods"
			}
		}
	default:
		otheruid := user2
		if user1 != myid {
			otheruid = user1
		}
		if otheruid > 0 {
			var fullname string
			db.Raw("SELECT fullname FROM users WHERE id = ?", otheruid).Scan(&fullname)
			if fullname != "" {
				return fullname
			}
		}
	}
	return ""
}

// joinIDs converts a slice of uint64 to a comma-separated string for SQL IN clauses.
func joinIDs(ids []uint64) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.FormatUint(id, 10)
	}
	return strings.Join(strs, ",")
}
