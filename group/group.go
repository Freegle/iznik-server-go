package group

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"os"
	"strconv"
	"time"
)

const MODERATOR = "Moderator"
const OWNER = "Owner"
const FREEGLE = "Freegle"

type Group struct {
	ID                   uint64           `json:"id" gorm:"primary_key"`
	Nameshort            string           `json:"nameshort"`
	Namefull             string           `json:"namefull"`
	Namedisplay          string           `json:"namedisplay"`
	Settings             json.RawMessage  `json:"settings"` // This is JSON stored in the DB as a string.
	Region               string           `json:"region"`
	Logo                 string           `json:"logo"`
	Publish              int              `json:"publish"`
	Ontn                 int              `json:"ontn"`
	Membercount          int              `json:"membercount"`
	Modcount             int              `json:"modcount"`
	Lat                  float32          `json:"lat"`
	Lng                  float32          `json:"lng"`
	GroupProfile         GroupProfile     `gorm:"ForeignKey:groupid" json:"-"`
	GroupProfileStr      string           `json:"profile"`
	Onmap                int              `json:"onmap"`
	Tagline              string           `json:"tagline"`
	Description          string           `json:"description"`
	Contactmail          string           `json:"-"`
	Modsemail            string           `json:"modsemail"`
	Fundingtarget        int              `json:"fundingtarget"`
	Affiliationconfirmed time.Time        `json:"affiliationconfirmed"`
	Founded              time.Time        `json:"founded"`
	GroupSponsors        []GroupSponsor   `gorm:"ForeignKey:groupid" json:"sponsors"`
	GroupVolunteers      []GroupVolunteer `gorm:"ForeignKey:groupid" json:"showmods"`
}

type GroupEntry struct {
	ID          uint64 `json:"id" gorm:"primary_key"`
	Nameshort   string `json:"nameshort"`
	Namefull    string `json:"namefull"`
	Namedisplay string `json:"namedisplay"`
}

func GetGroup(c *fiber.Ctx) error {
	//time.Sleep(30 * time.Second)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}

	db := database.DBConn
	var group Group

	if !db.Preload("GroupProfile").Preload("GroupSponsors").Where("id = ? AND publish = 1 AND onhere = 1 AND type = ?", id, FREEGLE).Find(&group).RecordNotFound() {

		group.GroupProfileStr = "https://" + os.Getenv("USER_SITE") + "/gimg_" + strconv.FormatUint(group.GroupProfile.ID, 10) + ".jpg"

		if len(group.Namedisplay) > 0 {
			group.Namedisplay = group.Namedisplay
		} else {
			group.Namedisplay = group.Nameshort
		}

		if len(group.Contactmail) > 0 {
			group.Modsemail = group.Contactmail
		} else {
			group.Modsemail = group.Nameshort + "-volunteers@" + os.Getenv("GROUP_DOMAIN")
		}

		group.GroupVolunteers = GetGroupVolunteers(id)

		return c.JSON(group)
	} else {
		return fiber.NewError(fiber.StatusNotFound, "Message not found")
	}
}

func ListGroups(c *fiber.Ctx) error {
	db := database.DBConn

	var groups []GroupEntry

	db.Raw("SELECT id, nameshort, namefull FROM `groups` WHERE publish = 1 AND onhere = 1 AND type = ?", FREEGLE).Scan(&groups)

	for ix, group := range groups {
		if len(group.Namefull) > 0 {
			groups[ix].Namedisplay = group.Namefull
		} else {
			groups[ix].Namedisplay = group.Nameshort
		}
	}

	return c.JSON(groups)
}
