package isochrone

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"sync"
)

type IsochronesUsers struct {
	ID          uint64 `json:"id" gorm:"primary_key"`
	Userid      uint64 `json:"userid"`
	Isochroneid uint64 `json:"isochroneid"`
}

func Messages(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	var isochrones []IsochronesUsers
	res := []message.MessageSummary{}

	// The optional postvisibility property of a group indicates the area within which members must lie for a post
	// on that group to be visible.
	latlng := user.GetLatLng(myid)

	db.Where("userid = ?", myid).Find(&isochrones)
	if len(isochrones) > 0 {
		// We've got the isochrones for this user.  We want to find the message ids in each.
		// We might have multiple - if so then get them in parallel.
		var mu sync.Mutex

		var wg sync.WaitGroup

		for _, isochrone := range isochrones {
			wg.Add(1)

			go func(isochrone IsochronesUsers) {
				defer wg.Done()

				msgs := []message.MessageSummary{}

				db.Raw("SELECT ST_Y(point) AS lat, "+
					"ST_X(point) AS lng, "+
					"messages_spatial.msgid AS id, "+
					"messages_spatial.successful, "+
					"messages_spatial.promised, "+
					"messages_spatial.groupid, "+
					"messages_spatial.msgtype AS type, "+
					"messages_spatial.arrival, "+
					"CASE WHEN messages_likes.msgid IS NULL THEN 1 ELSE 0 END AS unseen "+
					"FROM messages_spatial "+
					"INNER JOIN isochrones ON ST_Contains(isochrones.polygon, point) "+
					"INNER JOIN `groups` ON groups.id = messages_spatial.groupid "+
					"LEFT JOIN messages_likes ON messages_likes.msgid = messages_spatial.msgid AND messages_likes.userid = ? AND messages_likes.type = 'View' "+
					"WHERE isochrones.id = ? "+
					"AND (CASE WHEN postvisibility IS NULL OR ST_Contains(postvisibility, ST_SRID(POINT(?, ?),?)) THEN 1 ELSE 0 END) = 1 "+
					"ORDER BY unseen DESC, messages_spatial.arrival DESC, messages_spatial.msgid DESC;", myid, isochrone.Isochroneid, latlng.Lng, latlng.Lat, utils.SRID).Scan(&msgs)

				mu.Lock()
				defer mu.Unlock()
				res = append(res, msgs...)
			}(isochrone)
		}

		wg.Wait()

		for ix, r := range res {
			// Protect anonymity of poster a bit.
			res[ix].Lat, res[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
		}
	}

	return c.JSON(res)
}

func Count(c *fiber.Ctx) error {
	db := database.DBConn
	myid := user.WhoAmI(c)

	var count uint64 = 0

	browseView := c.Query("browseView", "nearby")

	if browseView == "mygroups" {
		db.Raw("SELECT COUNT(DISTINCT(messages_spatial.msgid)) FROM memberships "+
			"INNER JOIN messages_spatial ON messages_spatial.groupid = memberships.groupid "+
			"LEFT JOIN messages_likes ON messages_likes.msgid = messages_spatial.msgid AND messages_likes.userid = ? AND messages_likes.type = 'View' "+
			"WHERE memberships.userid = ? AND messages_spatial.successful = 0 AND messages_likes.msgid IS NULL", myid, myid).Scan(&count)
	} else {
		count = isochroneCount(myid)
	}

	return c.JSON(fiber.Map{
		"count": count,
	})
}

func isochroneCount(myid uint64) uint64 {
	db := database.DBConn

	var isochrones []IsochronesUsers
	res := uint64(0)

	latlng := user.GetLatLng(myid)

	db.Where("userid = ?", myid).Find(&isochrones)

	if len(isochrones) > 0 {
		var mu sync.Mutex

		var wg sync.WaitGroup

		for _, isochrone := range isochrones {
			wg.Add(1)

			go func(isochrone IsochronesUsers) {
				defer wg.Done()

				thiscount := uint64(0)

				db.Raw("SELECT COUNT(DISTINCT(messages_spatial.msgid)) "+
					"FROM messages_spatial "+
					"INNER JOIN isochrones ON ST_Contains(isochrones.polygon, point) "+
					"INNER JOIN `groups` ON groups.id = messages_spatial.groupid "+
					"LEFT JOIN messages_likes ON messages_likes.msgid = messages_spatial.msgid AND messages_likes.userid = ? AND messages_likes.type = 'View' "+
					"WHERE isochrones.id = ? AND messages_spatial.successful = 0 "+
					"AND (CASE WHEN postvisibility IS NULL OR ST_Contains(postvisibility, ST_SRID(POINT(?, ?),?)) THEN 1 ELSE 0 END) = 1 "+
					"AND messages_likes.msgid IS NULL;", myid, isochrone.Isochroneid, latlng.Lng, latlng.Lat, utils.SRID).Scan(&thiscount)

				mu.Lock()
				defer mu.Unlock()
				res += thiscount
			}(isochrone)
		}

		wg.Wait()
	}

	return res
}
