package volunteering

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

func (Volunteering) TableName() string {
	return "volunteering"
}

type Volunteering struct {
	ID             uint64             `json:"id" gorm:"primary_key"`
	Userid         uint64             `json:"userid"`
	Title          string             `json:"title"`
	Location       string             `json:"location"`
	Contactname    string             `json:"contactname"`
	Contactphone   string             `json:"contactphone"`
	Contactemail   string             `json:"contactemail"`
	Contacturl     string             `json:"contacturl"`
	Description    string             `json:"description"`
	Timecommitment string             `json:"timecommitment"`
	Added          time.Time          `json:"added"`
	Groups         []uint64           `json:"groups"  gorm:"-"`
	Image          *VolunteeringImage `json:"image" gorm:"-"`
	Dates          []VolunteeringDate `json:"dates" gorm:"-"`
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

	db.Raw("SELECT DISTINCT volunteering.id FROM volunteering "+
		"LEFT JOIN volunteering_groups ON volunteering.id = volunteering_groups.volunteeringid "+
		"LEFT JOIN volunteering_dates ON volunteering.id = volunteering_dates.volunteeringid "+
		"WHERE (groupid IS NULL OR groupid IN (?)) AND "+
		"(applyby IS NULL OR applyby >= ?) AND (end IS NULL OR end >= ?) AND deleted = 0 AND expired = 0 AND pending = 0 "+
		"ORDER BY id DESC", groupids, start, start).Pluck("volunteeringid", &ids)

	if len(ids) > 0 {
		return c.JSON(ids)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}

func ListGroup(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid groupid")
	}

	db := database.DBConn

	var ids []uint64

	start := time.Now().Format("2006-01-02")

	db.Raw("SELECT DISTINCT volunteering.id FROM volunteering "+
		"LEFT JOIN volunteering_groups ON volunteering.id = volunteering_groups.volunteeringid "+
		"LEFT JOIN volunteering_dates ON volunteering.id = volunteering_dates.volunteeringid "+
		"WHERE groupid = ? AND "+
		"(applyby IS NULL OR applyby >= ?) AND (end IS NULL OR end >= ?) AND deleted = 0 AND expired = 0 AND pending = 0 "+
		"ORDER BY id DESC", id, start, start).Pluck("volunteeringid", &ids)

	if len(ids) > 0 {
		return c.JSON(ids)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}

func Single(c *fiber.Ctx) error {
	var wg sync.WaitGroup
	var volunteering Volunteering
	var image VolunteeringImage
	var found bool
	var groups []uint64
	var dates []VolunteeringDate
	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	imageDomain := os.Getenv("IMAGE_DOMAIN")

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err == nil {
		db := database.DBConn

		wg.Add(1)

		go func() {
			defer wg.Done()

			// Can always fetch a single one if we know the id, even if it's pending.
			err := db.Where("id = ? AND deleted = 0 AND heldby IS NULL", id).First(&volunteering).Error
			found = !errors.Is(err, gorm.ErrRecordNotFound)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT id, archived FROM volunteering_images WHERE opportunityid = ? ORDER BY id DESC LIMIT 1", id).Scan(&image)

			if image.ID > 0 {
				if image.Archived > 0 {
					image.Path = archiveDomain + "/oimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = archiveDomain + "/toimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				} else {
					image.Path = imageDomain + "/oimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = imageDomain + "/toimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				}
			}
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT groupid FROM volunteering_groups WHERE volunteeringid = ?", id).Pluck("groupid", &groups)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT * FROM volunteering_dates WHERE volunteeringid = ?", id).Scan(&dates)
		}()

		wg.Wait()

		if found {
			if image.ID > 0 {
				volunteering.Image = &image
			}

			volunteering.Groups = groups
			volunteering.Dates = dates

			return c.JSON(volunteering)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Not found")
}
