package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const chatActiveLimitMT = 365

type Tabler interface {
	TableName() string
}

type ChatRoomListEntry struct {
	ID            uint64     `json:"id" gorm:"primary_key"`
	Chattype      string     `json:"chattype"`
	Groupid       uint64     `json:"groupid"`
	User1         uint64     `json:"user1"`
	User2         uint64     `json:"user2"`
	Otheruid      uint64     `json:"otheruid"`
	Otherdeleted  *time.Time `json:"-"`
	Supporter     bool       `json:"supporter"`
	Icon          string     `json:"icon"`
	Lastdate      *time.Time `json:"lastdate"`
	Lastmsg       uint64     `json:"lastmsg"`
	Lastmsgseen   uint64     `json:"lastmsgseen"`
	Lasttype      *time.Time `json:"lasttype"`
	Name          string     `json:"name"`
	Nameshort     string     `json:"-"`
	Namefull      string     `json:"-"`
	Firstname     string     `json:"-"`
	Lastname      string     `json:"-"`
	Fullname      string     `json:"-"`
	Replyexpected uint64     `json:"replyexpected"`
	Snippet       string     `json:"snippet"`
	Unseen        uint64     `json:"unseen"`
	Chatmsg       string     `json:"-"`
	Chatmsgtype   string     `json:"-"`
	Refmsgtype    string     `json:"-"`
	Gimageid      uint64     `json:"-"`
	U1imageid     uint64     `json:"-"`
	U2imageid     uint64     `json:"-"`
	U1imageurl      string          `json:"-"`
	U2imageurl      string          `json:"-"`
	U1externaluid   string          `json:"-"`
	U2externaluid   string          `json:"-"`
	U1externalmods  json.RawMessage `json:"-"`
	U2externalmods  json.RawMessage `json:"-"`
	U1archived      int             `json:"-"`
	U2archived      int             `json:"-"`
	U1useprofile    bool            `json:"-"`
	U2useprofile    bool            `json:"-"`
	Status        string     `json:"status"`

	Search bool `json:"search,omitempty" gorm:"column:search"`
}

// buildUserIcon builds a profile image URL using the same logic as user.ProfileSetPath,
// ensuring chat listing icons match user profile fetch results.
//
// Chat icon rules:
//   User2Mod, mod viewing:  icon = member's profile (the user who contacted the group)
//   User2Mod, user viewing: icon = group logo
//   User2User:              icon = other user's profile
//
// The image data MUST come from the latest users_images row (ORDER BY id DESC
// LIMIT 1) to match GetProfileRecord(). Using an arbitrary row causes avatar
// mismatch between the chat list icon and the chat header/user profile (#281).
func buildUserIcon(imageid uint64, imageurl string, externaluid string, externalmods json.RawMessage, archived int) string {
	var profile user.UserProfile
	user.ProfileSetPath(imageid, imageurl, externaluid, externalmods, archived, &profile)
	if profile.Paththumb != "" {
		return profile.Paththumb
	}
	return "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"
}

func (ChatRoom) TableName() string {
	return "chat_rooms"
}

type ChatRoom struct {
	ID       uint64 `json:"id" gorm:"primary_key"`
	Chattype string `json:"chattype"`
	User1    uint64 `json:"user1"`
	User2    uint64 `json:"user2"`
}

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

	return []string{utils.CHAT_TYPE_USER2USER, utils.CHAT_TYPE_USER2MOD}
}

// =============================================================================
// GET handlers
// =============================================================================

func ListForUser(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	chattypes := parseChattypes(c)

	since := c.Query("since")

	var start string

	if since != "" {
		t, err := time.Parse(time.RFC3339, since)

		if err == nil {
			start = t.Format("2006-01-02")
		}
	} else {
		// Use a longer lookback for moderator chat types.
		hasMod := false
		for _, ct := range chattypes {
			if ct == utils.CHAT_TYPE_USER2MOD || ct == utils.CHAT_TYPE_MOD2MOD {
				hasMod = true
				break
			}
		}

		if hasMod {
			start = time.Now().AddDate(0, 0, -chatActiveLimitMT).Format("2006-01-02")
		} else {
			start = time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")
		}
	}

	search := c.Query("search")
	keepChatStr := c.Query("keepChat", "")
	keepChat := uint64(0)
	includeClosed := c.QueryBool("includeClosed", false)

	if keepChatStr != "" {
		keepChat, _ = strconv.ParseUint(keepChatStr, 10, 64)
	}

	r := listChats(myid, chattypes, start, search, 0, keepChat, includeClosed, true)

	if len(r) == 0 {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	} else {
		return c.JSON(r)
	}
}

// ListForUserMT handles GET /chat/rooms for moderator chat listing.
// Returns the same data as ListForUser but wrapped in {"chatrooms": [...]} for ModTools client compatibility.
func ListForUserMT(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	chattypes := parseChattypes(c)

	since := c.Query("since")

	var start string

	if since != "" {
		t, err := time.Parse(time.RFC3339, since)

		if err == nil {
			start = t.Format("2006-01-02")
		}
	} else {
		start = time.Now().AddDate(0, 0, -chatActiveLimitMT).Format("2006-01-02")
	}

	search := c.Query("search")

	r := listChats(myid, chattypes, start, search, 0, 0, false, false)
	if r == nil {
		r = []ChatRoomListEntry{}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "chatrooms": r})
}

func GetChat(c *fiber.Ctx) error {
	// convert id to uint64
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid chat id")
	}

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	chat, err2 := GetChatRoom(id, myid)

	if !err2 {
		return c.JSON(chat)
	}

	return fiber.NewError(fiber.StatusNotFound, "Chat not found")
}

func GetChatRoom(id uint64, myid uint64) (ChatRoomListEntry, bool) {
	// Include empty (and maybe closed) chats if we're getting a specific chat.
	// Pass all chat types so any type of chat can be fetched.
	chats := listChats(myid, []string{utils.CHAT_TYPE_USER2USER, utils.CHAT_TYPE_USER2MOD, utils.CHAT_TYPE_MOD2MOD}, "2009-09-11", "", id, id, true, false)

	if len(chats) > 0 {
		return chats[0], false
	}

	// listChats only returns chats where the user is a direct participant
	// (or a moderator for User2Mod chats). Moderators may also need to view
	// User2User chats in their groups (for chat review, support, etc.).
	// Fall back to a direct lookup with permission check via canSeeChatRoom,
	// then fetch enriched data by running listChats as a participant.
	db := database.DBConn

	type roomBasic struct {
		ID       uint64 `gorm:"column:id"`
		User1    uint64 `gorm:"column:user1"`
		User2    uint64 `gorm:"column:user2"`
		Groupid  uint64 `gorm:"column:groupid"`
		Chattype string `gorm:"column:chattype"`
	}
	var room roomBasic
	db.Raw("SELECT id, user1, user2, COALESCE(groupid, 0) AS groupid, chattype FROM chat_rooms WHERE id = ?", id).Scan(&room)

	if room.ID == 0 {
		var chat ChatRoomListEntry
		return chat, true
	}

	if !canSeeChatRoom(myid, room.User1, room.User2, room.Groupid) {
		var chat ChatRoomListEntry
		return chat, true
	}

	// Fetch enriched data by running listChats as one of the participants.
	// Use user1 (or user2 if user1 is 0, e.g. User2Mod where user2 is the group).
	participant := room.User1
	if participant == 0 {
		participant = room.User2
	}
	chats = listChats(participant, []string{room.Chattype}, "2009-09-11", "", id, id, true, false)
	if len(chats) > 0 {
		return chats[0], false
	}

	var chat ChatRoomListEntry
	return chat, true
}

// GetChatRoomsMT handles GET /chatrooms for moderator operations.
// Only supports count mode. Single-chat fetch uses GET /chat/:id.
//
// @Summary Get chatroom unseen count for moderator
// @Tags chat
// @Produce json
// @Param count query boolean false "Return unseen count only"
// @Param chattypes query string false "Chat types"
// @Success 200 {object} map[string]interface{}
// @Router /api/chatrooms [get]
func GetChatRoomsMT(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	countMode := c.QueryBool("count", false)
	chattypes := parseChattypes(c)

	if countMode {
		return countUnseenMT(c, myid, chattypes)
	}

	// the old frontend calls GET /chatrooms?id=X to fetch a single chat.
	chatID := c.QueryInt("id", 0)
	if chatID > 0 {
		chat, notFound := GetChatRoom(uint64(chatID), myid)
		if notFound {
			return fiber.NewError(fiber.StatusNotFound, "Chat not found")
		}
		return c.JSON(chat)
	}

	// Listing is handled by ListForUserMT via /chat/rooms.
	// Single-chat fetch uses GET /chat/:id.
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"ret": 3, "status": "Use GET /chat/:id for single chat, or /chat/rooms for listing"})
}

// =============================================================================
// PUT handler
// =============================================================================

type PutChatRoomRequest struct {
	Userid       uint64 `json:"userid"`
	Groupid      uint64 `json:"groupid"`
	Chattype     string `json:"chattype"`
	UpdateRoster *bool  `json:"updateRoster"`
}

// PutChatRoom handles PUT /chat/rooms - open/create a User2User chat with another user.
//
// @Summary Open or create a chat room with another user
// @Tags chat
// @Accept json
// @Produce json
// @Param body body PutChatRoomRequest true "User to chat with"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} fiber.Error
// @Failure 401 {object} fiber.Error
// @Router /api/chat/rooms [put]
func PutChatRoom(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PutChatRoomRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Determine chat type — default to User2User.
	chattype := utils.CHAT_TYPE_USER2USER
	if req.Chattype == utils.CHAT_TYPE_USER2MOD {
		chattype = utils.CHAT_TYPE_USER2MOD
	}

	if chattype == utils.CHAT_TYPE_USER2USER {
		if req.Userid == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "userid is required")
		}
		if req.Userid == myid {
			return fiber.NewError(fiber.StatusBadRequest, "Cannot create a chat with yourself")
		}
	} else if chattype == utils.CHAT_TYPE_USER2MOD {
		if req.Groupid == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "groupid is required for User2Mod")
		}
	}

	db := database.DBConn
	now := time.Now()

	if chattype == utils.CHAT_TYPE_USER2MOD {
		// Determine the target user for this User2Mod chat.
		// If a moderator provides userid, they want to open the MEMBER's existing
		// chat (e.g. from ModTools Feedback page). Non-mods always get their own chat.
		chatUserID := myid
		if req.Userid > 0 && req.Userid != myid && auth.IsModOfGroup(myid, req.Groupid) {
			chatUserID = req.Userid
		}

		// Find or create a chat between the target user and the group's mods.
		var existingID uint64
		db.Raw("SELECT id FROM chat_rooms WHERE user1 = ? AND chattype = ? AND groupid = ? LIMIT 1",
			chatUserID, utils.CHAT_TYPE_USER2MOD, req.Groupid).Scan(&existingID)

		if existingID > 0 {
			return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": existingID})
		}

		// Create new User2Mod chat.
		sqlDB, err := db.DB()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Database error")
		}
		sqlResult, err := sqlDB.Exec(
			"INSERT INTO chat_rooms (user1, chattype, groupid, latestmessage) VALUES (?, ?, ?, ?) "+
				"ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), latestmessage = VALUES(latestmessage)",
			chatUserID, utils.CHAT_TYPE_USER2MOD, req.Groupid, now)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create chat room")
		}
		lastID, _ := sqlResult.LastInsertId()
		if lastID == 0 {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create chat room")
		}

		chatID := uint64(lastID)

		// Create roster entry for the chat owner.
		db.Exec("INSERT INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE date = VALUES(date)",
			chatID, chatUserID, utils.CHAT_STATUS_ONLINE, now)

		// add ALL group moderators to the roster so they get notifications.
		var modIDs []uint64
		db.Raw("SELECT userid FROM memberships WHERE groupid = ? AND role IN (?, ?) AND collection = ?",
			req.Groupid, utils.ROLE_OWNER, utils.ROLE_MODERATOR, utils.COLLECTION_APPROVED).Pluck("userid", &modIDs)
		for _, modID := range modIDs {
			db.Exec("INSERT IGNORE INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, ?, ?)",
				chatID, modID, utils.CHAT_STATUS_ONLINE, now)
		}

		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": chatID})
	}

	// User2User flow below.

	// Check for existing chat first (covers both user orderings).
	var existingID uint64
	db.Raw("SELECT id FROM chat_rooms WHERE ((user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)) AND chattype = ? LIMIT 1",
		myid, req.Userid, req.Userid, myid, utils.CHAT_TYPE_USER2USER).Scan(&existingID)

	if existingID > 0 {
		if req.UpdateRoster != nil && *req.UpdateRoster {
			db.Exec("UPDATE chat_roster SET status = ? WHERE chatid = ? AND userid = ?",
				utils.CHAT_STATUS_ONLINE, existingID, myid)
		}
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": existingID})
	}

	// Use INSERT ... ON DUPLICATE KEY UPDATE to handle concurrent creation atomically.
	// The unique key (user1, user2, chattype) ensures only one row exists;
	// LAST_INSERT_ID(id) returns the existing row's ID on conflict.
	//
	// Use the underlying sql.DB to get LastInsertId() directly from the MySQL protocol
	// response — never issue a separate SELECT LAST_INSERT_ID() as it's unsafe under
	// parallel load (GORM's connection pool may assign a different connection).
	var chatID uint64
	sqlDB, err := db.DB()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	sqlResult, err := sqlDB.Exec("INSERT INTO chat_rooms (user1, user2, chattype, latestmessage) VALUES (?, ?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), latestmessage = VALUES(latestmessage)",
		myid, req.Userid, utils.CHAT_TYPE_USER2USER, now)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create chat room")
	}
	lastID, err := sqlResult.LastInsertId()
	if err == nil && lastID > 0 {
		chatID = uint64(lastID)
	}

	if chatID == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create chat room")
	}

	// Create roster entries for both users.
	db.Exec("INSERT INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE date = VALUES(date)",
		chatID, myid, utils.CHAT_STATUS_ONLINE, now)
	db.Exec("INSERT INTO chat_roster (chatid, userid, status, date) VALUES (?, ?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE date = VALUES(date)",
		chatID, req.Userid, utils.CHAT_STATUS_ONLINE, now)

	// If updateRoster is true, unblock the chat for the current user after creation
	// (opening a chat unblocks it).
	if req.UpdateRoster != nil && *req.UpdateRoster {
		db.Exec("UPDATE chat_roster SET status = ? WHERE chatid = ? AND userid = ?",
			utils.CHAT_STATUS_ONLINE, chatID, myid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": chatID})
}

// GetOrCreateUser2ModChat finds or creates a User2Mod chat room for a user on a group.
// Uses transaction + SELECT FOR UPDATE to prevent duplicate creation, matching V1
// ChatRoom::createUser2Mod(). The unique key (user1, user2, chattype) does NOT prevent
// User2Mod duplicates because user2 is NULL and MySQL treats NULLs as distinct in
// unique indexes. We must lock explicitly.
func GetOrCreateUser2ModChat(db *gorm.DB, userID uint64, groupID uint64) (uint64, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return 0, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	tx, err := sqlDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock any existing row to close the timing window.
	var chatID uint64
	row := tx.QueryRow(
		"SELECT id FROM chat_rooms WHERE user1 = ? AND groupid = ? AND chattype = ? FOR UPDATE",
		userID, groupID, utils.CHAT_TYPE_USER2MOD)
	if err := row.Scan(&chatID); err == nil && chatID > 0 {
		// Existing chat found — just update latestmessage and return.
		tx.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)
		tx.Commit()
	} else {
		// No existing chat — create one inside the same transaction.
		result, err := tx.Exec(
			"INSERT INTO chat_rooms (user1, groupid, chattype, latestmessage) VALUES (?, ?, ?, NOW())",
			userID, groupID, utils.CHAT_TYPE_USER2MOD)
		if err != nil {
			return 0, fmt.Errorf("failed to insert chat room: %w", err)
		}

		lastID, err := result.LastInsertId()
		if err != nil || lastID == 0 {
			return 0, fmt.Errorf("failed to get last insert id: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("failed to commit transaction: %w", err)
		}

		chatID = uint64(lastID)
	}

	// Ensure the user and group mods are in the roster so that
	// chat notifications reach everyone.
	db.Exec("INSERT IGNORE INTO chat_roster (chatid, userid) VALUES (?, ?)", chatID, userID)

	var modUserIDs []uint64
	db.Raw("SELECT userid FROM memberships WHERE groupid = ? AND role IN (?, ?)", groupID, utils.ROLE_OWNER, utils.ROLE_MODERATOR).Scan(&modUserIDs)
	for _, modUID := range modUserIDs {
		db.Exec("INSERT IGNORE INTO chat_roster (chatid, userid) VALUES (?, ?)", chatID, modUID)
	}

	return chatID, nil
}

// =============================================================================
// POST handler (roster updates, nudge, typing, actions)
// =============================================================================

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
	case "AllSeen":
		return handleAllSeen(c, db, myid)
	case "Nudge":
		return handleNudge(c, db, myid, req.ID)
	case "Typing":
		return handleTyping(c, db, myid, req.ID)
	case "ReferToSupport":
		return handleReferToSupport(c, db, myid, req.ID)
	default:
		if req.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "Chat ID required")
		}
		return handleRosterUpdate(c, db, myid, req)
	}
}

// =============================================================================
// Internal helpers
// =============================================================================

func listChats(myid uint64, chattypes []string, start string, search string, onlyChat uint64, keepChat uint64, includeClosed bool, memberOnly bool) []ChatRoomListEntry {
	var r []ChatRoomListEntry

	// V1 parity: unseen messages older than CHAT_ACTIVE_LIMIT days are excluded
	// from the count, regardless of the chat list's own start date (which may be
	// older for single-chat fetches or keepChat).
	unseenSince := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

	// The chats we can see are:
	// - a conversation that we have not closed
	// - active within the lookback period
	// - a specific chat which we have asked for which was closed or blocked, which we would otherwise exclude
	//
	// We build UNION branches dynamically based on the requested chat types.
	// For User2Mod: user is user1 (the member contacting the group)
	// For User2User: user is either user1 or user2
	// For Mod2Mod: user is a moderator of the group (joined via memberships)
	var chats []ChatRoomListEntry

	statusq := " "

	if !includeClosed {
		// Filter out closed and blocked chats.
		statusq = " AND (c1.status IS NULL OR (c1.status != '" + utils.CHAT_STATUS_CLOSED + "' AND c1.status != '" + utils.CHAT_STATUS_BLOCKED + "') "

		if keepChat > 0 {
			statusq += " OR chat_rooms.id = " + fmt.Sprintf("%d", keepChat)
		}

		statusq += ") "
	}

	onlyChatq := ""

	if onlyChat > 0 {
		onlyChatq += " AND chat_rooms.id = " + strconv.FormatUint(onlyChat, 10) + " "
	}

	atts := "chat_rooms.id, chat_rooms.chattype, chat_rooms.groupid, chat_rooms.user1, chat_rooms.user2, chat_rooms.latestmessage"

	// Build UNION branches dynamically based on requested chat types.
	unions := []string{}
	params := []interface{}{}

	for _, ct := range chattypes {
		switch ct {
		case utils.CHAT_TYPE_USER2MOD:
			if memberOnly {
				// Freegle (user-facing): only show User2Mod chats where we are the member (user1).
				// Moderators should not see other members' modmails on their personal Freegle chat list.
				unions = append(unions,
					"SELECT 0 AS search, user1 AS otheruid, nameshort, namefull, "+
						"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS firstname, "+
						"'' AS lastname, "+
						"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS fullname, "+
						"(SELECT deleted FROM users WHERE users.id = user1) AS otherdeleted, "+
						atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
						"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"WHERE chattype = ? AND latestmessage >= ? "+
						"AND user1 = ? "+
						statusq+" "+onlyChatq)
				params = append(params, myid, utils.CHAT_TYPE_USER2MOD, start, myid)
			} else {
				// ModTools: show User2Mod chats where we are the member OR a moderator of the group.
				// Exclude backup mods (active:0 in membership settings) unless searching.
				unions = append(unions,
					"SELECT 0 AS search, user1 AS otheruid, nameshort, namefull, "+
						"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS firstname, "+
						"'' AS lastname, "+
						"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS fullname, "+
						"(SELECT deleted FROM users WHERE users.id = user1) AS otherdeleted, "+
						atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
						"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"WHERE chattype = ? AND latestmessage >= ? "+
						"AND (user1 = ? OR EXISTS(SELECT 1 FROM memberships WHERE memberships.userid = ? AND memberships.groupid = chat_rooms.groupid AND memberships.role IN (?, ?) "+
						"AND (memberships.settings IS NULL OR LOCATE('\"active\"', memberships.settings) = 0 OR LOCATE('\"active\":1', memberships.settings) > 0))) "+
						statusq+" "+onlyChatq)
				params = append(params, myid, utils.CHAT_TYPE_USER2MOD, start, myid, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER)
			}

		case utils.CHAT_TYPE_USER2USER:
			// User2User: user is user1
			unions = append(unions,
				"SELECT 0 AS search, user2 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
					atts+", c1.status, c2.lasttype FROM chat_rooms "+
					"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
					"LEFT JOIN chat_roster c2 ON c2.userid = user2 AND chat_rooms.id = c2.chatid "+
					"INNER JOIN users ON users.id = user2 "+
					"WHERE user1 = ? AND user1 != user2 AND chattype = ? AND latestmessage >= ? "+onlyChatq+statusq)
			params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER, start)

			// User2User: user is user2
			unions = append(unions,
				"SELECT 0 AS search, user1 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
					atts+", c1.status, c2.lasttype FROM chat_rooms "+
					"INNER JOIN users ON users.id = user1 "+
					"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
					"LEFT JOIN chat_roster c2 ON c2.userid = user1 AND chat_rooms.id = c2.chatid "+
					"WHERE user2 = ? AND user1 != user2 AND chattype = ? AND latestmessage >= ? "+onlyChatq+statusq)
			params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER, start)

		case utils.CHAT_TYPE_MOD2MOD:
			// Mod2Mod: user is a moderator of the group.
			// Exclude backup mods and all-spam chats.
			unions = append(unions,
				"SELECT 0 AS search, 0 AS otheruid, nameshort, namefull, '' AS firstname, '' AS lastname, '' AS fullname, NULL AS otherdeleted, "+
					atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
					"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
					"INNER JOIN memberships ON memberships.groupid = chat_rooms.groupid AND memberships.userid = ? AND memberships.role IN (?, ?) "+
					"AND (memberships.settings IS NULL OR LOCATE('\"active\"', memberships.settings) = 0 OR LOCATE('\"active\":1', memberships.settings) > 0) "+
					"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
					"WHERE chattype = ? AND latestmessage >= ? "+
					"AND (chat_rooms.msgvalid + chat_rooms.msginvalid = 0 OR chat_rooms.msgvalid > 0) "+
					statusq+" "+onlyChatq)
			params = append(params, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER, myid, utils.CHAT_TYPE_MOD2MOD, start)
		}
	}

	if search != "" {
		searchLike := "%" + search + "%"

		for _, ct := range chattypes {
			switch ct {
			case utils.CHAT_TYPE_USER2USER:
				// Search User2User chats where user is user1 — by message content/subject.
				unions = append(unions,
					"SELECT 1 AS search, user2 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
						atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"INNER JOIN users ON users.id = user2 "+
						"INNER JOIN chat_messages ON chat_messages.chatid = chat_rooms.id "+
						"LEFT JOIN messages ON messages.id = chat_messages.refmsgid "+
						"WHERE user1 = ? AND user1 != user2 AND chattype = ? "+onlyChatq+" "+
						"AND (chat_messages.message LIKE ? OR messages.subject LIKE ?) ")
				params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER, searchLike, searchLike)

				// Search User2User chats where user is user1 — by other user's name/email.
				unions = append(unions,
					"SELECT 1 AS search, user2 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
						atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"INNER JOIN users ON users.id = user2 "+
						"LEFT JOIN users_emails ON users_emails.userid = user2 "+
						"WHERE user1 = ? AND user1 != user2 AND chattype = ? "+onlyChatq+" "+
						"AND (users.fullname LIKE ? OR users_emails.email LIKE ?) ")
				params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER, searchLike, searchLike)

				// Search User2User chats where user is user2 — by message content/subject.
				unions = append(unions,
					"SELECT 1 AS search, user1 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
						atts+", c1.status, c2.lasttype FROM chat_rooms "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"LEFT JOIN chat_roster c2 ON c2.userid = user1 AND chat_rooms.id = c2.chatid "+
						"INNER JOIN users ON users.id = user1 "+
						"INNER JOIN chat_messages ON chat_messages.chatid = chat_rooms.id "+
						"LEFT JOIN messages ON messages.id = chat_messages.refmsgid "+
						"WHERE user2 = ? AND user1 != user2 AND chattype = ? "+onlyChatq+" "+
						"AND (chat_messages.message LIKE ? OR messages.subject LIKE ?) ")
				params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER, searchLike, searchLike)

				// Search User2User chats where user is user2 — by other user's name/email.
				unions = append(unions,
					"SELECT 1 AS search, user1 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
						atts+", c1.status, c2.lasttype FROM chat_rooms "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"LEFT JOIN chat_roster c2 ON c2.userid = user1 AND chat_rooms.id = c2.chatid "+
						"INNER JOIN users ON users.id = user1 "+
						"LEFT JOIN users_emails ON users_emails.userid = user1 "+
						"WHERE user2 = ? AND user1 != user2 AND chattype = ? "+onlyChatq+" "+
						"AND (users.fullname LIKE ? OR users_emails.email LIKE ?) ")
				params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER, searchLike, searchLike)

			case utils.CHAT_TYPE_USER2MOD:
				if memberOnly {
					// Freegle: search only User2Mod chats where we are the member — by message content/subject.
					unions = append(unions,
						"SELECT 1 AS search, user1 AS otheruid, nameshort, namefull, "+
							"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS firstname, "+
							"'' AS lastname, "+
							"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS fullname, "+
							"(SELECT deleted FROM users WHERE users.id = user1) AS otherdeleted, "+
							atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
							"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
							"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
							"INNER JOIN chat_messages ON chat_messages.chatid = chat_rooms.id "+
							"LEFT JOIN messages ON messages.id = chat_messages.refmsgid "+
							"WHERE chattype = ? "+
							"AND user1 = ? "+
							onlyChatq+" "+
							"AND (chat_messages.message LIKE ? OR messages.subject LIKE ?) ")
					params = append(params, myid, utils.CHAT_TYPE_USER2MOD, myid, searchLike, searchLike)
				} else {
					// ModTools: search User2Mod chats visible to user — by message content/subject.
					unions = append(unions,
						"SELECT 1 AS search, user1 AS otheruid, nameshort, namefull, "+
							"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS firstname, "+
							"'' AS lastname, "+
							"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS fullname, "+
							"(SELECT deleted FROM users WHERE users.id = user1) AS otherdeleted, "+
							atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
							"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
							"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
							"INNER JOIN chat_messages ON chat_messages.chatid = chat_rooms.id "+
							"LEFT JOIN messages ON messages.id = chat_messages.refmsgid "+
							"WHERE chattype = ? "+
							"AND (user1 = ? OR EXISTS(SELECT 1 FROM memberships WHERE memberships.userid = ? AND memberships.groupid = chat_rooms.groupid AND memberships.role IN (?, ?))) "+
							onlyChatq+" "+
							"AND (chat_messages.message LIKE ? OR messages.subject LIKE ?) ")
					params = append(params, myid, utils.CHAT_TYPE_USER2MOD, myid, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER, searchLike, searchLike)

					// ModTools: search User2Mod chats by member's name/email.
					unions = append(unions,
						"SELECT 1 AS search, user1 AS otheruid, nameshort, namefull, "+
							"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS firstname, "+
							"'' AS lastname, "+
							"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS fullname, "+
							"(SELECT deleted FROM users WHERE users.id = user1) AS otherdeleted, "+
							atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
							"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
							"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
							"INNER JOIN users ON users.id = user1 "+
							"LEFT JOIN users_emails ON users_emails.userid = user1 "+
							"WHERE chattype = ? "+
							"AND (user1 = ? OR EXISTS(SELECT 1 FROM memberships WHERE memberships.userid = ? AND memberships.groupid = chat_rooms.groupid AND memberships.role IN (?, ?))) "+
							onlyChatq+" "+
							"AND (users.fullname LIKE ? OR users_emails.email LIKE ?) ")
					params = append(params, myid, utils.CHAT_TYPE_USER2MOD, myid, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER, searchLike, searchLike)
				}

			case utils.CHAT_TYPE_MOD2MOD:
				// Search Mod2Mod chats visible to user as moderator — by message content/subject.
				unions = append(unions,
					"SELECT 1 AS search, 0 AS otheruid, nameshort, namefull, '' AS firstname, '' AS lastname, '' AS fullname, NULL AS otherdeleted, "+
						atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
						"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
						"INNER JOIN memberships ON memberships.groupid = chat_rooms.groupid AND memberships.userid = ? AND memberships.role IN (?, ?) "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"INNER JOIN chat_messages ON chat_messages.chatid = chat_rooms.id "+
						"LEFT JOIN messages ON messages.id = chat_messages.refmsgid "+
						"WHERE chattype = ? "+onlyChatq+" "+
						"AND (chat_messages.message LIKE ? OR messages.subject LIKE ?) ")
				params = append(params, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER, myid, utils.CHAT_TYPE_MOD2MOD, searchLike, searchLike)
			}
		}
	}

	// keepChat: include a specific chat even if it predates the 'start' cutoff.
	// We add extra UNION branches for the requested chat types that match only
	// this chat ID, without the latestmessage date restriction.
	if keepChat > 0 {
		keepChatID := fmt.Sprintf("%d", keepChat)
		for _, ct := range chattypes {
			switch ct {
			case utils.CHAT_TYPE_USER2MOD:
				unions = append(unions,
					"SELECT 0 AS search, user1 AS otheruid, nameshort, namefull, "+
						"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS firstname, "+
						"'' AS lastname, "+
						"COALESCE((SELECT fullname FROM users WHERE users.id = user1), '') AS fullname, "+
						"(SELECT deleted FROM users WHERE users.id = user1) AS otherdeleted, "+
						atts+", c1.status, NULL AS lasttype FROM chat_rooms "+
						"INNER JOIN `groups` ON groups.id = chat_rooms.groupid "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"WHERE chattype = ? AND user1 = ? AND chat_rooms.id = "+keepChatID+statusq)
				params = append(params, myid, utils.CHAT_TYPE_USER2MOD, myid)

			case utils.CHAT_TYPE_USER2USER:
				// User is user1.
				unions = append(unions,
					"SELECT 0 AS search, user2 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
						atts+", c1.status, c2.lasttype FROM chat_rooms "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"LEFT JOIN chat_roster c2 ON c2.userid = user2 AND chat_rooms.id = c2.chatid "+
						"INNER JOIN users ON users.id = user2 "+
						"WHERE user1 = ? AND user1 != user2 AND chattype = ? AND chat_rooms.id = "+keepChatID+statusq)
				params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER)

				// User is user2.
				unions = append(unions,
					"SELECT 0 AS search, user1 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, users.deleted AS otherdeleted, "+
						atts+", c1.status, c2.lasttype FROM chat_rooms "+
						"INNER JOIN users ON users.id = user1 "+
						"LEFT JOIN chat_roster c1 ON c1.userid = ? AND chat_rooms.id = c1.chatid "+
						"LEFT JOIN chat_roster c2 ON c2.userid = user1 AND chat_rooms.id = c2.chatid "+
						"WHERE user2 = ? AND user1 != user2 AND chattype = ? AND chat_rooms.id = "+keepChatID+statusq)
				params = append(params, myid, myid, utils.CHAT_TYPE_USER2USER)
			}
		}
	}

	if len(unions) == 0 {
		return r
	}

	sql := "SELECT MAX(t.search) AS search, t.otheruid, t.nameshort, t.namefull, t.firstname, t.lastname, t.fullname, t.otherdeleted, t.id, t.chattype, t.groupid, t.user1, t.user2, t.latestmessage, t.status, t.lasttype FROM (" + strings.Join(unions, " UNION ") + ") t GROUP BY t.id ORDER BY t.latestmessage DESC"

	db := database.DBConn
	db.Raw(sql, params...).Scan(&chats)

	// We hide the "-gxxx" part of names, which will almost always be for TN members.
	tnre := regexp.MustCompile(utils.TN_REGEXP)

	for ix, chat := range chats {
		if chat.Chattype == utils.CHAT_TYPE_USER2MOD {
			// Show the member's name to the moderator.
			if chat.Otheruid != myid && len(chat.Fullname) > 0 {
				groupName := chat.Nameshort
				if groupName == "" {
					groupName = chat.Namefull
				}
				chats[ix].Name = tnre.ReplaceAllString(chat.Fullname, "$1")
				if groupName != "" {
					chats[ix].Name += " (" + groupName + ")"
				}
			} else if len(chat.Namefull) > 0 {
				chats[ix].Name = chat.Namefull + " Volunteers"
			} else if len(chat.Nameshort) > 0 {
				chats[ix].Name = chat.Nameshort + " Volunteers"
			}

			// For User2Mod, otheruid should be user1 only when user1 != myid.
			if chat.Otheruid == myid {
				chats[ix].Otheruid = 0
			}
		} else if chat.Chattype == utils.CHAT_TYPE_MOD2MOD {
			if len(chat.Nameshort) > 0 {
				chats[ix].Name = chat.Nameshort + " Mods"
			} else if len(chat.Namefull) > 0 {
				chats[ix].Name = chat.Namefull + " Mods"
			}
		} else {
			if chat.Otherdeleted == nil {
				if len(chat.Fullname) > 0 {
					chats[ix].Name = chat.Fullname
				} else {
					chats[ix].Name = chat.Firstname + " " + chat.Lastname
				}
			} else {
				chats[ix].Name = "Deleted User #" + strconv.FormatUint(chat.Otheruid, 10)
			}

			chats[ix].Name = tnre.ReplaceAllString(chats[ix].Name, "$1")
		}
	}

	// Now we have the basic chat info.  We still need:
	// - the most recent chat message (if any) for a snippet
	// - the count of unread messages for the logged-in user
	// - the count of reply requested from other people
	// - the last seen for this user.
	// - the profile pic and setting about whether to show it
	// - the supporter info for the chat users
	// This is a beast of a query.
	if len(chats) > 0 {
		var chats2 []ChatRoomListEntry

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()

			ids := []string{}

			for _, chat := range chats {
				ids = append(ids, strconv.FormatUint(chat.ID, 10))
			}

			idlist := "(" + strings.Join(ids, ",") + ") "

			sql = "SELECT DISTINCT chat_rooms.id, chat_rooms.chattype, chat_rooms.groupid, chat_rooms.user1, chat_rooms.user2, " +
				"CASE WHEN JSON_EXTRACT(u1.settings, '$.useprofile') IS NULL THEN 1 ELSE JSON_EXTRACT(u1.settings, '$.useprofile') END AS u1useprofile, " +
				"CASE WHEN JSON_EXTRACT(u2.settings, '$.useprofile') IS NULL THEN 1 ELSE JSON_EXTRACT(u2.settings, '$.useprofile') END AS u2useprofile, " +
				"(SELECT COUNT(*) AS count FROM chat_messages WHERE id > " +
				"  COALESCE((SELECT lastmsgseen FROM chat_roster c1 WHERE chatid = chat_rooms.id AND userid = ? " +
				"  " + statusq + "), 0) AND chatid = chat_rooms.id AND userid != ? AND chat_messages.date >= ? AND (reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1)) AS unseen, " +
				"(SELECT COUNT(*) AS count FROM chat_messages WHERE chatid = chat_rooms.id AND replyexpected = 1 AND" +
				"  replyreceived = 0 AND userid != ? AND chat_messages.date >= ? AND chat_rooms.chattype = ? AND processingsuccessful = 1) AS replyexpected, " +
				"i1.id AS u1imageid, " +
				"i2.id AS u2imageid, " +
				"i1.url AS u1imageurl, " +
				"i2.url AS u2imageurl, " +
				"COALESCE(i1.externaluid, '') AS u1externaluid, " +
				"COALESCE(i2.externaluid, '') AS u2externaluid, " +
				"i1.externalmods AS u1externalmods, " +
				"i2.externalmods AS u2externalmods, " +
				"COALESCE(i1.archived, 0) AS u1archived, " +
				"COALESCE(i2.archived, 0) AS u2archived, " +
				"i3.id AS gimageid, " +
				"(SELECT chat_roster.lastmsgseen FROM chat_roster WHERE chatid = chat_rooms.id AND userid = ?) AS lastmsgseen, " +
				"messages.type AS refmsgtype, " +
				"rcm.* " +
				"FROM chat_rooms " +
				"LEFT JOIN `groups` ON groups.id = chat_rooms.groupid " +
				"LEFT JOIN users u1 ON chat_rooms.user1 = u1.id " +
				"LEFT JOIN users u2 ON chat_rooms.user2 = u2.id " +
				// Profile image join must match GetProfileRecord() logic: latest image
				// (ORDER BY id DESC LIMIT 1) so the icon is identical to what the user
				// store returns. Users may have multiple images; picking an arbitrary
				// one causes avatar mismatch between chat list and chat header (#281).
				"LEFT JOIN users_images i1 ON i1.id = (SELECT id FROM users_images WHERE userid = u1.id ORDER BY id DESC LIMIT 1) " +
				"LEFT JOIN users_images i2 ON i2.id = (SELECT id FROM users_images WHERE userid = u2.id ORDER BY id DESC LIMIT 1) " +
				"LEFT JOIN groups_images i3 ON i3.id = (SELECT id FROM groups_images WHERE groupid = chat_rooms.groupid ORDER BY id DESC LIMIT 1) " +
				"LEFT JOIN chat_messages ON chat_messages.id = " +
				"  (SELECT id FROM chat_messages WHERE chat_messages.chatid = chat_rooms.id AND reviewrequired = 0 AND reviewrejected = 0 AND (processingsuccessful = 1 OR chat_messages.userid = ?) ORDER BY chat_messages.id DESC LIMIT 1) " +
				"LEFT JOIN messages ON messages.id = chat_messages.refmsgid " +
				"LEFT JOIN (WITH cm AS (SELECT chat_messages.id AS lastmsg, chat_messages.chatid, chat_messages.message AS chatmsg," +
				" chat_messages.date AS lastdate, chat_messages.type AS chatmsgtype, ROW_NUMBER() OVER (PARTITION BY chatid ORDER BY id DESC) AS rn " +
				" FROM chat_messages WHERE chatid IN " + idlist + " AND (reviewrequired = 0 AND reviewrejected = 0 AND (processingsuccessful = 1 OR chat_messages.userid = ?) OR userid = ?)) " +
				"  SELECT * FROM cm WHERE rn = 1) rcm ON rcm.chatid = chat_rooms.id " +
				"WHERE chat_rooms.id IN " + idlist

			res := db.Raw(sql, myid, myid, unseenSince, myid, start, utils.CHAT_TYPE_USER2USER, myid, myid, myid, myid)
			res.Scan(&chats2)
		}()

		supporterMap := map[uint64]bool{}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Get the supporter status for the other users.
			ids := []string{}

			ids = append(ids, strconv.FormatUint(myid, 10))

			for _, chat := range chats {
				if chat.Otherdeleted == nil && chat.Otheruid > 0 {
					ids = append(ids, strconv.FormatUint(chat.Otheruid, 10))
				}
			}

			if (len(ids)) > 0 {
				idlist := "(" + strings.Join(ids, ",") + ") "

				start := time.Now().AddDate(0, 0, -utils.SUPPORTER_PERIOD).Format("2006-01-02")

				// Use a temporary type struct to hold the supporter status as the user field is not in the DB
				// and is marked as - in the GORM definition.
				type supporterStatus struct {
					ID        uint64 `json:"id"`
					Supporter bool   `json:"supporter"`
				}
				var supporters []supporterStatus

				db.Raw("SELECT DISTINCT users.id, (CASE WHEN "+
					"((users.systemrole != ? OR "+
					"EXISTS(SELECT id FROM users_donations WHERE userid = users.id AND users_donations.timestamp >= ?) OR "+
					"EXISTS(SELECT id FROM microactions WHERE userid = users.id AND microactions.timestamp >= ?)) AND "+
					"(CASE WHEN JSON_EXTRACT(users.settings, '$.hidesupporter') IS NULL THEN 0 ELSE JSON_EXTRACT(users.settings, '$.hidesupporter') END) = 0) "+
					"THEN 1 ELSE 0 END) "+
					"AS supporter "+
					"FROM users "+
					"WHERE users.id IN "+idlist, utils.SYSTEMROLE_USER, start, start).Scan(&supporters)

				// Convert supporters into a map for easy of access below.
				for _, supporter := range supporters {
					supporterMap[supporter.ID] = supporter.Supporter
				}
			}
		}()

		wg.Wait()

		// Combine the data.
		//
		// Scalability isn't great here.
		for ix, chat1 := range chats {
			chats[ix].Supporter = false

			if chat1.Otherdeleted == nil && chat1.Otheruid > 0 {
				// Check if otheruid is in map
				if val, ok := supporterMap[chat1.Otheruid]; ok {
					chats[ix].Supporter = val
				}
			}

			for _, chat := range chats2 {
				if chat1.ID == chat.ID {
					if chat.Lastdate != nil {
						chats[ix].Lastdate = chat.Lastdate
						chats[ix].Lastmsg = chat.Lastmsg
						chats[ix].Lastmsgseen = chat.Lastmsgseen
					}

					if chat1.Otherdeleted == nil {
						chats[ix].Unseen = chat.Unseen
						chats[ix].Replyexpected = chat.Replyexpected

						if chat.Chattype == utils.CHAT_TYPE_MOD2MOD {
							// Mod2Mod: show group logo.
							if chat.Gimageid > 0 {
								chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/gimg_" + strconv.FormatUint(chat.Gimageid, 10) + ".jpg"
							} else {
								chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"
							}
						} else if chat.Chattype == utils.CHAT_TYPE_USER2MOD {
							// User2Mod: depends on perspective.
							// - Member (user1) sees group logo (they're chatting with volunteers)
							// - Mod sees member's (user1) profile picture
							if chat.User1 == myid {
								// I'm the member — show group logo.
								if chat.Gimageid > 0 {
									chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/gimg_" + strconv.FormatUint(chat.Gimageid, 10) + ".jpg"
								} else {
									chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"
								}
							} else {
								// I'm a mod — show member's profile picture.
								if chat.U1useprofile && chat.U1imageid > 0 {
									chats[ix].Icon = buildUserIcon(chat.U1imageid, chat.U1imageurl, chat.U1externaluid, chat.U1externalmods, chat.U1archived)
								} else {
									chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"
								}
							}
						} else {
							if chat.User1 == myid {
								if chat.U2useprofile && chat.U2imageid > 0 {
									chats[ix].Icon = buildUserIcon(chat.U2imageid, chat.U2imageurl, chat.U2externaluid, chat.U2externalmods, chat.U2archived)
								} else {
									chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"
								}
							} else {
								if chat.U1useprofile && chat.U1imageid > 0 {
									chats[ix].Icon = buildUserIcon(chat.U1imageid, chat.U1imageurl, chat.U1externaluid, chat.U1externalmods, chat.U1archived)
								} else {
									chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"
								}
							}
						}

					} else {
						chats[ix].Icon = "https://" + os.Getenv("IMAGE_DOMAIN") + "/defaultprofile.png"
					}

					// Snippet is set for all chats, including deleted users.
					if chats[ix].Search {
						chats[ix].Snippet = "...contains '" + search + "'"
					} else {
						chats[ix].Snippet = getSnippet(chat.Chatmsgtype, chat.Chatmsg, chat.Refmsgtype)
					}

					r = append(r, chats[ix])
					break
				}
			}
		}
	}

	return r
}

func getSnippet(msgtype string, chatmsg string, refmsgtype string) string {
	var ret string

	switch msgtype {
	case utils.CHAT_MESSAGE_ADDRESS:
		ret = "Address sent"
	case utils.CHAT_MESSAGE_NUDGE:
		ret = "Nudged"
	case utils.CHAT_MESSAGE_COMPLETED:
		if len(chatmsg) > 0 {
			ret = splitEmoji(chatmsg)

			if len(ret) > 100 {
				ret = ret[:100]
			}
		} else if refmsgtype == utils.OFFER {
			ret = "Item is no longer available"
		} else {
			ret = "No longer looking for this"
		}
	case utils.CHAT_MESSAGE_PROMISED:
		ret = "Item promised"
	case utils.CHAT_MESSAGE_RENEGED:
		ret = "Promise cancelled"
	case utils.CHAT_MESSAGE_IMAGE:
		ret = "Image"
	default:
		{
			// We don't want to land in the middle of an encoded emoji otherwise it will display
			// wrongly.
			ret = splitEmoji(chatmsg)

			if len(ret) > 100 {
				ret = ret[:100]
			}
		}
	}

	return ret
}

func splitEmoji(msg string) string {
	without := regexp.MustCompile("\\\\u.*?\\\\u/").ReplaceAllString(msg, "")

	// If we have something other than emojis, return that.  Otherwise return the emoji(s) which will be
	// rendered in the client.
	if len(without) > 0 {
		msg = without
	}

	return msg
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
	sqlDB, err := db.DB()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	sqlResult, err := sqlDB.Exec(
		"INSERT INTO chat_messages (chatid, userid, type, date, message, replyexpected, reportreason, reviewrequired, reviewrejected, processingsuccessful) VALUES (?, ?, ?, ?, '', 1, NULL, 0, 0, 1)",
		chatid, myid, utils.CHAT_MESSAGE_NUDGE, now,
	)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create nudge")
	}

	newIdInt, _ := sqlResult.LastInsertId()
	newId := uint64(newIdInt)

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

	// Verify the chat room exists.
	var roomID uint64
	db.Raw("SELECT id FROM chat_rooms WHERE id = ?", chatid).Scan(&roomID)
	if roomID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Chat not found")
	}

	// Bump date on recent unmailed messages to delay email batching.
	// This batches multiple chat messages into a single email when user is actively typing.
	// Uses a 30-second delay window.
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
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": strconv.FormatUint(req.ID, 10) + " Not visible to you"})
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
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": strconv.FormatUint(req.ID, 10) + " Not visible to you"})
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
				"ON DUPLICATE KEY UPDATE status = IF(status = ?, status, ?), lastip = ?, date = NOW()",
			req.ID, myid, status, ip, utils.CHAT_STATUS_BLOCKED, status, ip,
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

	// Get unseen count — only count messages from the last CHAT_ACTIVE_LIMIT days (V1 parity: ACTIVELIM).
	activeSince := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")
	var unseen int64
	db.Raw(`SELECT COUNT(*) FROM chat_messages
		WHERE chatid = ? AND userid != ?
		AND id > COALESCE((SELECT lastmsgseen FROM chat_roster WHERE chatid = ? AND userid = ?), 0)
		AND chat_messages.date >= ?
		AND reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1`,
		req.ID, myid, req.ID, myid, activeSince).Scan(&unseen)

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

func handleReferToSupport(c *fiber.Ctx, db *gorm.DB, myid uint64, chatid uint64) error {
	if chatid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Chat ID required")
	}

	// Verify user is a member of this chat.
	var room ChatRoom
	db.Raw("SELECT id, chattype, user1, user2, groupid FROM chat_rooms WHERE id = ?", chatid).Scan(&room)
	if room.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Chat not found")
	}
	if room.User1 != myid && room.User2 != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not a member of this chat")
	}

	// Queue sending a support referral email.
	db.Exec("INSERT INTO background_tasks (task_type, data) VALUES (?, JSON_OBJECT('chatid', ?, 'userid', ?))",
		"refer_to_support", chatid, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleAllSeen(c *fiber.Ctx, db *gorm.DB, myid uint64) error {
	// Mark all chat messages as seen across all chats for this user.
	// Sets lastmsgseen to the maximum message ID in each chat the user is rostered in.
	db.Exec(`UPDATE chat_roster
		SET lastmsgseen = (
			SELECT COALESCE(MAX(id), 0) FROM chat_messages WHERE chatid = chat_roster.chatid
		)
		WHERE userid = ?`, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func getOtherUser(room ChatRoom, myid uint64) uint64 {
	if room.User1 == myid {
		return room.User2
	}
	return room.User1
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

// getModeratorChatIDs returns chat room IDs visible to a moderator for the given chat types.
func getModeratorChatIDs(db *gorm.DB, myid uint64, chattypes []string, search string, age int) []uint64 {
	activeDays := chatActiveLimitMT
	if age > 0 {
		activeDays = age
	}
	activeSince := time.Now().AddDate(0, 0, -activeDays).Format("2006-01-02")

	var allIDs []uint64

	// Filter to exclude chats where all messages are held for review (likely spam).
	countq := " AND (chat_rooms.msgvalid + chat_rooms.msginvalid = 0 OR chat_rooms.msgvalid > 0) "

	// Filter to exclude backup mods (those with active:0 in their membership settings),
	// unless we're searching for a specific chat.
	activeq := ""
	if search == "" {
		activeq = " AND (memberships.settings IS NULL OR LOCATE('\"active\"', memberships.settings) = 0 OR LOCATE('\"active\":1', memberships.settings) > 0) "
	}

	for _, ct := range chattypes {
		var ids []uint64

		switch ct {
		case utils.CHAT_TYPE_MOD2MOD:
			db.Raw("SELECT DISTINCT chat_rooms.id FROM chat_rooms "+
				"INNER JOIN memberships ON chat_rooms.groupid = memberships.groupid "+
				"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid "+
				"WHERE memberships.userid = ? AND memberships.role IN (?, ?) "+
				activeq+
				"AND chat_rooms.chattype = ? "+
				"AND (chat_roster.status IS NULL OR chat_roster.status != ?) "+
				"AND chat_rooms.latestmessage >= ?"+
				countq,
				myid, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER, utils.CHAT_TYPE_MOD2MOD, utils.CHAT_STATUS_CLOSED, activeSince).Scan(&ids)

		case utils.CHAT_TYPE_USER2MOD:
			// User2Mod chats on modtools are not subject to the count query filter.
			db.Raw("SELECT DISTINCT id FROM ("+
				"SELECT chat_rooms.id FROM chat_rooms "+
				"INNER JOIN memberships ON chat_rooms.groupid = memberships.groupid "+
				"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid "+
				"WHERE memberships.userid = ? AND (memberships.role IN (?, ?) OR chat_rooms.user1 = ?) "+
				activeq+
				"AND chat_rooms.chattype = ? "+
				"AND (chat_roster.status IS NULL OR chat_roster.status != ?) "+
				"AND chat_rooms.latestmessage >= ?"+
				") AS combined",
				myid, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER, myid, utils.CHAT_TYPE_USER2MOD, utils.CHAT_STATUS_CLOSED, activeSince).Scan(&ids)
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
		// if I'm the member (user1), show "GroupName Volunteers".
		// If I'm a mod, show "MemberName on GroupName".
		if user1 == myid {
			if groupid > 0 {
				var nameshort string
				db.Raw("SELECT COALESCE(namefull, nameshort) FROM `groups` WHERE id = ?", groupid).Scan(&nameshort)
				if nameshort != "" {
					return nameshort + " Volunteers"
				}
			}
		} else if user1 > 0 {
			var fullname string
			db.Raw("SELECT fullname FROM users WHERE id = ?", user1).Scan(&fullname)
			if fullname != "" {
				if groupid > 0 {
					var groupname string
					db.Raw("SELECT COALESCE(namefull, nameshort) FROM `groups` WHERE id = ?", groupid).Scan(&groupname)
					if groupname != "" {
						return fullname + " on " + groupname
					}
				}
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
