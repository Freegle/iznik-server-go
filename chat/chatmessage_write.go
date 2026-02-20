package chat

import (
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

type PatchChatMessageRequest struct {
	ID            uint64 `json:"id"`
	Roomid        uint64 `json:"roomid"`
	Replyexpected *bool  `json:"replyexpected"`
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

	// Operations require message ownership.
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
