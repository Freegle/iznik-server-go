package chat

import (
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

type PutChatRoomRequest struct {
	Userid uint64 `json:"userid"`
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

	if req.Userid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "userid is required")
	}

	if req.Userid == myid {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot create a chat with yourself")
	}

	db := database.DBConn
	now := time.Now()

	// Check for existing chat first (covers both user orderings).
	var existingID uint64
	db.Raw("SELECT id FROM chat_rooms WHERE ((user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)) AND chattype = ? LIMIT 1",
		myid, req.Userid, req.Userid, myid, utils.CHAT_TYPE_USER2USER).Scan(&existingID)

	if existingID > 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": existingID})
	}

	// Use INSERT ... ON DUPLICATE KEY UPDATE to handle concurrent creation atomically,
	// matching V1 ChatRoom::createConversation(). The unique key (user1, user2, chattype)
	// ensures only one row exists; LAST_INSERT_ID(id) returns the existing row's ID on conflict.
	db.Exec("INSERT INTO chat_rooms (user1, user2, chattype, latestmessage) VALUES (?, ?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), latestmessage = VALUES(latestmessage)",
		myid, req.Userid, utils.CHAT_TYPE_USER2USER, now)

	var chatID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&chatID)

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

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": chatID})
}
