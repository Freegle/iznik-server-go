package newsfeed

import (
	"context"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	geo "github.com/kellydunn/golang-geo"
	xurls "mvdan.cc/xurls/v2"
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
	ID                  uint64     `json:"id" gorm:"primary_key"`
	Userid              uint64     `json:"userid"`
	Hidden              *time.Time `json:"hidden"`
	Eventpending        bool       `json:"-"`
	Volunteeringpending bool       `json:"-"`
	Storypending        bool       `json:"-"`
}

func (NewsfeedPreview) TableName() string {
	return "link_previews"
}

type NewsfeedPreview struct {
	ID          uint64 `json:"id" gorm:"primary_key"`
	Url         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

type Newsfeed struct {
	ID             uint64            `json:"id" gorm:"primary_key"`
	Threadhead     uint64            `json:"threadhead"`
	Timestamp      time.Time         `json:"timestamp"`
	Added          time.Time         `json:"added"`
	Type           string            `json:"type"`
	Userid         uint64            `json:"userid"`
	Displayname    string            `json:"displayname"`
	Profile        user.UserProfile  `json:"profile" gorm:"-"`
	Info           user.UserInfo     `json:"userinfo" gorm:"-"`
	Showmod        bool              `json:"showmod"`
	Location       string            `json:"location"`
	Imageid        uint64            `json:"imageid"`
	Imagearchived  bool              `json:"-"`
	Image          *NewsImage        `json:"image" gorm:"-"`
	Msgid          uint64            `json:"msgid"`
	Replyto        uint64            `json:"replyto"`
	Groupid        uint64            `json:"groupid"`
	Eventid        uint64            `json:"eventid"`
	Volunteeringid uint64            `json:"volunteeringid"`
	Publicityid    uint64            `json:"publicityid"`
	Storyid        uint64            `json:"storyid"`
	Message        string            `json:"message"`
	Html           string            `json:"html"`
	Pinned         bool              `json:"pinned"`
	Hidden         *time.Time        `json:"hidden"`
	Deleted        *time.Time        `json:"deleted"`
	Loves          int64             `json:"loves"`
	Loved          bool              `json:"loved"`
	Replies        []Newsfeed        `json:"replies" gorm:"-"`
	Lovelist       []NewsLove        `json:"lovelist" gorm:"-"`
	Previews       []NewsfeedPreview `json:"previews" gorm:"-"`
}

func GetNearbyDistance(uid uint64) (float64, utils.LatLng, float64, float64, float64, float64) {
	// We want to calculate a distance which includes at least some other people who have posted a message.
	// Start at fairly close and keep doubling until we reach that, or get too far away.
	//
	// Because this is Go we can fire off these requests in parallel and just stop when we get enough results.
	// This reduces latency significantly, even though it's a bit mean to the database server.  To cancel the queries
	// properly we need to use the Pool.
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

	var cancels []context.CancelFunc

	if latlng.Lat > 0 || latlng.Lng > 0 {
		type Nearby struct {
			Userid uint64 `json:"userid"`
		}

		wg.Add(1)

		for {
			// Use a timeout context - partly so that we don't wait for too long, and partly so that we can
			// cancel queries if we get enough results.
			timeoutContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			cancels = append(cancels, cancel)

			go func(dist float64) {
				var nelat, nelng, swlat, swlng float64
				var nearbys []Nearby

				// Get an exclusive connection.
				db, err := database.Pool.Conn(timeoutContext)

				if err != nil {
					return
				}

				p := geo.NewPoint(float64(latlng.Lat), float64(latlng.Lng))
				ne := p.PointAtDistanceAndBearing(dist, 45)
				nelat = ne.Lat()
				nelng = ne.Lng()
				sw := p.PointAtDistanceAndBearing(dist, 225)
				swlat = sw.Lat()
				swlng = sw.Lng()

				nelats := fmt.Sprint(nelat)
				nelngs := fmt.Sprint(nelng)
				swlats := fmt.Sprint(swlat)
				swlngs := fmt.Sprint(swlng)

				sql := "SELECT DISTINCT userid FROM newsfeed FORCE INDEX (position) WHERE " +
					"MBRContains(ST_SRID(POLYGON(LINESTRING(" +
					"POINT(" + swlngs + ", " + swlats + "), " +
					"POINT(" + swlngs + ", " + nelats + "), " +
					"POINT(" + nelngs + ", " + nelats + "), " +
					"POINT(" + nelngs + ", " + swlats + "), " +
					"POINT(" + swlngs + ", " + swlats + "))), " + fmt.Sprint(utils.SRID) + "), position) AND " +
					"replyto IS NULL AND type != '" + utils.NEWSFEED_TYPE_ALERT + "' AND timestamp >= '" + then.Format("2006-01-02") +
					"' LIMIT " + fmt.Sprint(limit+1)

				rows, err := db.QueryContext(timeoutContext, sql)

				// Return the connection to the pool.
				defer db.Close()

				// We might be cancelled/timed out, in which case we have no rows to process.
				if err == nil {
					for rows.Next() {
						var nearby Nearby
						err = rows.Scan(&nearby.Userid)

						if err != nil {
							break
						}

						nearbys = append(nearbys, nearby)
					}
				}

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

	wg.Wait()

	// Cancel any outstanding ops.
	for _, cancel := range cancels {
		defer func() {
			go cancel()
		}()
	}

	return ret, latlng, retnelat, retnelng, retswlat, retswlng
}

func Feed(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var distance uint64
	var err error
	gotDistance := false

	if c.Query("distance") != "" && c.Query("distance") != "nearby" {
		distance, err = strconv.ParseUint(c.Query("distance"), 10, 32)

		if err == nil {
			gotDistance = true
		}
	}

	ret := getFeed(myid, gotDistance, distance)
	if len(ret) == 0 {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	} else {
		return c.JSON(ret)
	}
}

func getFeed(myid uint64, gotDistance bool, distance uint64) []NewsfeedSummary {
	db := database.DBConn

	var gotLatLng bool

	gotLatLng = false

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
			var reasonable float64
			reasonable, _, nelat, nelng, swlat, swlng = GetNearbyDistance(myid)

			if reasonable > 0 {
				gotLatLng = true
			}
		} else if distance > 0 {
			// We've been given a distance.
			latlng := user.GetLatLng(myid)

			if latlng.Lat != 0 && latlng.Lng != 0 {
				// Get a bounding box for the distance.
				p := geo.NewPoint(float64(latlng.Lat), float64(latlng.Lng))
				ne := p.PointAtDistanceAndBearing(float64(distance/1000), 45)
				nelat = ne.Lat()
				nelng = ne.Lng()
				sw := p.PointAtDistanceAndBearing(float64(distance/1000), 225)
				swlat = sw.Lat()
				swlng = sw.Lng()
				gotLatLng = true
			}
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

	// Get the top-level threads, i.e. replyto IS NULL.  Put a limit on the number of threads we get - we're
	// unlikely to scroll that far.
	//
	// We use a UNION to pick up the alerts, even though it makes the SQL longer, because it allows efficient
	// use of the spatial index.
	if gotLatLng {
		db.Raw(
			"(SELECT newsfeed.id, newsfeed.userid, (CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, pinned, newsfeed.timestamp, "+
				"(CASE WHEN communityevents.id IS NOT NULL AND communityevents.pending THEN 1 ELSE 0 END) AS eventpending,"+
				"(CASE WHEN volunteering.id IS NOT NULL AND volunteering.pending THEN 1 ELSE 0 END) AS volunteeringpending, "+
				"(CASE WHEN users_stories.id IS NOT NULL AND (users_stories.public = 0 OR users_stories.reviewed = 0) THEN 1 ELSE 0 END) AS storypending "+
				"FROM newsfeed FORCE INDEX (position) "+
				"LEFT JOIN users ON users.id = newsfeed.userid "+
				"LEFT JOIN spam_users ON spam_users.userid = newsfeed.userid AND collection IN ('PendingAdd', 'Spammer') "+
				"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
				"LEFT JOIN communityevents ON newsfeed.eventid = communityevents.id "+
				"LEFT JOIN volunteering ON newsfeed.volunteeringid = volunteering.id "+
				"LEFT JOIN users_stories ON newsfeed.storyid = users_stories.id "+
				"WHERE MBRContains(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?), position) AND "+
				"replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 AND "+
				"newsfeed.type NOT IN (?) "+
				"AND users.deleted IS NULL "+
				"AND spam_users.id IS NULL "+
				"ORDER BY timestamp DESC "+
				"LIMIT 100 "+
				") UNION ("+
				"SELECT newsfeed.id, newsfeed.userid, (CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, pinned, newsfeed.timestamp, "+
				"(CASE WHEN communityevents.id IS NOT NULL AND communityevents.pending THEN 1 ELSE 0 END) AS eventpending,"+
				"(CASE WHEN volunteering.id IS NOT NULL AND volunteering.pending THEN 1 ELSE 0 END) AS volunteeringpending, "+
				"(CASE WHEN users_stories.id IS NOT NULL AND (users_stories.public = 0 OR users_stories.reviewed = 0) THEN 1 ELSE 0 END) AS storypending "+
				"FROM newsfeed FORCE INDEX (position) "+
				"LEFT JOIN users ON users.id = newsfeed.userid "+
				"LEFT JOIN spam_users ON spam_users.userid = newsfeed.userid AND collection IN ('PendingAdd', 'Spammer') "+
				"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
				"LEFT JOIN communityevents ON newsfeed.eventid = communityevents.id "+
				"LEFT JOIN volunteering ON newsfeed.volunteeringid = volunteering.id "+
				"LEFT JOIN users_stories ON newsfeed.storyid = users_stories.id "+
				"WHERE newsfeed.type = ? AND "+
				"replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 "+
				"AND users.deleted IS NULL "+
				"AND spam_users.id IS NULL "+
				"ORDER BY pinned DESC, timestamp DESC "+
				"LIMIT 5) "+
				"ORDER BY pinned DESC, timestamp DESC LIMIT 100;",
			myid,
			swlng, swlat,
			swlng, nelat,
			nelng, nelat,
			nelng, swlat,
			swlng, swlat,
			utils.SRID,
			utils.NEWSFEED_TYPE_CENTRAL_PUBLICITY,
			myid,
			utils.NEWSFEED_TYPE_ALERT,
		).Scan(&newsfeed)
	} else {
		db.Raw("SELECT newsfeed.id, newsfeed.userid, (CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, "+
			"(CASE WHEN communityevents.id IS NOT NULL AND communityevents.pending THEN 1 ELSE 0 END) AS eventpending,"+
			"(CASE WHEN volunteering.id IS NOT NULL AND volunteering.pending THEN 1 ELSE 0 END) AS volunteeringpending, "+
			"(CASE WHEN users_stories.id IS NOT NULL AND (users_stories.public = 0 OR users_stories.reviewed = 0) THEN 1 ELSE 0 END) AS storypending "+
			"FROM newsfeed "+
			"LEFT JOIN users ON users.id = newsfeed.userid "+
			"LEFT JOIN spam_users ON spam_users.userid = newsfeed.userid AND collection IN ('PendingAdd', 'Spammer') "+
			"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
			"LEFT JOIN communityevents ON newsfeed.eventid = communityevents.id "+
			"LEFT JOIN volunteering ON newsfeed.volunteeringid = volunteering.id "+
			"LEFT JOIN users_stories ON newsfeed.storyid = users_stories.id "+
			"WHERE replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 AND "+
			"newsfeed.type NOT IN (?) "+
			"AND users.deleted IS NULL "+
			"AND users.deleted IS NULL "+
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
				ret = append(ret, newsfeed[i])
			}
		} else {
			// Don't return volunteering/events/stories if they are still pending.
			if !newsfeed[i].Eventpending && !newsfeed[i].Volunteeringpending && !newsfeed[i].Storypending {
				ret = append(ret, newsfeed[i])
			}
		}
	}

	return ret
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
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			newsfeed, _ = fetchSingle(id, myid, lovelist)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()

			var me user.User
			db := database.DBConn
			db.First(&me, myid)
			amAMod := me.Systemrole != "User"

			replies = fetchReplies(id, myid, id, amAMod)
		}()

		wg.Wait()

		if newsfeed.ID > 0 {
			newsfeed.Replies = replies

			if newsfeed.Replyto > 0 {
				// We need to find the thread head.
				parentid := newsfeed.Replyto
				for parentid > 0 {
					newsfeed.Threadhead = parentid
					parent, _ := fetchSingle(parentid, myid, lovelist)
					parentid = parent.Replyto
				}
			}

			newsfeed.Previews = getPreviews(newsfeed.Message)

			return c.JSON(newsfeed)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Newsfeed item not found")
}

func getPreviews(text string) []NewsfeedPreview {
	db := database.DBConn

	previews := []NewsfeedPreview{}

	rxRelaxed := xurls.Relaxed()
	urls := rxRelaxed.FindAllString(text, -1)

	if len(urls) > 0 {
		var wg2 sync.WaitGroup
		var mu sync.Mutex

		for _, url := range urls {
			wg2.Add(1)

			go func(url string) {
				defer wg2.Done()

				// Replace http:// with https://
				url = strings.ReplaceAll(url, "http://", "https://")

				// Prepend https:// to the url if not present.
				if !strings.HasPrefix(strings.ToLower(url), "https://") {
					url = "https://" + url
				}

				// Exclude email addresses which contain @
				if !strings.Contains(url, "@") {
					// Get the title of the URL.  Don't use First() as logs error.
					var preview NewsfeedPreview
					preview.ID = 0
					db.Where("url LIKE ?", url).Limit(1).Find(&preview)

					if preview.ID > 0 {
						mu.Lock()
						defer mu.Unlock()
						previews = append(previews, preview)
					}
				}
			}(url)
		}

		wg2.Wait()
	}

	return previews
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
			"(CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, "+
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
					Path:      "https://" + os.Getenv("IMAGE_DOMAIN") + "/fimg_" + strconv.FormatUint(newsfeed.Imageid, 10) + ".jpg",
					PathThumb: "https://" + os.Getenv("IMAGE_DOMAIN") + "/tfimg_" + strconv.FormatUint(newsfeed.Imageid, 10) + ".jpg",
				}
			}
		}

		var wg2 sync.WaitGroup

		wg2.Add(2)

		var info user.UserInfo
		var profileRecord user.UserProfileRecord

		wg2.Add(1)
		go func() {
			defer wg2.Done()
			info = user.GetUserInfo(newsfeed.Userid, myid)
		}()

		go func() {
			defer wg2.Done()
			profileRecord = user.GetProfileRecord(newsfeed.Userid)
		}()

		previews := []NewsfeedPreview{}

		go func() {
			defer wg2.Done()
			previews = getPreviews(newsfeed.Message)
		}()

		wg2.Wait()

		newsfeed.Info = info
		newsfeed.Previews = previews

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
		// We return the hidden flag.  This would allow someone whose posts had been hidden to spot that in the API
		// call, but it saves some extra DB ops to determine that we are a mod. So we hide that from them in the client.
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

func fetchReplies(id uint64, myid uint64, threadhead uint64, amAMod bool) []Newsfeed {
	db := database.DBConn

	var replies = []Newsfeed{}

	type ReplyId struct {
		ID uint64 `json:"id"`
	}

	var replyids []ReplyId
	var mu sync.Mutex

	db.Raw("SELECT id FROM newsfeed WHERE replyto = ? AND deleted IS NULL ORDER BY timestamp ASC", id).Scan(&replyids)

	var wg sync.WaitGroup

	// We have to fetch the replies first otherwise we don't have a slot into which
	// to put the replies to the replies.
	for i := 0; i < len(replyids); i++ {
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
	}

	wg.Wait()

	var wg2 sync.WaitGroup

	// Fetch any replies to the replies (which in turn will fetch replies to those).
	for i := 0; i < len(replyids); i++ {
		wg2.Add(1)
		go func(replyid uint64) {
			defer wg2.Done()

			repliestoreplies := fetchReplies(replyid, myid, threadhead, amAMod)
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

	wg2.Wait()

	// Sort replies by ascending timestamp.
	sort.Slice(replies, func(i, j int) bool {
		return replies[i].Timestamp.Before(replies[j].Timestamp)
	})

	// Remove any hidden replies unless they are ours or we're a mod.
	var newReplies = []Newsfeed{}

	for i := 0; i < len(replies); i++ {
		if replies[i].Hidden == nil || replies[i].Userid == myid || amAMod {
			newReplies = append(newReplies, replies[i])
		}
	}

	return newReplies
}

func Count(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var distance uint64 = 1609
	var err error
	gotDistance := true

	if c.Query("distance") != "" && c.Query("distance") != "nearby" {
		distance, err = strconv.ParseUint(c.Query("distance"), 10, 32)

		if err != nil {
			gotDistance = true
		}
	}

	// Get what we've already seen, and our current feed.
	var wg sync.WaitGroup
	var ret []NewsfeedSummary
	var seen uint64

	wg.Add(1)

	go func() {
		defer wg.Done()
		ret = getFeed(myid, gotDistance, distance)
	}()

	db := database.DBConn
	wg.Add(1)

	go func() {
		defer wg.Done()
		db.Raw("SELECT newsfeedid FROM newsfeed_users WHERE userid = ?", myid).Row().Scan(&seen)
	}()

	wg.Wait()

	// Count the ids in the feed that are greater than seen.
	var count uint64 = 0

	for i := 0; i < len(ret); i++ {
		if ret[i].ID > seen && ret[i].Hidden == nil {
			count++
		}
	}

	return c.JSON(fiber.Map{
		"count": count,
	})
}
