package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"time"
)

func (Isochrone) TableName() string {
	return "isochrones"
}

type Isochrone struct {
	ID         uint64    `json:"id" gorm:"primary_key"`
	Locationid uint64    `json:"locationid"`
	Transport  string    `json:"transport"`
	Minutes    int       `json:"minutes"`
	Timestamp  time.Time `json:"timestamp"`
}

type IsochronesUsers struct {
	ID          uint64    `json:"id" gorm:"primary_key"`
	Userid      uint64    `json:"userid"`
	Isochroneid uint64    `json:"isochroneid"`
	Isochrone   Isochrone `gorm:"ForeignKey:isochroneid" json:"isochrone"`
}

type MessagesSpatial struct {
	ID         uint64    `json:"id" gorm:"primary_key"`
	Msgid      uint64    `json:"msgid"`
	Successful bool      `json:"successful"`
	Promised   bool      `json:"promised"`
	Groupid    uint64    `json:"groupid"`
	Type       string    `json:"type"`
	Arrival    time.Time `json:"arrival"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
}

func Isochrones(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	var isochrones []IsochronesUsers

	if !db.Preload("Isochrone").Where("userid = ?", myid).Find(&isochrones).RecordNotFound() {
		// We've got the isochrones for this user.  We want to find the message ids in each.

		if len(isochrones) > 0 {
			var res []MessagesSpatial

			// TODO parallelise.
			for _, isochrone := range isochrones {
				var msgs []MessagesSpatial

				db.Raw("SELECT ST_Y(point) AS lat, "+
					"ST_X(point) AS lng, "+
					"messages_spatial.msgid AS id, "+
					"messages_spatial.successful, "+
					"messages_spatial.promised, "+
					"messages_spatial.groupid, "+
					"messages_spatial.msgtype AS type, "+
					"messages_spatial.arrival "+
					"FROM messages_spatial "+
					"INNER JOIN isochrones ON ST_Contains(isochrones.polygon, point) "+
					"WHERE isochrones.id = ? ORDER BY messages_spatial.arrival DESC, messages_spatial.msgid DESC;", isochrone.ID).Scan(&msgs)

				res = append(res, msgs...)
			}

			// TODO Filter by group visibility setting.  Check number returned vs existing code.

			for ix, r := range res {
				// Protect anonymity of poster a bit.
				res[ix].Lat, res[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
			}

			return c.JSON(res)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Isochrone not found")
}
