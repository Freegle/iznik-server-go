package chat

import (
	"os"
	"regexp"
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
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
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

	return doListChatRoomsMT(c, myid, chattypes, "", 0)
}

// ListChatRoomsMT handles GET /chat/rooms for moderator chat listing.
//
// @Summary List chatrooms for moderator
// @Tags chat
// @Produce json
// @Param chattypes query string false "Chat types filter"
// @Param search query string false "Search term"
// @Param age query integer false "Max age in days"
// @Success 200 {object} map[string]interface{}
// @Router /api/chat/rooms [get]
func ListChatRoomsMT(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	chattypes := parseChattypes(c)
	search := c.Query("search", "")
	age, _ := strconv.Atoi(c.Query("age", "0"))

	return doListChatRoomsMT(c, myid, chattypes, search, age)
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
		return c.JSON(fiber.Map{"ret": 2, "status": "Chat not found"})
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
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
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

// doListChatRoomsMT returns a list of moderator chat rooms with enrichment.
func doListChatRoomsMT(c *fiber.Ctx, myid uint64, chattypes []string, search string, age int) error {
	db := database.DBConn

	chatIDs := getModeratorChatIDs(db, myid, chattypes, search, age)
	if len(chatIDs) == 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "chatrooms": make([]interface{}, 0)})
	}

	idlist := joinIDs(chatIDs)

	// Fetch room details.
	type roomRow struct {
		ID            uint64     `json:"id"`
		Chattype      string     `json:"chattype"`
		User1         uint64     `json:"user1"`
		User2         uint64     `json:"user2"`
		Groupid       uint64     `json:"groupid"`
		Latestmessage *time.Time `json:"latestmessage"`
		Nameshort     string     `json:"-"`
		Namefull      string     `json:"-"`
	}

	var rooms []roomRow
	db.Raw("SELECT chat_rooms.id, chat_rooms.chattype, chat_rooms.user1, chat_rooms.user2, "+
		"COALESCE(chat_rooms.groupid, 0) AS groupid, chat_rooms.latestmessage, "+
		"COALESCE(g.nameshort, '') AS nameshort, COALESCE(g.namefull, '') AS namefull "+
		"FROM chat_rooms "+
		"LEFT JOIN `groups` g ON g.id = chat_rooms.groupid "+
		"WHERE chat_rooms.id IN ("+idlist+") "+
		"ORDER BY chat_rooms.latestmessage DESC").Scan(&rooms)

	if len(rooms) == 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "chatrooms": make([]interface{}, 0)})
	}

	// Enrichment data types.
	type unseenEntry struct {
		Chatid uint64
		Unseen int64
	}
	type lastMsgEntry struct {
		Chatid      uint64
		Lastmsg     uint64
		Lastdate    *time.Time
		Chatmsg     string
		Chatmsgtype string
		Refmsgtype  string
	}
	type lastSeenEntry struct {
		Chatid      uint64
		Lastmsgseen uint64
	}
	type userNameEntry struct {
		ID        uint64
		Firstname string
		Lastname  string
		Fullname  string
	}

	var unseens []unseenEntry
	var lastMsgs []lastMsgEntry
	var lastSeens []lastSeenEntry
	userNames := map[uint64]string{}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Get unseen counts per chat.
	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT chat_messages.chatid, COUNT(*) AS unseen FROM chat_messages "+
			"LEFT JOIN chat_roster ON chat_roster.chatid = chat_messages.chatid AND chat_roster.userid = ? "+
			"INNER JOIN users ON users.id = chat_messages.userid "+
			"WHERE chat_messages.chatid IN ("+idlist+") "+
			"AND chat_messages.userid != ? "+
			"AND chat_messages.id > COALESCE(chat_roster.lastmsgseen, 0) "+
			"AND chat_messages.reviewrequired = 0 AND chat_messages.reviewrejected = 0 "+
			"AND chat_messages.processingsuccessful = 1 "+
			"AND users.deleted IS NULL "+
			"GROUP BY chat_messages.chatid",
			myid, myid).Scan(&unseens)
	}()

	// Get last message per chat.
	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT cm.chatid, cm.id AS lastmsg, cm.date AS lastdate, cm.message AS chatmsg, "+
			"cm.type AS chatmsgtype, COALESCE(m.type, '') AS refmsgtype "+
			"FROM chat_messages cm "+
			"LEFT JOIN messages m ON m.id = cm.refmsgid "+
			"WHERE cm.id IN (SELECT MAX(id) FROM chat_messages WHERE chatid IN ("+idlist+") "+
			"AND reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1 "+
			"GROUP BY chatid)").Scan(&lastMsgs)
	}()

	// Get last seen per chat.
	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT chatid, COALESCE(lastmsgseen, 0) AS lastmsgseen FROM chat_roster "+
			"WHERE chatid IN ("+idlist+") AND userid = ?", myid).Scan(&lastSeens)
	}()

	// Get user names for other participants.
	wg.Add(1)
	go func() {
		defer wg.Done()
		userIDs := map[uint64]bool{}
		for _, r := range rooms {
			if r.User1 > 0 && r.User1 != myid {
				userIDs[r.User1] = true
			}
			if r.User2 > 0 && r.User2 != myid {
				userIDs[r.User2] = true
			}
		}
		if len(userIDs) > 0 {
			ids := []string{}
			for uid := range userIDs {
				ids = append(ids, strconv.FormatUint(uid, 10))
			}
			var users []userNameEntry
			db.Raw("SELECT id, firstname, lastname, fullname FROM users WHERE id IN (" + strings.Join(ids, ",") + ")").Scan(&users)
			mu.Lock()
			for _, u := range users {
				if u.Fullname != "" {
					userNames[u.ID] = u.Fullname
				} else {
					userNames[u.ID] = strings.TrimSpace(u.Firstname + " " + u.Lastname)
				}
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	// Build lookup maps.
	unseenMap := map[uint64]int64{}
	for _, u := range unseens {
		unseenMap[u.Chatid] = u.Unseen
	}
	lastMsgMap := map[uint64]lastMsgEntry{}
	for _, m := range lastMsgs {
		lastMsgMap[m.Chatid] = m
	}
	lastSeenMap := map[uint64]uint64{}
	for _, s := range lastSeens {
		lastSeenMap[s.Chatid] = s.Lastmsgseen
	}

	tnre := regexp.MustCompile(utils.TN_REGEXP)

	// Build response.
	chatrooms := make([]fiber.Map, 0, len(rooms))
	for _, r := range rooms {
		name := ""
		otheruid := uint64(0)

		switch r.Chattype {
		case utils.CHAT_TYPE_USER2MOD:
			if r.Namefull != "" {
				name = r.Namefull + " Volunteers"
			} else if r.Nameshort != "" {
				name = r.Nameshort + " Volunteers"
			}
			// For User2Mod, user1 is the member contacting the group.
			if r.User1 != myid {
				otheruid = r.User1
			}
		case utils.CHAT_TYPE_MOD2MOD:
			if r.Nameshort != "" {
				name = r.Nameshort + " Mods"
			}
		default:
			if r.User1 == myid {
				otheruid = r.User2
			} else {
				otheruid = r.User1
			}
			if n, ok := userNames[otheruid]; ok {
				name = tnre.ReplaceAllString(n, "$1")
			}
		}

		snippet := ""
		var lastdate *time.Time
		var lastmsg uint64
		if lm, ok := lastMsgMap[r.ID]; ok {
			lastdate = lm.Lastdate
			lastmsg = lm.Lastmsg
			snippet = getSnippet(lm.Chatmsgtype, lm.Chatmsg, lm.Refmsgtype)
		}

		defaultIcon := "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"

		room := fiber.Map{
			"id":          r.ID,
			"chattype":    r.Chattype,
			"name":        name,
			"unseen":      unseenMap[r.ID],
			"lastdate":    lastdate,
			"lastmsg":     lastmsg,
			"lastmsgseen": lastSeenMap[r.ID],
			"snippet":     snippet,
			"otheruid":    otheruid,
			"groupid":     r.Groupid,
			"icon":        defaultIcon,
		}

		chatrooms = append(chatrooms, room)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "chatrooms": chatrooms})
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
func getChatName(db *gorm.DB, chattype string, groupid uint64, user1 uint64, user2 uint64, myid uint64) string {
	switch chattype {
	case utils.CHAT_TYPE_USER2MOD:
		if groupid > 0 {
			var namefull string
			db.Raw("SELECT namefull FROM `groups` WHERE id = ?", groupid).Scan(&namefull)
			if namefull != "" {
				return namefull + " Volunteers"
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
