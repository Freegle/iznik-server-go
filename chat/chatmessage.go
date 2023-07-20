package chat

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"os"
	"strconv"
	"time"
)

type ChatMessage struct {
	ID                 uint64          `json:"id" gorm:"primary_key"`
	Chatid             uint64          `json:"chatid"`
	Userid             uint64          `json:"userid"`
	Type               string          `json:"type"`
	Refmsgid           *uint64         `json:"refmsgid"`
	Refchatid          *uint64         `json:"refchatid"`
	Imageid            *uint64         `json:"imageid"`
	Image              *ChatAttachment `json:"image" gorm:"-"`
	Date               time.Time       `json:"date"`
	Message            string          `json:"message"`
	Seenbyall          bool            `json:"seenbyall"`
	Mailedtoall        bool            `json:"mailedtoall"`
	Replyexpected      bool            `json:"replyexpected"`
	Replyreceived      bool            `json:"replyreceived"`
	Archived           int             `json:"-" gorm:"-"`
	Reportreason       string          `json:"reportreason"`
	Processingrequired bool            `json:"processingrequired"`
	Addressid          *uint64         `json:"addressid" gorm:"-"`
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
			"LEFT JOIN chat_images ON chat_images.chatmsgid = chat_messages.id "+
			"WHERE chatid = ? AND (userid = ? OR (reviewrequired = 0 AND reviewrejected = 0 AND (processingrequired = 0 OR processingsuccessful = 1))) ORDER BY date ASC", id, myid).Scan(&messages)

		// loop
		for ix, a := range messages {
			if a.Imageid != nil {
				if a.Archived > 0 {
					messages[ix].Image = &ChatAttachment{
						ID:        *a.Imageid,
						Path:      "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/mimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
						Paththumb: "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tmimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
					}
				} else {
					messages[ix].Image = &ChatAttachment{
						ID:        *a.Imageid,
						Path:      "https://" + os.Getenv("IMAGE_DOMAIN") + "/mimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
						Paththumb: "https://" + os.Getenv("IMAGE_DOMAIN") + "/tmimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
					}
				}
			}
		}

		return c.JSON(messages)
	}

	return fiber.NewError(fiber.StatusNotFound, "Invalid chat id")
}

func CreateChatMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	db := database.DBConn
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid chat id")
	}

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var payload ChatMessage
	err = c.BodyParser(&payload)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
	}

	chattype := utils.CHAT_MESSAGE_DEFAULT

	if payload.Refmsgid != nil {
		chattype = utils.CHAT_MESSAGE_INTERESTED
	} else if payload.Refchatid != nil {
		chattype = utils.CHAT_MESSAGE_REPORTEDUSER
	} else if payload.Imageid != nil {
		chattype = utils.CHAT_MESSAGE_IMAGE
	} else if payload.Addressid != nil {
		chattype = utils.CHAT_MESSAGE_ADDRESS
		s := fmt.Sprint(*payload.Addressid)
		payload.Message = s
	} else if payload.Message == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Message must be non-empty")
	}

	chatid := []ChatRoomListEntry{}

	db.Raw("SELECT id FROM chat_rooms WHERE id = ? AND user1 = ? "+
		"UNION SELECT id FROM chat_rooms WHERE id = ? AND user2 = ?", id, myid, id, myid).Scan(&chatid)

	if len(chatid) == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Invalid chat id")
	}

	// We can see this chat room.  Create a chat message, but flagged as needing processing.  That means it
	// will only show up to the user who sent it until it is fully processed.
	payload.Userid = myid
	payload.Chatid = id
	payload.Type = chattype
	payload.Processingrequired = true
	payload.Date = time.Now()
	db.Create(&payload)
	newid := payload.ID

	if newid == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Error creating chat message")
	}

	if payload.Imageid != nil {
		// Update the chat image to link it to this chat message.  This also stops it being purged in
		// purge_chats.
		db.Raw("UPDATE chat_images SET chatmsgid = ? WHERE id = ?;", newid, *payload.Imageid)
	}

	ret := struct {
		Id int64 `json:"id"`
	}{}
	ret.Id = int64(newid)

	return c.JSON(ret)
}
