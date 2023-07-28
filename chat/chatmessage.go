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
	Reportreason       *string         `json:"reportreason"`
	Processingrequired bool            `json:"processingrequired"`
	Addressid          *uint64         `json:"addressid" gorm:"-"`
}

type ChatAttachment struct {
	ID        uint64 `json:"id" gorm:"-"`
	Path      string `json:"path"`
	Paththumb string `json:"paththumb"`
}

type LoveJunk struct {
	Ljuserid   uint64
	Firstname  string
	Lastname   string
	ProfileURL string
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
						Path:      os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/mimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
						Paththumb: os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tmimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
					}
				} else {
					messages[ix].Image = &ChatAttachment{
						ID:        *a.Imageid,
						Path:      os.Getenv("IMAGE_DOMAIN") + "/mimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
						Paththumb: os.Getenv("IMAGE_DOMAIN") + "/tmimg_" + strconv.FormatUint(*a.Imageid, 10) + ".jpg",
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

	// Also parse out LoveJunk parametes.
	var lovejunk LoveJunk
	err = c.BodyParser(&lovejunk)

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

	if lovejunk.Ljuserid != 0 {
		// ljuserid is the LoveJunk unique id for the user creating the message.  We need to
		// - create the user if they don't exist
		// - create a chat room if it doesn't exist
		// Then we can proceed to create the chat message.
		db.Raw("SELECT id FROM users WHERE ljuserid = ?", lovejunk.Ljuserid).Scan(&myid)
		fmt.Println("Got LJ user", myid)

		if myid == 0 {
			// We need to create the user.
			var newuser user.User
			newuser.Ljuserid = lovejunk.Ljuserid
			newuser.Firstname = lovejunk.Firstname
			newuser.Lastname = lovejunk.Lastname
			db.Create(&newuser)

			if newuser.ID == 0 {
				return fiber.NewError(fiber.StatusInternalServerError, "Failed to create user")
			}

			myid = newuser.ID

			// TODO Profile.
		}

		// Find the sender of the referenced message.
		var fromuser uint64
		db.Raw("SELECT fromuser AS FROM messages WHERE id = ?", payload.Refmsgid).Scan(&fromuser)
		fmt.Println("Got from user", fromuser)

		db.Raw("SELECT id FROM chat_rooms WHERE user1 = ? AND user2 = ? OR user1 = ? AND user2 = ?;", myid, fromuser, fromuser, myid).Scan(&payload.Chatid)
		fmt.Println("Got chat room", payload.Chatid)

		// TODO Banned in common and spammer check

		if payload.Chatid == 0 {
			// We need to create the chat.
			var newchat ChatRoomListEntry
			newchat.User1 = myid
			newchat.User2 = fromuser
			newchat.Chattype = utils.CHAT_TYPE_USER2USER

			db.Create(&newchat)

			// TODO Anything else- roster?

			if newchat.ID == 0 {
				return fiber.NewError(fiber.StatusInternalServerError, "Failed to create chat")
			}

			payload.Chatid = newchat.ID
		}
	} else {
		// FD passes the chatid.
		chatid := []ChatRoomListEntry{}

		db.Raw("SELECT id FROM chat_rooms WHERE id = ? AND user1 = ? "+
			"UNION SELECT id FROM chat_rooms WHERE id = ? AND user2 = ?", id, myid, id, myid).Scan(&chatid)

		if len(chatid) == 0 {
			return fiber.NewError(fiber.StatusNotFound, "Invalid chat id")
		}
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
