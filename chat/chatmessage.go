package chat

import (
	"encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"os"
	"strconv"
	"strings"
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
	Reportreason       *string         `json:"reportreason"`
	Processingrequired bool            `json:"processingrequired"`
	Addressid          *uint64         `json:"addressid" gorm:"-"`
	Archived           int             `json:"-" gorm:"-"`
	Externaluid        string          `json:"-" gorm:"-"`
	Externalmods       json.RawMessage `json:"-" gorm:"-"`
}

type ChatAttachment struct {
	ID           uint64          `json:"id" gorm:"-"`
	Path         string          `json:"path"`
	Paththumb    string          `json:"paththumb"`
	Externaluid  string          `json:"externaluid"`
	Ouruid       string          `json:"ouruid"` // Temp until Uploadcare retired.
	Externalmods json.RawMessage `json:"externalmods"`
}

type ChatMessageLovejunk struct {
	Refmsgid     *uint64 `json:"refmsgid"`
	Partnerkey   string  `json:"partnerkey"`
	Message      string  `json:"message"`
	Ljuserid     *uint64 `json:"ljuserid" gorm:"-"`
	Firstname    *string `json:"firstname" gorm:"-"`
	Lastname     *string `json:"lastname" gorm:"-"`
	Profileurl   *string `json:"profileurl" gorm:"-"`
	Initialreply bool    `json:"initialreply" gorm:"-"`
	Offerid      *uint64 `json:"offerid" gorm:"-"`
}

type ChatMessageLovejunkResponse struct {
	Id     uint64 `json:"id"`
	Chatid uint64 `json:"chatid"`
}

func (ChatRosterEntry) TableName() string {
	return "chat_roster"
}

type ChatRosterEntry struct {
	Id             uint64     `json:"id"`
	Chatid         uint64     `json:"chatid"`
	Userid         uint64     `json:"userid"`
	Date           *time.Time `json:"date"`
	Status         string     `json:"status"`
	Lastmsgseen    *uint64    `json:"lastmsgseen"`
	Lastemailed    *time.Time `json:"lastemailed"`
	Lastmsgemailed *uint64    `json:"lastmsgemailed"`
	Lastip         *string    `json:"lastip"`
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
		// We can see this chat room. Don't return messages:
		// - held for review unless we sent them
		// - for deleted users unless that's us.
		messages := []ChatMessage{}
		db.Raw("SELECT chat_messages.*, chat_images.archived, chat_images.externaluid, chat_images.externalmods FROM chat_messages "+
			"LEFT JOIN chat_images ON chat_images.chatmsgid = chat_messages.id "+
			"INNER JOIN users ON users.id = chat_messages.userid "+
			"WHERE chatid = ? AND (userid = ? OR (reviewrequired = 0 AND reviewrejected = 0 AND processingsuccessful = 1)) "+
			"AND (users.deleted IS NULL OR users.id = ?) "+
			"ORDER BY date ASC", id, myid, myid).Scan(&messages)

		// loop
		for ix, a := range messages {
			if a.Imageid != nil {
				if a.Externaluid != "" {
					// Until Uploadcare is retired we need to return different variants to allow for client code
					// which doesn't yet know about our own image hosting.
					if strings.Contains(a.Externaluid, "freegletusd-") {
						messages[ix].Image = &ChatAttachment{
							ID:           *a.Imageid,
							Ouruid:       a.Externaluid,
							Externalmods: a.Externalmods,
							Path:         misc.GetImageDeliveryUrl(a.Externaluid, string(a.Externalmods)),
							Paththumb:    misc.GetImageDeliveryUrl(a.Externaluid, string(a.Externalmods)),
						}
					} else {
						messages[ix].Image = &ChatAttachment{
							ID:           *a.Imageid,
							Externaluid:  a.Externaluid,
							Externalmods: a.Externalmods,
							Path:         misc.GetUploadcareUrl(a.Externaluid, string(a.Externalmods)),
							Paththumb:    misc.GetUploadcareUrl(a.Externaluid, string(a.Externalmods)),
						}
					}
				} else if a.Archived > 0 {
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
		db.Exec("UPDATE chat_images SET chatmsgid = ? WHERE id = ?;", newid, *payload.Imageid)
	}

	ret := struct {
		Id int64 `json:"id"`
	}{}
	ret.Id = int64(newid)

	return c.JSON(ret)
}

func CreateChatMessageLoveJunk(c *fiber.Ctx) error {
	var payload ChatMessageLovejunk
	err := c.BodyParser(&payload)

	if err != nil || payload.Ljuserid == nil || payload.Partnerkey == "" || payload.Refmsgid == nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
	}

	err2, myid := user.GetLoveJunkUser(*payload.Ljuserid, payload.Partnerkey, payload.Firstname, payload.Lastname)

	if err2.Code != fiber.StatusOK {
		return err2
	}

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Find the user who sent the message we are replying to.
	type msgInfo struct {
		Fromuser uint64
		Groupid  uint64
	}

	var m msgInfo

	db.Raw("SELECT fromuser, groupid FROM messages "+
		"INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
		"INNER JOIN users ON users.id = messages.fromuser "+
		"WHERE messages.id = ? AND users.deleted IS NULL", payload.Refmsgid).Scan(&m)

	if m.Fromuser == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Invalid message id")
	}

	// Ensure we're a member of the group.  This may fail if we're banned.
	if !user.AddMembership(myid, m.Groupid, utils.ROLE_MEMBER, utils.COLLECTION_APPROVED, utils.FREQUENCY_NEVER, 0, 0, "LoveJunk user joining to reply") {
		return fiber.NewError(fiber.StatusForbidden, "Failed to join relevant group")
	}

	// Find the chat between m.Fromuser and myid
	var chat ChatRoom
	db.Raw("SELECT * FROM chat_rooms WHERE user1 = ? AND user2 = ?", myid, m.Fromuser).Scan(&chat)

	if chat.ID == 0 {
		// We don't yet have a chat.  We need to create one.
		chat.User1 = myid
		chat.User2 = m.Fromuser
		chat.Chattype = utils.CHAT_TYPE_USER2USER
		db.Create(&chat)

		if chat.ID == 0 {
			return fiber.NewError(fiber.StatusInternalServerError, "Error creating chat")
		}

		// We also need to add both users into the roster for the chat (which is what will trigger replies to come
		// back to us).
		var roster ChatRosterEntry
		roster.Chatid = chat.ID
		roster.Userid = myid
		roster.Status = utils.CHAT_STATUS_ONLINE
		now := time.Now()
		roster.Date = &now
		db.Create(&roster)

		if roster.Id == 0 {
			return fiber.NewError(fiber.StatusInternalServerError, "Error creating roster entry")
		}

		var roster2 ChatRosterEntry
		roster2.Chatid = chat.ID
		roster2.Userid = m.Fromuser
		roster2.Date = &now
		roster2.Status = utils.CHAT_STATUS_AWAY
		db.Create(&roster2)

		if roster2.Id == 0 {
			return fiber.NewError(fiber.StatusInternalServerError, "Error creating roster entry2")
		}
	}

	if payload.Offerid != nil {
		// Update the offer id in the chat room, which we need to be able to send back replies.  LoveJunk only allows
		// one offer per Freegle user and hence this can be stored in the chat room.
		db.Exec("UPDATE chat_rooms SET ljofferid = ? WHERE id = ?", *payload.Offerid, chat.ID)
	}

	var chattype string

	if payload.Initialreply {
		chattype = utils.CHAT_MESSAGE_INTERESTED
	} else {
		chattype = utils.CHAT_MESSAGE_DEFAULT
	}

	if payload.Message == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Message must be non-empty")
	}

	// Create a chat message, but flagged as needing processing.
	var cm ChatMessage
	cm.Userid = myid
	cm.Chatid = chat.ID
	cm.Type = chattype
	cm.Processingrequired = true
	cm.Date = time.Now()
	cm.Message = payload.Message
	cm.Refmsgid = payload.Refmsgid
	db.Create(&cm)
	newid := cm.ID

	if newid == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Error creating chat message")
	}

	// TODO Images?

	var ret ChatMessageLovejunkResponse
	ret.Id = newid
	ret.Chatid = chat.ID

	return c.JSON(ret)
}
