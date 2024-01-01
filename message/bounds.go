package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"strconv"
	"time"
)

func Bounds(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)

	swlat, _ := strconv.ParseFloat(c.Query("swlat"), 32)
	swlng, _ := strconv.ParseFloat(c.Query("swlng"), 32)
	nelat, _ := strconv.ParseFloat(c.Query("nelat"), 32)
	nelng, _ := strconv.ParseFloat(c.Query("nelng"), 32)

	msgs := []MessageSummary{}

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

	// We want to include our own messages, so that it is less obvious if a message is delayed for approval and
	// hasn't made it into messages_spatial yet.
	start := time.Now().AddDate(0, 0, -utils.OPEN_AGE).Format("2006-01-02")

	db.Raw(""+
		"SELECT * FROM ("+
		"SELECT ST_Y(point) AS lat, "+
		"ST_X(point) AS lng, "+
		"messages_spatial.msgid AS id, "+
		"messages_spatial.successful, "+
		"messages_spatial.promised, "+
		"messages_spatial.groupid, "+
		"messages_spatial.msgtype AS type, "+
		"messages_spatial.arrival, "+
		"CASE WHEN messages_likes.msgid IS NULL THEN 1 ELSE 0 END AS unseen "+
		"FROM messages_spatial "+
		"INNER JOIN `groups` ON groups.id = messages_spatial.groupid "+
		"LEFT JOIN messages_likes ON messages_likes.msgid = messages_spatial.msgid AND messages_likes.userid = ? AND messages_likes.type = 'View' "+
		"WHERE ST_Contains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), point) "+
		"AND (CASE WHEN postvisibility IS NULL OR ST_Contains(postvisibility, ST_SRID(POINT(?, ?),?)) THEN 1 ELSE 0 END) = 1 "+
		"UNION "+
		"SELECT messages.lat, messages.lng, messages.id, "+
		"(CASE WHEN messages_outcomes.outcome IN (?, ?) THEN 1 ELSE 0 END) AS successful, "+
		"(CASE WHEN messages_promises.id IS NOT NULL THEN 1 ELSE 0 END) AS promised, "+
		"messages_groups.groupid, "+
		"messages.type,"+
		"messages_groups.arrival, "+
		"CASE WHEN messages_likes.msgid IS NULL THEN 1 ELSE 0 END AS unseen "+
		"FROM messages "+
		"INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
		"INNER JOIN `groups` ON groups.id = messages_groups.groupid "+
		"LEFT JOIN messages_outcomes ON messages_outcomes.msgid = messages.id "+
		"LEFT JOIN messages_promises ON messages_promises.msgid = messages.id "+
		"LEFT JOIN messages_likes ON messages_likes.msgid = messages.id AND messages_likes.userid = ? AND messages_likes.type = 'View' "+
		"WHERE fromuser = ? AND messages_groups.arrival >= ? AND "+
		"ST_Contains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), ST_SRID(POINT(messages.lng, messages.lat), ?)) "+
		"AND (CASE WHEN postvisibility IS NULL OR ST_Contains(postvisibility, ST_SRID(POINT(?, ?),?)) THEN 1 ELSE 0 END) = 1 "+
		"AND messages_outcomes.id IS NULL "+
		") t "+
		"ORDER BY unseen DESC, arrival DESC, id DESC;",
		myid,
		swlng, swlat,
		swlng, nelat,
		nelng, nelat,
		nelng, swlat,
		swlng, swlat,
		utils.SRID,
		latlng.Lng,
		latlng.Lat,
		utils.SRID,
		utils.OUTCOME_TAKEN,
		utils.OUTCOME_RECEIVED,
		myid,
		myid,
		start,
		swlng, swlat,
		swlng, nelat,
		nelng, nelat,
		nelng, swlat,
		swlng, swlat,
		utils.SRID,
		utils.SRID,
		latlng.Lng,
		latlng.Lat,
		utils.SRID,
	).Scan(&msgs)

	for ix, r := range msgs {
		// Protect anonymity of poster a bit.
		msgs[ix].Lat, msgs[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
	}

	return c.JSON(msgs)
}
