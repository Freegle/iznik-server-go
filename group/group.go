package group

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"time"
)

type Group struct {
	gorm.Model
	id                   uint
	Nameshort            string  `json:"nameshort"`
	Namefull             string  `json:"namefull"`
	Namedisplay          string  `json:"namedisplay"`
	Settings             string  `json:"settings"` // TODO decode
	Type                 string  `json:"type"`
	Region               string  `json:"region"`
	Logo                 string  `json:"logo"`
	Publish              int     `json:"publish"`
	Onhere               int     `json:"onhere"`
	Ontn                 int     `json:"ontn"`
	Membercount          int     `json:"membercount"`
	Modcount             int     `json:"modcount"`
	Lat                  float32 `json:"lat"`
	Lng                  float32 `json:"lng"`
	Profile              string  `json:"profile"`
	Cover                string  `json:"cover"`
	Onmap                int     `json:"onmap"`
	Tagline              string  `json:"tagline"`
	Description          string  `json:"description"`
	Contactmail          string  `json:"contactmail"`
	Fundingtarget        int     `json:"fundingtarget"`
	Affiliationconfirmed time.Time
	//'affiliationconfirmedby',
	//'mentored',
	//'privategroup',
	//'defaultlocation',
	//'moderationstatus',
	//'maxagetoshow',
	//'nearbygroups',
	//'microvolunteering',
	//'microvolunteeringoptions',
	//'autofunctionoverride',
	//'overridemoderation',
	//'precovidmoderated'

}

func GetGroup(c *fiber.Ctx) {
	id := c.Params("id")
	db := database.DBConn
	var group Group
	db.Debug().Unscoped().Find(&group, id)
	c.JSON(group)
}
