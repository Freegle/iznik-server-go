package volunteering

import (
	"github.com/freegle/iznik-server-go/database"
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
	Image          VolunteeringImage  `json:"image"`
	Dates          []VolunteeringDate `json:"dates"`
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
			volunteering.Image = image
			volunteering.Groups = groups
			volunteering.Dates = dates

			return c.JSON(volunteering)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Not found")

}
