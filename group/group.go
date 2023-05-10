package group

import (
	"encoding/json"
	"errors"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"os"
	"strconv"
	"sync"
	"time"
)

const MODERATOR = "Moderator"
const OWNER = "Owner"
const FREEGLE = "Freegle"

// Full group details.
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
	Altlat               float32          `json:"altlat"`
	Altlng               float32          `json:"altlng"`
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
	GroupVolunteers      []GroupVolunteer `gorm:"-" json:"showmods"`
}

// Summary group details.
type GroupEntry struct {
	ID          uint64  `json:"id" gorm:"primary_key"`
	Nameshort   string  `json:"nameshort"`
	Namefull    string  `json:"namefull"`
	Namedisplay string  `json:"namedisplay"`
	Lat         float32 `json:"lat"`
	Lng         float32 `json:"lng"`
	Altlat      float32 `json:"altlat"`
	Altlng      float32 `json:"altlng"`
	Publish     int     `json:"publish"`
	Onmap       int     `json:"onmap"`
	Region      string  `json:"region"`
	Contactmail string  `json:"-"`
	Modsemail   string  `json:"modsemail"`
}

type RepostSettings struct {
	Offer    int `json:"offer"`
	Wanted   int `json:"wanted"`
	Max      int `json:"max"`
	Chaseups int `json:"chaseups"`
}

func GetGroup(c *fiber.Ctx) error {
	//time.Sleep(30 * time.Second)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}

	db := database.DBConn
	var group Group
	var volunteers []GroupVolunteer
	found := false

	// Get group and volunteers info in parallel for speed.
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		volunteers = GetGroupVolunteers(id)
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		// Return the group even if publish = 0 or onhere = 0 because they have the actual id, so they must really
		// want it.  This can happen if a user has a message on a group that is then set to publish = 0, for example.
		err := db.Preload("GroupProfile").Preload("GroupSponsors").Where("id = ? AND type = ?", id, FREEGLE).First(&group).Error
		found = !errors.Is(err, gorm.ErrRecordNotFound)

		if found {
			if group.GroupProfile.ID > 0 {
				group.GroupProfileStr = "https://" + os.Getenv("USER_SITE") + "/gimg_" + strconv.FormatUint(group.GroupProfile.ID, 10) + ".jpg"
			}

			if len(group.Namefull) > 0 {
				group.Namedisplay = group.Namefull
			} else {
				group.Namedisplay = group.Nameshort
			}

			if len(group.Contactmail) > 0 {
				group.Modsemail = group.Contactmail
			} else {
				group.Modsemail = group.Nameshort + "-volunteers@" + os.Getenv("GROUP_DOMAIN")
			}
		}
	}()

	wg.Wait()

	if found {
		group.GroupVolunteers = volunteers
		return c.JSON(group)
	} else {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}
}

func ListGroups(c *fiber.Ctx) error {
	db := database.DBConn

	var groups []GroupEntry

	db.Raw("SELECT id, nameshort, namefull, lat, lng, onmap, publish, region, contactmail FROM `groups` WHERE publish = 1 AND onhere = 1 AND type = ?", FREEGLE).Scan(&groups)

	for ix, group := range groups {
		if len(group.Namefull) > 0 {
			groups[ix].Namedisplay = group.Namefull
		} else {
			groups[ix].Namedisplay = group.Nameshort
		}

		if len(group.Contactmail) > 0 {
			groups[ix].Modsemail = group.Contactmail
		} else {
			groups[ix].Modsemail = group.Nameshort + "-volunteers@" + os.Getenv("GROUP_DOMAIN")
		}
	}

	return c.JSON(groups)
}
