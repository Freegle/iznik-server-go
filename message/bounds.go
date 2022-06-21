package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"strconv"
)

func Bounds(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)

	swlat, _ := strconv.ParseFloat(c.Query("swlat"), 32)
	swlng, _ := strconv.ParseFloat(c.Query("swlng"), 32)
	nelat, _ := strconv.ParseFloat(c.Query("nelat"), 32)
	nelng, _ := strconv.ParseFloat(c.Query("nelng"), 32)

	var msgs []MessagesSpatial

	// The optional postvisibility property of a group indicates the area within which members must lie for a post
	// on that group to be visibile.
	var latlng utils.LatLng

	if myid > 0 {
		latlng = user.GetLatLng(myid)
	} else {
		// Not logged in.  Best guess is centre of wherever they're looking.
		latlng.Lat = float32((swlat + nelat)) / 2
		latlng.Lng = float32((swlng + nelng)) / 2
	}

	db.Raw("SELECT ST_Y(point) AS lat, "+
		"ST_X(point) AS lng, "+
		"messages_spatial.msgid AS id, "+
		"messages_spatial.successful, "+
		"messages_spatial.promised, "+
		"messages_spatial.groupid, "+
		"messages_spatial.msgtype AS type, "+
		"messages_spatial.arrival "+
		"FROM messages_spatial "+
		"INNER JOIN `groups` ON groups.id = messages_spatial.groupid "+
		"WHERE ST_Contains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), point) "+
		"AND (CASE WHEN postvisibility IS NULL OR ST_Contains(postvisibility, ST_SRID(POINT(?, ?),?)) THEN 1 ELSE 0 END) = 1 "+
		"ORDER BY messages_spatial.arrival DESC, messages_spatial.msgid DESC;",
		swlng, swlat,
		swlng, nelat,
		nelng, nelat,
		nelng, swlat,
		swlng, swlat,
		utils.SRID).Scan(&msgs)

	// TODO groupid parameter

	for ix, r := range msgs {
		// Protect anonymity of poster a bit.
		msgs[ix].Lat, msgs[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
	}

	return c.JSON(msgs)
}
