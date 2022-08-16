package newsfeed

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	geo "github.com/kellydunn/golang-geo"
	"time"
)

type Newsfeed struct {
	ID             uint64     `json:"id" gorm:"primary_key"`
	Timestamp      time.Time  `json:"timestamp"`
	Added          time.Time  `json:"added"`
	Type           string     `json:"type"`
	Userid         uint64     `json:"userid"`
	Imageid        uint64     `json:"imageid"`
	Msgid          uint64     `json:"msgid"`
	Replyto        uint64     `json:"replyto"`
	Groupid        uint64     `json:"groupid"`
	Eventid        uint64     `json:"eventid"`
	Volunteeringid uint64     `json:"volunteeringid"`
	Publicityid    uint64     `json:"publicityid"`
	Storyid        uint64     `json:"storyid"`
	Message        string     `json:"message"`
	Html           string     `json:"html"`
	Pinned         bool       `json:"pinned"`
	Hidden         *time.Time `json:"hidden"`
}

func GetNearbyDistance(uid uint64) (float64, utils.LatLng, float64, float64, float64, float64) {
	// We want to calculate a distance which includes at least some other people who have posted a message.
	// Start at fairly close and keep doubling until we reach that, or get too far away.
	dist := float64(1)
	limit := 10
	max := float64(248)
	now := time.Now()
	then := now.AddDate(0, 0, -31)

	var nelat, nelng, swlat, swlng float64

	latlng := user.GetLatLng(uid)

	if latlng.Lat > 0 || latlng.Lng > 0 {
		type Nearby struct {
			Userid uint64 `json:"userid"`
		}

		var nearbys []Nearby

		db := database.DBConn

		for {
			p := geo.NewPoint(float64(latlng.Lat), float64(latlng.Lng))
			ne := p.PointAtDistanceAndBearing(dist, 45)
			nelat = ne.Lat()
			nelng = ne.Lng()
			sw := p.PointAtDistanceAndBearing(dist, 225)
			swlat = sw.Lat()
			swlng = sw.Lng()

			db.Raw("SELECT DISTINCT userid FROM newsfeed FORCE INDEX (position) WHERE "+
				"MBRContains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), position) AND "+
				"replyto IS NULL AND type != ? AND timestamp >= ? LIMIT ?;",
				swlng, swlat,
				swlng, nelat,
				nelng, nelat,
				nelng, swlat,
				swlng, swlat,
				utils.SRID,
				utils.NEWSFEED_TYPE_ALERT,
				then,
				limit+1).Scan(&nearbys)

			if dist >= max || len(nearbys) > limit {
				break
			}

			dist *= 2
		}
	}

	return dist, latlng, nelat, nelng, swlat, swlng
}

func Feed(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var amAMod bool

	_, _, nelat, nelng, swlat, swlng := GetNearbyDistance(myid)

	var newsfeed []Newsfeed
	var ret []Newsfeed

	db := database.DBConn

	// Get the top-level threads, i.e. replyto IS NULL.
	db.Raw("SELECT newsfeed.id, newsfeed.timestamp, newsfeed.added, newsfeed.type, newsfeed.userid, "+
		"newsfeed.imageid, newsfeed.msgid, newsfeed.replyto, newsfeed.groupid, newsfeed.eventid, "+
		"newsfeed.volunteeringid, newsfeed.publicityid, newsfeed.storyid, newsfeed.message, "+
		"newsfeed.html, newsfeed.pinned, newsfeed_unfollow.id AS unfollowed, newsfeed.hidden FROM newsfeed "+
		"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
		"WHERE MBRContains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), position) AND "+
		"replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 "+
		"ORDER BY pinned DESC, timestamp DESC;",
		myid,
		swlng, swlat,
		swlng, nelat,
		nelng, nelat,
		nelng, swlat,
		swlng, swlat,
		utils.SRID,
	).Scan(&newsfeed)

	for i := 0; i < len(newsfeed); i++ {
		if newsfeed[i].Hidden != nil {
			if newsfeed[i].Userid == myid || amAMod {
				// Don't use hidden entries unless they are ours.  This means that to a spammer or suppressed user
				// it looks like their posts are there but nobody else sees them.
				//
				// Mods can see hidden items.
				ret = append(ret, newsfeed[i])
			}
		} else {
			// TODO Don't return volunteering/events/stories if they are still pending.
			ret = append(ret, newsfeed[i])
		}
	}

	return c.JSON(ret)
}
