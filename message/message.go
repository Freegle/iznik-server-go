package message

import (
	"fmt"
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
}

type Tabler interface {
	TableName() string
}

func (MessageGroup) TableName() string {
	return "messages_groups"
}

type MessageGroup struct {
	ID          uint64    `json:"id" gorm:"primary_key"`
	Msgid       uint64    `json:"msgid"`
	Arrival     time.Time `json:"arrival"`
	Collection  string    `json:"collection"`
	Autoreposts uint      `json:"autoreposts"`
}

func (MessageAttachment) TableName() string {
	return "messages_attachments"
}

type MessageAttachment struct {
	ID        uint64 `json:"id" gorm:"primary_key"`
	Msgid     uint64 `json:"-"`
	Path      string `json:"path"`
	Paththumb string `json:"paththumb"`
}

func GetMessage(c *fiber.Ctx) error {
	id := c.Params("id")
	db := database.DBConn

	var message Message

	db.Debug().Preload("MessageGroups", func(db *gorm.DB) *gorm.DB {
		return db.Where("deleted = 0")
	}).Preload("MessageAttachments").Where("messages.id = ? AND messages.deleted IS NULL", id).Find(&message)

	// Protect anonymity of poster a bit.
	message.Lat, message.Lng = utils.Blur(message.Lat, message.Lng, utils.BLUR_USER)

	for i, a := range message.MessageAttachments {
		fmt.Printf("Attachment %+v\n", a)
		message.MessageAttachments[i].Path = "https://img_" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + strconv.FormatUint(a.ID, 10) + ".jpg"
		message.MessageAttachments[i].Paththumb = "https://timg_" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + strconv.FormatUint(a.ID, 10) + ".jpg"
	}
	// TODO Outcomes, replycount, daysago, url
	return c.JSON(message)
}
