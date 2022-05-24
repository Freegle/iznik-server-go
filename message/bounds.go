package message

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

func Bounds(c *fiber.Ctx) error {
	db := database.DBConn

	swlat := c.Query("swlat")
	swlng := c.Query("swlng")
	nelat := c.Query("nelat")
	nelng := c.Query("nelng")

	fmt.Println("Bounds", swlat, swlng, nelat, nelng)

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
		"WHERE ST_Contains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), point) "+
		" ORDER BY messages_spatial.arrival DESC, messages_spatial.msgid DESC;",
		swlng, swlat,
		swlng, nelat,
		nelng, nelat,
		nelng, swlat,
		swlng, swlat,
		utils.SRID).Scan(&msgs)

	// TODO groupid parameter
	// TODO Filter by group visibility setting.  Check number returned vs existing code.

	for ix, r := range msgs {
		// Protect anonymity of poster a bit.
		msgs[ix].Lat, msgs[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
	}

	return c.JSON(msgs)
}
