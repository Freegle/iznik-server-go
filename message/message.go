package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"os"
	"regexp"
	"strconv"
	"time"
)

type Message struct {
	ID                 uint64              `json:"id" gorm:"primary_key"`
	Arrival            time.Time           `json:"arrival"`
	Date               time.Time           `json:"date"`
	Fromuser           uint64              `json:"-"`
	FromuserObj        user.User           `json:"fromuser" gorm:"-"`
	Subject            string              `json:"subject"`
	Type               string              `json:"type"`
	Textbody           string              `json:"textbody"`
	Lat                float64             `json:"lat"`
	Lng                float64             `json:"lng"`
	Availablenow       uint                `json:"availablenow"`
	Availableinitially uint                `json:"availableinitially"`
	MessageGroups      []MessageGroup      `gorm:"ForeignKey:msgid" json:"groups"`
	MessageAttachments []MessageAttachment `gorm:"ForeignKey:msgid" json:"attachments"`
	MessageOutcomes    []MessageOutcome    `gorm:"ForeignKey:msgid" json:"outcomes"`
	MessageReply       []MessageReply      `gorm:"ForeignKey:refmsgid" json:"replies"`
	Replycount         int                 `json:"replycount"`
	MessageURL         string              `json:"url"`
}

func GetMessage(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	id := c.Params("id")
	db := database.DBConn

	var message Message

	if !db.Preload("MessageGroups", func(db *gorm.DB) *gorm.DB {
		if myid != 0 {
			// Can see own messages even if they are still pending.
			return db.Where("deleted = 0")
		} else {
			// Only showing approved messages.
			return db.Where("collection = ? AND deleted = 0", utils.COLLECTION_APPROVED)
		}
	}).Preload("MessageAttachments", func(db *gorm.DB) *gorm.DB {
		return db.Order("id ASC")
	}).Preload("MessageOutcomes").Preload("MessageReply", func(db *gorm.DB) *gorm.DB {
		// Only chat responses from users (not reports or anything else).
		return db.Where("type = ?", utils.MESSAGE_INTERESTED)
	}).Where("messages.id = ? AND messages.deleted IS NULL", id).Find(&message).RecordNotFound() {
		message.Replycount = len(message.MessageReply)
		message.MessageURL = "https://" + os.Getenv("USER_SITE") + "/message/" + strconv.FormatUint(message.ID, 10)

		// Protect anonymity of poster a bit.
		message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

		// Remove confidential info.
		var er = regexp.MustCompile(utils.EMAIL_REGEXP)
		message.Textbody = er.ReplaceAllString(message.Textbody, "***@***.com")
		var ep = regexp.MustCompile(utils.PHONE_REGEXP)
		message.Textbody = ep.ReplaceAllString(message.Textbody, "***")

		// Get the paths.
		for i, a := range message.MessageAttachments {
			if a.Archived > 0 {
				message.MessageAttachments[i].Path = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
				message.MessageAttachments[i].Paththumb = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
			} else {
				message.MessageAttachments[i].Path = "https://" + os.Getenv("USER_SITE") + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
				message.MessageAttachments[i].Paththumb = "https://" + os.Getenv("USER_SITE") + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
			}
		}

		message.FromuserObj = user.GetUserById(message.Fromuser, myid)

		return c.JSON(message)
	} else {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
}

func GetMessagesForUser(c *fiber.Ctx) error {
	db := database.DBConn

	if c.Params("id") != "" {
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			var msgs []MessageSummary

			db.Raw("SELECT lat, lng, messages.id, groupid, type, messages_groups.arrival, "+
				"EXISTS(SELECT id FROM messages_outcomes WHERE messages_outcomes.msgid = messages.id AND outcome IN (?, ?)) AS successful, "+
				"EXISTS(SELECT id FROM messages_promises WHERE messages_promises.msgid = messages.id) AS promised "+
				"FROM messages "+
				"INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
				"WHERE fromuser = ? "+
				"ORDER BY messages_groups.arrival DESC", utils.TAKEN, utils.RECEIVED, id).Scan(&msgs)

			for ix, r := range msgs {
				// Protect anonymity of poster a bit.
				msgs[ix].Lat, msgs[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
			}

			return c.JSON(msgs)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "User not found")
}
