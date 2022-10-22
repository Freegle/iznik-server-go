package volunteering

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
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
	Groups         []uint64           `json:"groups"`
	Image          *VolunteeringImage `json:"image"`
	Dates          []VolunteeringDate `json:"dates"`
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

	db.Raw("SELECT volunteering.id FROM volunteering_groups "+
		"INNER JOIN volunteering ON volunteering.id = volunteering_groups.volunteeringid "+
		"LEFT JOIN volunteering_dates ON volunteering.id = volunteering_dates.volunteeringid "+
		"WHERE (groupid IS NULL OR groupid IN (?)) AND "+
		"(applyby IS NULL OR applyby >= ?) AND (end IS NULL OR end >= ?) AND deleted = 0 AND expired = 0 AND pending = 0 "+
		"ORDER BY id ASC", groupids, start, start).Pluck("volunteeringid", &ids)

	return c.JSON(ids)
}

func Single(c *fiber.Ctx) error {
	var wg sync.WaitGroup
	var volunteering Volunteering
	var image VolunteeringImage
	var found bool
	var groups []uint64
	var dates []VolunteeringDate
	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	userSite := os.Getenv("USER_SITE")

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err == nil {

		db := database.DBConn

		wg.Add(1)

		go func() {
			defer wg.Done()

			found = !db.Where("id = ? AND pending = 0 AND deleted = 0 AND heldby IS NULL", id).Find(&volunteering).RecordNotFound()

		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT id, archived FROM volunteering_images WHERE opportunityid = ? ORDER BY id DESC LIMIT 1", id).Scan(&image)

			if image.ID > 0 {
				if image.Archived > 0 {
					image.Path = "https://" + archiveDomain + "/oimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + archiveDomain + "/toimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				} else {
					image.Path = "https://" + userSite + "/oimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + userSite + "/toimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
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
