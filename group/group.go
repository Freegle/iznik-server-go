package group

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"time"
)

type Group struct {
	gorm.Model
	id                   uint
	Nameshort            string          `json:"nameshort"`
	Namefull             string          `json:"namefull"`
	Namedisplay          string          `json:"namedisplay"`
	Settings             json.RawMessage `json:"settings"` // This is JSON stored in the DB as a string.
	Region               string          `json:"region"`
	Logo                 string          `json:"logo"`
	Publish              int             `json:"publish"`
	Ontn                 int             `json:"ontn"`
	Membercount          int             `json:"membercount"`
	Modcount             int             `json:"modcount"`
	Lat                  float32         `json:"lat"`
	Lng                  float32         `json:"lng"`
	Profile              string          `json:"profile"`
	Onmap                int             `json:"onmap"`
	Tagline              string          `json:"tagline"`
	Description          string          `json:"description"`
	Contactmail          string          `json:"contactmail"`
	Fundingtarget        int             `json:"fundingtarget"`
	Affiliationconfirmed time.Time
	Founded              time.Time
}

func GetGroup(c *fiber.Ctx) error {
	id := c.Params("id")
	db := database.DBConn
	var group Group
	db.Unscoped().Where("id = ? AND publish = 1 AND onhere = 1 AND type = 'Freegle'", id).Find(&group)
	return c.JSON(group)
}
