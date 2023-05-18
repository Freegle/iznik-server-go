package chat

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"os"
	"strconv"
	"time"
)

type ChatMessage struct {
	ID            uint64          `json:"id" gorm:"primary_key"`
	Chatid        uint64          `json:"chatid"`
	Userid        uint64          `json:"userid"`
	Type          string          `json:"type"`
	Refmsgid      uint64          `json:"refmsgid"`
	Refchatid     uint64          `json:"refchatid"`
	Imageid       uint64          `json:"imageid"`
	Image         *ChatAttachment `json:"image" gorm:"-"`
	Date          time.Time       `json:"date"`
	Message       string          `json:"message"`
	Seenbyall     bool            `json:"seenbyall"`
	Mailedtoall   bool            `json:"mailedtoall"`
	Replyexpected bool            `json:"replyexpected"`
	Replyreceived bool            `json:"replyreceived"`
	Archived      int             `json:"-"`
}

type ChatAttachment struct {
	ID        uint64 `json:"id" gorm:"-"`
	Path      string `json:"path"`
	Paththumb string `json:"paththumb"`
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
		// We can see this chat room. Don't return messages held for review unless we sent them.
		messages := []ChatMessage{}
		db.Raw("SELECT chat_messages.*, chat_images.archived FROM chat_messages "+
			"LEFT JOIN chat_images ON chat_images.id = chat_messages.imageid "+
			"WHERE chatid = ? AND (userid = ? OR (reviewrequired = 0 AND reviewrejected = 0)) ORDER BY date ASC", id, myid).Scan(&messages)

		// loop
		for ix, a := range messages {
			if a.Imageid > 0 {
				if a.Archived > 0 {
					messages[ix].Image = &ChatAttachment{
						ID:        a.Imageid,
						Path:      "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/mimg_" + strconv.FormatUint(a.Imageid, 10) + ".jpg",
						Paththumb: "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tmimg_" + strconv.FormatUint(a.Imageid, 10) + ".jpg",
					}
				} else {
					messages[ix].Image = &ChatAttachment{
						ID:        a.Imageid,
						Path:      "https://" + os.Getenv("IMAGE_DOMAIN") + "/mimg_" + strconv.FormatUint(a.Imageid, 10) + ".jpg",
						Paththumb: "https://" + os.Getenv("IMAGE_DOMAIN") + "/tmimg_" + strconv.FormatUint(a.Imageid, 10) + ".jpg",
					}
				}
			}
		}

		return c.JSON(messages)
	}

	return fiber.NewError(fiber.StatusNotFound, "Invalid chat id")
}
