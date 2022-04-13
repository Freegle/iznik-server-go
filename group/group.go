package group

import (
	"encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"os"
	"strconv"
	"time"
)

const MODERATOR = "Moderator"
const OWNER = "Owner"

type Group struct {
	ID                   uint64          `json:"id" gorm:"primary_key"`
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
	GroupProfile         GroupProfile    `gorm:"ForeignKey:groupid" json:"-"`
	GroupProfileStr      string          `json:"profile"`
	Onmap                int             `json:"onmap"`
	Tagline              string          `json:"tagline"`
	Description          string          `json:"description"`
	Contactmail          string          `json:"contactmail"`
	Fundingtarget        int             `json:"fundingtarget"`
	Affiliationconfirmed time.Time
	Founded              time.Time
	GroupSponsors        []GroupSponsor   `gorm:"ForeignKey:groupid" json:"sponsors"`
	GroupVolunteers      []GroupVolunteer `gorm:"ForeignKey:groupid" json:"showmods"`
}

// TODO modsemail

func GetGroup(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}

	db := database.DBConn
	var group Group

	if !db.Debug().Preload("GroupProfile").Preload("GroupSponsors").Where("id = ? AND publish = 1 AND onhere = 1 AND type = 'Freegle'", id).Find(&group).RecordNotFound() {

		group.GroupProfileStr = "https://" + os.Getenv("USER_SITE") + "/gimg_" + strconv.FormatUint(group.GroupProfile.ID, 10) + ".jpg"

		if len(group.Namedisplay) > 0 {
			group.Namedisplay = group.Namedisplay
		} else {
			group.Namedisplay = group.Nameshort
		}

		fmt.Println("Get volunteers")
		group.GroupVolunteers = GetGroupVolunteers(id)
		fmt.Println("Got volunteers")
		return c.JSON(group)
	} else {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
}
