package message

import (
	"fmt"
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

const INTERESTED = "Interested"

const EMAIL_REGEXP = "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}\b"
const PHONE_REGEXP = "[0-9]{4,}"

type Message struct {
	ID                 uint64              `json:"id" gorm:"primary_key"`
	Arrival            time.Time           `json:"arrival"`
	Date               time.Time           `json:"date"`
	Fromuser           uint                `json:"fromuser"`
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

// This provides enough information about a message to display a summary ont he browse page.
func GetMessage(c *fiber.Ctx) error {
	id := c.Params("id")
	db := database.DBConn

	var message Message

	var myid = user.WhoAmI(c)
	fmt.Println("Logged in user %d", myid)

	if !db.Preload("MessageGroups", func(db *gorm.DB) *gorm.DB {
		// Only showing approved messages.
		// TODO This means you can't see your own.
		return db.Where("collection = ? AND deleted = 0", APPROVED)
	}).Preload("MessageAttachments", func(db *gorm.DB) *gorm.DB {
		// Return the most recent image only.
		return db.Order("id ASC").Limit(1)
	}).Preload("MessageOutcomes").Preload("MessageReply", func(db *gorm.DB) *gorm.DB {
		// Only chat responses from users (not reports or anything else).
		return db.Where("type = ?", INTERESTED)
	}).Where("messages.id = ? AND messages.deleted IS NULL", id).Find(&message).RecordNotFound() {
		message.Replycount = len(message.MessageReply)
		message.MessageURL = "https://" + os.Getenv("USER_SITE") + "/message/" + strconv.FormatUint(message.ID, 10)

		// Protect anonymity of poster a bit.
		message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

		// Remove confidential info.
		var er = regexp.MustCompile(EMAIL_REGEXP)
		message.Textbody = er.ReplaceAllString(message.Textbody, "***@***.com")
		var ep = regexp.MustCompile(PHONE_REGEXP)
		message.Textbody = ep.ReplaceAllString(message.Textbody, "***")

		// Get the paths.
		for i, a := range message.MessageAttachments {
			message.MessageAttachments[i].Path = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/img_" + strconv.FormatUint(a.ID, 10) + ".jpg"
			message.MessageAttachments[i].Paththumb = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/timg_" + strconv.FormatUint(a.ID, 10) + ".jpg"
		}

		return c.JSON(message)
	} else {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
}
