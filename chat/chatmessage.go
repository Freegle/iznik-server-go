package chat

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"strconv"
	"time"
)

type ChatMessage struct {
	ID            uint64    `json:"id" gorm:"primary_key"`
	Chatid        uint64    `json:"chatid"`
	Userid        uint64    `json:"userid"`
	Type          string    `json:"type"`
	Refmsgid      uint64    `json:"refmsgid"`
	Refchatid     uint64    `json:"refchatid"`
	Imageid       uint64    `json:"imageid"`
	Date          time.Time `json:"date"`
	Message       string    `json:"message"`
	Seenbyall     bool      `json:"seenbyall"`
	Mailedtoall   bool      `json:"mailedtoall"`
	Replyexpected bool      `json:"replyexpected"`
	Replyreceived bool      `json:"replyreceived"`
}

func GetChatMessages(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	db := database.DBConn

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid chat id")
	}

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	_, err2 := GetChatRoom(id, myid)

	if !err2 {
		// We can see this chat room.
		var messages []ChatMessage
		db.Raw("SELECT * FROM chat_messages WHERE chatid = ? ORDER BY date ASC", id).Scan(&messages)
		return c.JSON(messages)
	}

	return fiber.NewError(fiber.StatusNotFound, "Invalid chat id")
}
