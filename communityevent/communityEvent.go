package communityevent

import (
	"errors"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"os"
	"strconv"
	"sync"
	"time"
)

func (CommunityEvent) TableName() string {
	return "communityevents"
}

type CommunityEvent struct {
	ID             uint64               `json:"id" gorm:"primary_key"`
	Userid         uint64               `json:"userid"`
	Title          string               `json:"title"`
	Location       string               `json:"location"`
	Contactname    string               `json:"contactname"`
	Contactphone   string               `json:"contactphone"`
	Contactemail   string               `json:"contactemail"`
	Contacturl     string               `json:"contacturl"`
	Description    string               `json:"description"`
	Timecommitment string               `json:"timecommitment"`
	Added          time.Time            `json:"added"`
	Groups         []uint64             `json:"groups" gorm:"-"`
	Image          *CommunityEventImage `json:"image" gorm:"-"`
	Dates          []CommunityEventDate `json:"dates" gorm:"-"`
}

func List(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	memberships := user.GetMemberships(myid)
	var groupids []uint64

	for _, membership := range memberships {
		groupids = append(groupids, membership.Groupid)
	}

	var ids []uint64

	start := time.Now().Format("2006-01-02")

	db.Raw("SELECT DISTINCT communityevents.id FROM communityevents "+
		"LEFT JOIN communityevents_groups ON communityevents.id = communityevents_groups.eventid "+
		"LEFT JOIN communityevents_dates ON communityevents.id = communityevents_dates.eventid "+
		"WHERE (groupid IS NULL OR groupid IN (?)) AND "+
		"(end IS NULL OR end >= ?) AND deleted = 0 AND pending = 0 "+
		"ORDER BY id DESC", groupids, start).Pluck("eventid", &ids)

	if len(ids) > 0 {
		return c.JSON(ids)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}

func Single(c *fiber.Ctx) error {
	var wg sync.WaitGroup
	var communityevent CommunityEvent
	var image CommunityEventImage
	var found bool
	var groups []uint64
	var dates []CommunityEventDate
	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	imageDomain := os.Getenv("IMAGE_DOMAIN")

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err == nil {

		db := database.DBConn

		wg.Add(1)

		go func() {
			defer wg.Done()

			// Can always fetch a single one if we know the id, even if it's pending.
			err := db.Where("id = ? AND deleted = 0 AND heldby IS NULL", id).First(&communityevent).Error
			found = !errors.Is(err, gorm.ErrRecordNotFound)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT id, archived FROM communityevents_images WHERE eventid = ? ORDER BY id DESC LIMIT 1", id).Scan(&image)

			if image.ID > 0 {
				if image.Archived > 0 {
					image.Path = "https://" + archiveDomain + "/cimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + archiveDomain + "/tcimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				} else {
					image.Path = "https://" + imageDomain + "/cimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + imageDomain + "/tcimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				}
			}
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT groupid FROM communityevents_groups WHERE eventid = ?", id).Pluck("groupid", &groups)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT * FROM communityevents_dates WHERE eventid = ?", id).Scan(&dates)
		}()

		wg.Wait()

		if found {
			if image.ID > 0 {
				communityevent.Image = &image
			}

			communityevent.Groups = groups
			communityevent.Dates = dates

			return c.JSON(communityevent)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Not found")
}
