package isochrone

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

func Messages(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	var isochrones []IsochronesUsers

	if !db.Preload("Isochrone").Where("userid = ?", myid).Find(&isochrones).RecordNotFound() {
		// We've got the isochrones for this user.  We want to find the message ids in each.
		if len(isochrones) > 0 {
			var res []message.MessagesSpatial

			// TODO parallelise.
			for _, isochrone := range isochrones {
				var msgs []message.MessagesSpatial

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
