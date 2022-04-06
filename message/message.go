package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"os"
	"strconv"
	"time"
)

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
}

func GetMessage(c *fiber.Ctx) error {
	id := c.Params("id")
	db := database.DBConn

	var message Message

	db.Preload("MessageGroups", func(db *gorm.DB) *gorm.DB {
		return db.Where("deleted = 0")
	}).Preload("MessageAttachments").Preload("MessageOutcomes").Preload("MessageReply", func(db *gorm.DB) *gorm.DB {
		return db.Where("type = 'Interested'")
	}).Where("messages.id = ? AND messages.deleted IS NULL", id).Find(&message)

	message.Replycount = len(message.MessageReply)

	// Protect anonymity of poster a bit.
	message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

	// Get the paths.
	for i, a := range message.MessageAttachments {
		message.MessageAttachments[i].Path = "https://img_" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + strconv.FormatUint(a.ID, 10) + ".jpg"
		message.MessageAttachments[i].Paththumb = "https://timg_" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + strconv.FormatUint(a.ID, 10) + ".jpg"
	}

	// TODO daysago, url
	// TODO Mask phone numbers etc.

	return c.JSON(message)
}
