package chat

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"strconv"
)

type PatchChatMessageRequest struct {
	ID            uint64  `json:"id"`
	Roomid        uint64  `json:"roomid"`
	Replyexpected *bool   `json:"replyexpected"`
	RSVP          *string `json:"rsvp"`
}

func PatchChatMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchChatMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 || req.Roomid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id and roomid are required")
	}

	db := database.DBConn

	// RSVP update: user just needs to be a member of the chat, not the message author.
	if req.RSVP != nil {
		return handleRSVP(c, db, myid, req)
	}

	// Other operations require message ownership.
	var msgUserid uint64
	db.Raw("SELECT userid FROM chat_messages WHERE id = ? AND chatid = ?", req.ID, req.Roomid).Scan(&msgUserid)

	if msgUserid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	if msgUserid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	// Update replyexpected if provided.
	if req.Replyexpected != nil {
		db.Exec("UPDATE chat_messages SET replyexpected = ? WHERE id = ?", *req.Replyexpected, req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleRSVP(c *fiber.Ctx, db *gorm.DB, myid uint64, req PatchChatMessageRequest) error {
	rsvp := *req.RSVP

	// Validate RSVP value.
	if rsvp != "Yes" && rsvp != "No" && rsvp != "Maybe" {
		return fiber.NewError(fiber.StatusBadRequest, "RSVP must be Yes, No, or Maybe")
	}

	// Verify message exists and is in the specified chat room.
	type msgInfo struct {
		Chatid uint64
		Type   string
	}
	var msg msgInfo
	db.Raw("SELECT chatid, type FROM chat_messages WHERE id = ? AND chatid = ?", req.ID, req.Roomid).Scan(&msg)

	if msg.Chatid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	// Verify user is a member of this chat.
	var room ChatRoom
	db.Raw("SELECT id, chattype, user1, user2 FROM chat_rooms WHERE id = ?", msg.Chatid).Scan(&room)
	if room.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Chat not found")
	}
	if room.User1 != myid && room.User2 != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not a member of this chat")
	}

	// Update the RSVP on the message.
	db.Exec("UPDATE chat_messages SET rsvp = ? WHERE id = ?", rsvp, req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func DeleteChatMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	idStr := c.Query("id")
	if idStr == "" {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid id")
	}

	db := database.DBConn

	// Verify the message exists and belongs to this user.
	var msgUserid uint64
	db.Raw("SELECT userid FROM chat_messages WHERE id = ?", id).Scan(&msgUserid)

	if msgUserid == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	if msgUserid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your message")
	}

	// Soft-delete: set type to Default, deleted to 1, clear imageid, remove chat_images.
	db.Exec("UPDATE chat_messages SET type = ?, deleted = 1, imageid = NULL WHERE id = ?",
		utils.CHAT_MESSAGE_DEFAULT, id)
	db.Exec("DELETE FROM chat_images WHERE chatmsgid = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
