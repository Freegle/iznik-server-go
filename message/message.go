package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"time"
)

type Message struct {
	gorm.Model
	Id                 uint `json:"id"`
	Arrival            time.Time
	Date               time.Time
	Fromuser           int
	Replyto            string
	Subject            string
	Type               string
	Textbody           string
	Lat                float32
	Lng                float32
	Availablenow       int
	Availableinitially int
	MessageGroups      []MessageGroup `gorm:"ForeignKey:msgid" json:"groups"`
}

type Tabler interface {
	TableName() string
}

func (MessageGroup) TableName() string {
	return "messages_groups"
}

type MessageGroup struct {
	gorm.Model
	Id          uint `json:"id"`
	Arrival     time.Time
	Collection  string
	Autoreposts uint
}

func GetMessage(c *fiber.Ctx) error {
	id := c.Params("id")
	db := database.DBConn
	var message Message
	var groups []MessageGroup
	// TODO Can't get preloading to work.
	//db.Debug().Unscoped().Preload("MessageGroups").Where("messages.id = ? AND messages.deleted IS NULL", id).Find(&message)
	db.Debug().Unscoped().Where("msgid = ? AND deleted = 0", id).Find(&groups)
	db.Debug().Unscoped().Where("messages.id = ? AND messages.deleted IS NULL", id).Find(&message)
	message.MessageGroups = groups

	return c.JSON(message)
}
