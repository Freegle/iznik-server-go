package newsfeed

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	geo "github.com/kellydunn/golang-geo"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (Newsfeed) TableName() string {
	return "newsfeed"
}

type NewsImage struct {
	ID        uint64 `json:"id"`
	Path      string `json:"path"`
	PathThumb string `json:"paththumb"`
}

type NewsLove struct {
	Newsfeedid uint64    `json:"newsfeedid"`
	Userid     uint64    `json:"userid"`
	Timestamp  time.Time `json:"timestamp"`
}

type NewsfeedSummary struct {
	ID     uint64     `json:"id" gorm:"primary_key"`
	Userid uint64     `json:"userid"`
	Hidden *time.Time `json:"hidden"`
}

type Newsfeed struct {
	ID             uint64           `json:"id" gorm:"primary_key"`
	Threadhead     uint64           `json:"threadhead"`
	Timestamp      time.Time        `json:"timestamp"`
	Added          time.Time        `json:"added"`
	Type           string           `json:"type"`
	Userid         uint64           `json:"userid"`
	Displayname    string           `json:"displayname"`
	Profile        user.UserProfile `json:"profile"`
	Info           user.UserInfo    `json:"userinfo"`
	Showmod        bool             `json:"showmod"`
	Location       string           `json:"location"`
	Imageid        uint64           `json:"imageid"`
	Imagearchived  bool             `json:"-"`
	Image          *NewsImage       `json:"image"`
	Msgid          uint64           `json:"msgid"`
	Replyto        uint64           `json:"replyto"`
	Groupid        uint64           `json:"groupid"`
	Eventid        uint64           `json:"eventid"`
	Volunteeringid uint64           `json:"volunteeringid"`
	Publicityid    uint64           `json:"publicityid"`
	Storyid        uint64           `json:"storyid"`
	Message        string           `json:"message"`
	Html           string           `json:"html"`
	Pinned         bool             `json:"pinned"`
	Hidden         *time.Time       `json:"hidden"`
	Deleted        *time.Time       `json:"deleted"`
	Loves          int64            `json:"loves"`
	Loved          bool             `json:"loved"`
	Replies        []Newsfeed       `json:"replies"`
	Lovelist       []NewsLove       `json:"lovelist"`
}

func GetNearbyDistance(uid uint64) (float64, utils.LatLng, float64, float64, float64, float64) {
	// We want to calculate a distance which includes at least some other people who have posted a message.
	// Start at fairly close and keep doubling until we reach that, or get too far away.
	//
	// Because this is Go we can fire off these requests in parallel and just stop when we get enough results.
	// This reduces latency significantly, even though it's a bit mean to the database server.
	var mu sync.Mutex
	var wg sync.WaitGroup
	done := false

	dist := float64(1)
	ret := float64(0)
	var retnelat, retnelng, retswlat, retswlng float64

	max := float64(248)
	count := 0

	for {
		if dist >= max {
			break
		}

		dist *= 2
		count++
	}

	dist = 1
	limit := 10
	now := time.Now()
	then := now.AddDate(0, 0, -31)

	latlng := user.GetLatLng(uid)

	if latlng.Lat > 0 || latlng.Lng > 0 {
		type Nearby struct {
			Userid uint64 `json:"userid"`
		}

		db := database.DBConn

		wg.Add(1)

		for {
			go func(dist float64) {
				var nelat, nelng, swlat, swlng float64
				var nearbys []Nearby

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

				mu.Lock()
				defer mu.Unlock()

				if !done {
					count--

					if len(nearbys) >= limit || count == 0 {
						// Either we found enough or we have finished looking.  Either way, stop and take the best we
						// have found.
						ret = dist
						retnelat = nelat
						retnelng = nelng
						retswlat = swlat
						retswlng = swlng
						done = true
						defer wg.Done()
					}
				}
			}(dist)

			dist *= 2

			if dist >= max {
				break
			}
		}
	}

	return ret, latlng, retnelat, retnelng, retswlat, retswlng
}

func Feed(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var err error
	var gotDistance bool
	var distance uint64

	gotDistance = false

	if c.Query("distance") != "" {
		distance, err = strconv.ParseUint(c.Query("distance"), 10, 32)

		if err == nil {
			gotDistance = true
		}
	}

	// We want the whole feed.
	//
	// Get:
	// - the distance we want to show.
	// - the current user to check mod status
	// - the feed
	var nelat, nelng, swlat, swlng float64

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		if !gotDistance {
			// We need to calculate a reasonable feed distance to show.
			_, _, nelat, nelng, swlat, swlng = GetNearbyDistance(myid)
		} else if distance > 0 {
			// We've been given a distance.
			latlng := user.GetLatLng(myid)

			// Get a bounding box for the distance.
			p := geo.NewPoint(float64(latlng.Lat), float64(latlng.Lng))
			ne := p.PointAtDistanceAndBearing(float64(distance), 45)
			nelat = ne.Lat()
			nelng = ne.Lng()
			sw := p.PointAtDistanceAndBearing(float64(distance), 225)
			swlat = sw.Lat()
			swlng = sw.Lng()
		}
	}()

	var me user.User

	wg.Add(1)
	go func() {
		defer wg.Done()
		// get user
		db := database.DBConn
		db.First(&me, myid)
	}()

	wg.Wait()

	var newsfeed []NewsfeedSummary

	// Get the top-level threads, i.e. replyto IS NULL.
	// TODO Crashes if we don't have a limit clause.  Why?
	if distance > 0 {
		db.Raw("SELECT newsfeed.id, newsfeed.userid, newsfeed.hidden FROM newsfeed "+
			"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
			"WHERE MBRContains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), position) AND "+
			"replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 AND "+
			"newsfeed.type NOT IN (?) "+
			"ORDER BY pinned DESC, newsfeed.timestamp DESC LIMIT 100;",
			myid,
			swlng, swlat,
			swlng, nelat,
			nelng, nelat,
			nelng, swlat,
			swlng, swlat,
			utils.SRID,
			utils.NEWSFEED_TYPE_CENTRAL_PUBLICITY,
		).Scan(&newsfeed)
	} else {
		db.Raw("SELECT newsfeed.id, newsfeed.userid, newsfeed.hidden FROM newsfeed "+
			"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
			"WHERE replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 AND "+
			"newsfeed.type NOT IN (?) "+
			"ORDER BY pinned DESC, newsfeed.timestamp DESC LIMIT 100;",
			myid,
			utils.NEWSFEED_TYPE_CENTRAL_PUBLICITY,
		).Scan(&newsfeed)
	}

	amAMod := me.Systemrole != "User"

	var ret []NewsfeedSummary

	for i := 0; i < len(newsfeed); i++ {
		if newsfeed[i].Hidden != nil {
			if newsfeed[i].Userid == myid || amAMod {
				// Don't use hidden entries unless they are ours.  This means that to a spammer or suppressed user
				// it looks like their posts are there but nobody else sees them.
				//
				// Mods can see hidden items.
				if !amAMod {
					newsfeed[i].Hidden = nil
				}

				ret = append(ret, newsfeed[i])
			}
		} else {
			// TODO Don't return volunteering/events/stories if they are still pending.
			ret = append(ret, newsfeed[i])
		}
	}

	return c.JSON(ret)
}

func Single(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	lovelist, _ := strconv.ParseBool(c.Query("lovelist", "false"))

	if err == nil {
		// Get a single thread.
		var wg sync.WaitGroup
		var newsfeed Newsfeed
		var replies = []Newsfeed{}

		wg.Add(1)
		go func() {
			defer wg.Done()
			newsfeed, _ = fetchSingle(id, myid, lovelist)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			replies = fetchReplies(id, myid, id)
		}()

		wg.Wait()

		if newsfeed.ID > 0 {
			newsfeed.Replies = replies
			return c.JSON(newsfeed)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Newsfeed item not found")
}

func fetchSingle(id uint64, myid uint64, lovelist bool) (Newsfeed, bool) {
	db := database.DBConn

	var newsfeed Newsfeed
	var loves int64
	var loved bool

	loverlist := []NewsLove{}

	newsfeed.Replies = []Newsfeed{}
	newsfeed.Lovelist = []NewsLove{}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		db.Raw("SELECT newsfeed.*, newsfeed_images.archived AS imagearchived, "+
			"CASE WHEN users.fullname IS NOT NULL THEN users.fullname ELSE CONCAT(users.firstname, ' ', users.lastname) END AS displayname, "+
			"CASE WHEN systemrole IN ('Moderator', 'Support', 'Admin') THEN CASE WHEN JSON_EXTRACT(users.settings, '$.showmod') IS NULL THEN 1 ELSE JSON_EXTRACT(users.settings, '$.showmod') END ELSE 0 END AS showmod "+
			"FROM newsfeed "+
			"LEFT JOIN users ON users.id = newsfeed.userid "+
			"LEFT JOIN newsfeed_images ON newsfeed.imageid = newsfeed_images.id WHERE newsfeed.id = ?;", id).Scan(&newsfeed)

		if newsfeed.Imageid > 0 {
			if newsfeed.Imagearchived {
				newsfeed.Image = &NewsImage{
					ID:        newsfeed.Imageid,
					Path:      "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/fimg_" + strconv.FormatUint(newsfeed.Imageid, 10) + ".jpg",
					PathThumb: "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tfimg_" + strconv.FormatUint(newsfeed.Imageid, 10) + ".jpg",
				}
			} else {
				newsfeed.Image = &NewsImage{
					ID:        newsfeed.Imageid,
					Path:      "https://" + os.Getenv("USER_SITE") + "/fimg_" + strconv.FormatUint(newsfeed.Imageid, 10) + ".jpg",
					PathThumb: "https://" + os.Getenv("USER_SITE") + "/tfimg_" + strconv.FormatUint(newsfeed.Imageid, 10) + ".jpg",
				}
			}
		}

		var wg2 sync.WaitGroup

		wg2.Add(2)

		var info user.UserInfo
		var profileRecord user.UserProfileRecord

		go func() {
			defer wg2.Done()
			info = user.GetUserInfo(newsfeed.Userid, myid)
		}()

		go func() {
			defer wg2.Done()
			profileRecord = user.GetProfileRecord(newsfeed.Userid)
		}()

		wg2.Wait()

		newsfeed.Info = info

		if profileRecord.Useprofile {
			user.ProfileSetPath(profileRecord.Profileid, profileRecord.Url, profileRecord.Archived, &newsfeed.Profile)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Get count of loves.
		db.Raw("SELECT COUNT(*) FROM newsfeed_likes WHERE newsfeedid = ?", id).Row().Scan(&loves)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Get any loves by us
		db.Raw("SELECT COUNT(*) FROM newsfeed_likes WHERE newsfeedid = ? AND userid = ?", id, myid).Row().Scan(&loved)
	}()

	if lovelist {
		wg.Add(1)
		go func() {
			defer wg.Done()

			db.Raw("SELECT * FROM newsfeed_likes WHERE newsfeedid = ?", id).Scan(&loverlist)
		}()
	}

	wg.Wait()

	if newsfeed.ID > 0 {
		// Don't return the hidden field when fetching an individual item.  We have that in the feed, and it
		// saves calls.
		newsfeed.Hidden = nil
		newsfeed.Loved = loved
		newsfeed.Loves = loves
		newsfeed.Lovelist = loverlist
		newsfeed.Message = strings.TrimSpace(newsfeed.Message)
		newsfeed.Displayname = strings.TrimSpace(newsfeed.Displayname)

		if newsfeed.Replyto == 0 {
			newsfeed.Threadhead = newsfeed.ID
		}

		return newsfeed, false
	} else {
		return newsfeed, true
	}
}

func fetchReplies(id uint64, myid uint64, threadhead uint64) []Newsfeed {
	db := database.DBConn

	var replies = []Newsfeed{}

	type ReplyId struct {
		ID uint64 `json:"id"`
	}

	var replyids []ReplyId
	var mu sync.Mutex

	db.Raw("SELECT id FROM newsfeed WHERE replyto = ? ORDER BY timestamp ASC", id).Scan(&replyids)

	var wg sync.WaitGroup

	for i := 0; i < len(replyids); i++ {
		// Fetch the replies
		wg.Add(1)
		go func(replyid uint64) {
			defer wg.Done()
			reply, err := fetchSingle(replyid, myid, false)

			if !err {
				reply.Threadhead = threadhead

				mu.Lock()
				defer mu.Unlock()
				replies = append(replies, reply)
			}
		}(replyids[i].ID)

		// Fetch any replies to the replies (which in turn will fetch replies to those).
		wg.Add(1)
		go func(replyid uint64) {
			defer wg.Done()

			repliestoreplies := fetchReplies(replyid, myid, threadhead)
			mu.Lock()
			defer mu.Unlock()

			// Add these replies to the entry in replies with the correct ID.
			for j := 0; j < len(replies); j++ {
				if replies[j].ID == replyid {
					replies[j].Replies = repliestoreplies
				}
			}
		}(replyids[i].ID)
	}

	wg.Wait()

	// Sort replies by ascending timestamp.
	sort.Slice(replies, func(i, j int) bool {
		return replies[i].Timestamp.Before(replies[j].Timestamp)
	})

	return replies
}

func Count(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var wg sync.WaitGroup

	var nelat, nelng, swlat, swlng float64
	var latlng utils.LatLng
	var dist float64

	// We only want to count items within the nearby area for the user, even if they have a larger area selected.
	wg.Add(1)
	go func() {
		defer wg.Done()
		dist, latlng, nelat, nelng, swlat, swlng = GetNearbyDistance(myid)
	}()

	// Get the last one seen if any.  Getting this makes the query below better indexed.
	var lastseen uint64

	wg.Add(1)
	go func() {
		defer wg.Done()

		db.Raw("SELECT newsfeedid FROM newsfeed_users WHERE userid = ?;", myid).Row().Scan(&lastseen)
	}()

	wg.Wait()

	p := geo.NewPoint(float64(latlng.Lat), float64(latlng.Lng))
	ne := p.PointAtDistanceAndBearing(dist, 45)
	nelat = ne.Lat()
	nelng = ne.Lng()
	sw := p.PointAtDistanceAndBearing(dist, 225)
	swlat = sw.Lat()
	swlng = sw.Lng()

	now := time.Now()
	then := now.AddDate(0, 0, -7)

	type NewsCount struct {
		Count uint64 `json:"count"`
	}

	var newscount NewsCount

	db.Raw("SELECT COUNT(DISTINCT(newsfeed.id)) AS count FROM newsfeed "+
		"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
		"LEFT JOIN newsfeed_users ON newsfeed_users.newsfeedid = newsfeed.id AND newsfeed_users.userid = ? "+
		"WHERE newsfeed.id > ? AND "+
		"(MBRContains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), position) OR `type` IN (?, ?)) AND "+
		"replyto IS NULL AND newsfeed.timestamp >= ?;",
		myid,
		myid,
		lastseen,
		swlng, swlat,
		swlng, nelat,
		nelng, nelat,
		nelng, swlat,
		swlng, swlat,
		utils.SRID,
		utils.NEWSFEED_TYPE_CENTRAL_PUBLICITY,
		utils.NEWSFEED_TYPE_ALERT,
		then,
	).Scan(&newscount)

	return c.JSON(newscount)
}
