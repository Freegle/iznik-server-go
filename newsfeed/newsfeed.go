package newsfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	geo "github.com/kellydunn/golang-geo"
	"gorm.io/gorm"
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
	ID           uint64          `json:"id"`
	Path         string          `json:"path"`
	PathThumb    string          `json:"paththumb"`
	Externaluid  string          `json:"externaluid"`
	Ouruid       string          `json:"ouruid"`
	Externalmods json.RawMessage `json:"externalmods"`
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
	Hiddenby            uint64     `json:"hiddenby"`
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
	Imageuid       string            `json:"-"`
	Imagemods      json.RawMessage   `json:"-"`
	Image          *NewsImage        `json:"image" gorm:"-"`
	Msgid          uint64            `json:"msgid"`
	Replyto        uint64            `json:"replyto"`
	Groupid        uint64            `json:"groupid"`
	Eventid        uint64            `json:"eventid"`
	Volunteeringid uint64            `json:"volunteeringid"`
	Storyid        uint64            `json:"storyid"`
	Message        string            `json:"message"`
	Html           string            `json:"html"`
	Pinned         bool              `json:"pinned"`
	Hidden         *time.Time        `json:"hidden"`
	Hiddenby       uint64            `json:"hiddenby"`
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
		if c.Query("distance") == "anywhere" {
			distance = 0
			gotDistance = true
		} else {
			distance, err = strconv.ParseUint(c.Query("distance"), 10, 32)

			if err == nil {
				gotDistance = true
			}
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
	//
	// Use a backstop timestamp so we can index better.
	start := time.Now().AddDate(0, 0, -utils.OPEN_AGE_CHITCHAT).Format("2006-01-02")

	if gotLatLng {
		db.Raw(
			"(SELECT newsfeed.id, newsfeed.userid, (CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, hiddenby, pinned, newsfeed.timestamp, "+
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
				"newsfeed.timestamp >= ? AND replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 "+
				"AND users.deleted IS NULL "+
				"AND spam_users.id IS NULL "+
				"ORDER BY timestamp DESC "+
				"LIMIT 100 "+
				") UNION ("+
				"SELECT newsfeed.id, newsfeed.userid, (CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, hiddenby, pinned, newsfeed.timestamp, "+
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
				"WHERE newsfeed.timestamp >= ? AND replyto IS NULL AND newsfeed.type = ? AND "+
				"newsfeed.deleted IS NULL AND reviewrequired = 0 "+
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
			start,
			myid,
			start,
			utils.NEWSFEED_TYPE_ALERT,
		).Scan(&newsfeed)
	} else {
		db.Raw("SELECT newsfeed.id, newsfeed.userid, (CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, hiddenby, "+
			"(CASE WHEN communityevents.id IS NOT NULL AND communityevents.pending THEN 1 ELSE 0 END) AS eventpending,"+
			"(CASE WHEN volunteering.id IS NOT NULL AND volunteering.pending THEN 1 ELSE 0 END) AS volunteeringpending, "+
			"(CASE WHEN users_stories.id IS NOT NULL AND (users_stories.public = 0 OR users_stories.reviewed = 0) THEN 1 ELSE 0 END) AS storypending "+
			"FROM newsfeed FORCE INDEX (timestamp) "+
			"LEFT JOIN users ON users.id = newsfeed.userid "+
			"LEFT JOIN spam_users ON spam_users.userid = newsfeed.userid AND collection IN ('PendingAdd', 'Spammer') "+
			"LEFT JOIN newsfeed_unfollow ON newsfeed.id = newsfeed_unfollow.newsfeedid AND newsfeed_unfollow.userid = ? "+
			"LEFT JOIN communityevents ON newsfeed.eventid = communityevents.id "+
			"LEFT JOIN volunteering ON newsfeed.volunteeringid = volunteering.id "+
			"LEFT JOIN users_stories ON newsfeed.storyid = users_stories.id "+
			"WHERE newsfeed.timestamp >= ? AND replyto IS NULL AND newsfeed.deleted IS NULL AND reviewrequired = 0 "+
			"AND users.deleted IS NULL "+
			"AND spam_users.id IS NULL "+
			"ORDER BY pinned DESC, newsfeed.timestamp DESC LIMIT 100;",
			myid,
			start,
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

			amAMod := false
			if myid > 0 {
				var me user.User
				db := database.DBConn
				db.First(&me, myid)
				amAMod = me.Systemrole != "User"
			}

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

		db.Raw("SELECT newsfeed.*, newsfeed_images.archived AS imagearchived, newsfeed_images.externaluid AS imageuid, newsfeed_images.externalmods AS imagemods, "+
			"(CASE WHEN users.newsfeedmodstatus = 'Suppressed' THEN NOW() ELSE newsfeed.hidden END) AS hidden, "+
			"CASE WHEN users.fullname IS NOT NULL THEN users.fullname ELSE CONCAT(users.firstname, ' ', users.lastname) END AS displayname, "+
			"CASE WHEN systemrole IN ('Moderator', 'Support', 'Admin') THEN CASE WHEN JSON_EXTRACT(users.settings, '$.showmod') IS NULL THEN 1 ELSE JSON_EXTRACT(users.settings, '$.showmod') END ELSE 0 END AS showmod "+
			"FROM newsfeed "+
			"LEFT JOIN users ON users.id = newsfeed.userid "+
			"LEFT JOIN newsfeed_images ON newsfeed.imageid = newsfeed_images.id WHERE newsfeed.id = ?;", id).Scan(&newsfeed)

		if newsfeed.Imageid > 0 {
			if newsfeed.Imageuid != "" {
				newsfeed.Image = &NewsImage{
					ID:           newsfeed.Imageid,
					Ouruid:       newsfeed.Imageuid,
					Externalmods: newsfeed.Imagemods,
					Path:         misc.GetImageDeliveryUrl(newsfeed.Imageuid, string(newsfeed.Imagemods)),
					PathThumb:    misc.GetImageDeliveryUrl(newsfeed.Imageuid, string(newsfeed.Imagemods)),
				}
			} else if newsfeed.Imagearchived {
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
			user.ProfileSetPath(profileRecord.Profileid, profileRecord.Url, profileRecord.Externaluid, profileRecord.Externalmods, profileRecord.Archived, &newsfeed.Profile)
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
		newsfeed.Displayname = utils.TidyName(newsfeed.Displayname)

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
		if c.Query("distance") == "anywhere" {
			distance = 0
			gotDistance = true
		} else {
			distance, err = strconv.ParseUint(c.Query("distance"), 10, 32)

			if err != nil {
				gotDistance = true
			}
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

type PostRequest struct {
	ID      uint64 `json:"id"`
	Action  string `json:"action"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Replyto uint64 `json:"replyto"`
	Imageid uint64 `json:"imageid"`
}

// canModifyPost checks if a user can edit/delete a newsfeed post.
// Allowed: post owner, admin/support, or any group moderator.
func canModifyPost(myid uint64, nfID uint64) bool {
	db := database.DBConn

	var ownerID uint64
	db.Raw("SELECT userid FROM newsfeed WHERE id = ?", nfID).Scan(&ownerID)

	if ownerID == myid {
		return true
	}

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	var modCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'", myid).Scan(&modCount)

	return modCount > 0
}

// canHidePost checks if a user can hide/unhide a newsfeed post.
// Requires: isAdminOrSupport() OR member of "ChitChat Moderation" team.
// This is stricter than canModifyPost - not all moderators can hide posts.
func canHidePost(myid uint64) bool {
	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	// Check if user is a member of the ChitChat Moderation team
	var teamMemberCount int64
	db.Raw("SELECT COUNT(*) FROM teams_members tm INNER JOIN teams t ON tm.teamid = t.id WHERE t.name = 'ChitChat Moderation' AND tm.userid = ?", myid).Scan(&teamMemberCount)

	return teamMemberCount > 0
}

func Post(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	switch req.Action {
	case "Love":
		if req.ID > 0 {
			db.Exec("INSERT IGNORE INTO newsfeed_likes (newsfeedid, userid) VALUES (?, ?)", req.ID, myid)

			// Send notification to the post/comment author.
			type PostOwner struct {
				Userid  uint64  `json:"userid"`
				Replyto *uint64 `json:"replyto"`
			}
			var owner PostOwner
			db.Raw("SELECT userid, replyto FROM newsfeed WHERE id = ?", req.ID).Scan(&owner)
			if owner.Userid > 0 && owner.Userid != myid {
				notifType := "LovedPost"
				if owner.Replyto != nil && *owner.Replyto > 0 {
					notifType = "LovedComment"
				}
				db.Exec("INSERT INTO users_notifications (fromuser, touser, type, newsfeedid) VALUES (?, ?, ?, ?)",
					myid, owner.Userid, notifType, req.ID)
			}
		}
	case "Unlove":
		if req.ID > 0 {
			db.Exec("DELETE FROM newsfeed_likes WHERE newsfeedid = ? AND userid = ?", req.ID, myid)
		}
	case "Seen":
		if req.ID > 0 {
			db.Exec("REPLACE INTO newsfeed_users (userid, newsfeedid) VALUES (?, ?)", myid, req.ID)
			db.Exec("UPDATE users_notifications SET seen = 1 WHERE touser = ? AND newsfeedid = ?", myid, req.ID)
		}
	case "Follow":
		if req.ID > 0 {
			db.Exec("DELETE FROM newsfeed_unfollow WHERE userid = ? AND newsfeedid = ?", myid, req.ID)
		}
	case "Unfollow":
		if req.ID > 0 {
			db.Exec("REPLACE INTO newsfeed_unfollow (userid, newsfeedid) VALUES (?, ?)", myid, req.ID)
			db.Exec("DELETE FROM users_notifications WHERE touser = ? AND (newsfeedid = ? OR newsfeedid IN (SELECT id FROM newsfeed WHERE replyto = ?))", myid, req.ID, req.ID)
		}
	case "Report":
		if req.ID > 0 {
			db.Exec("UPDATE newsfeed SET reviewrequired = 1 WHERE id = ?", req.ID)
			db.Exec("INSERT INTO newsfeed_reports (userid, newsfeedid, reason) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE reason = ?",
				myid, req.ID, req.Reason, req.Reason)

			// Queue email to ChitChat support.
			type ReporterInfo struct {
				Fullname string
				Email    string
			}
			var reporter ReporterInfo
			db.Raw("SELECT u.fullname, ue.email FROM users u LEFT JOIN users_emails ue ON ue.userid = u.id AND ue.preferred = 1 WHERE u.id = ?", myid).Scan(&reporter)

			queue.QueueTask(queue.TaskEmailChitchatReport, map[string]interface{}{
				"user_id":     myid,
				"user_name":   reporter.Fullname,
				"user_email":  reporter.Email,
				"newsfeed_id": req.ID,
				"reason":      req.Reason,
			})
		}
	case "Hide":
		if req.ID > 0 && canHidePost(myid) {
			db.Exec("UPDATE newsfeed SET hidden = NOW(), hiddenby = ? WHERE id = ?", myid, req.ID)
		} else if req.ID > 0 {
			return fiber.NewError(fiber.StatusForbidden, "Permission denied")
		}
	case "Unhide":
		if req.ID > 0 && canHidePost(myid) {
			db.Exec("UPDATE newsfeed SET hidden = NULL, hiddenby = NULL WHERE id = ?", req.ID)
		} else if req.ID > 0 {
			return fiber.NewError(fiber.StatusForbidden, "Permission denied")
		}
	case "ReferToWanted":
		if req.ID > 0 {
			createRefer(db, myid, req.ID, "ReferToWanted")
		}
	case "ReferToOffer":
		if req.ID > 0 {
			createRefer(db, myid, req.ID, "ReferToOffer")
		}
	case "ReferToTaken":
		if req.ID > 0 {
			createRefer(db, myid, req.ID, "ReferToTaken")
		}
	case "ReferToReceived":
		if req.ID > 0 {
			createRefer(db, myid, req.ID, "ReferToReceived")
		}
	case "AttachToThread":
		// Mod-only: attach a newsfeed item to a different thread
		if req.ID > 0 && req.Replyto > 0 {
			var modCount int64
			db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'", myid).Scan(&modCount)
			if modCount > 0 {
				db.Exec("UPDATE newsfeed SET replyto = ? WHERE id = ?", req.Replyto, req.ID)
			} else {
				return fiber.NewError(fiber.StatusForbidden, "Permission denied")
			}
		}
	case "":
		// No action = create new post or reply.
		return createPost(c, db, myid, req)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action")
	}

	return c.JSON(fiber.Map{"success": true})
}

// createPost creates a new newsfeed post or reply.
func createPost(c *fiber.Ctx, db *gorm.DB, myid uint64, req PostRequest) error {
	// Check if user is a spammer
	var spammerCount int64
	db.Raw("SELECT COUNT(*) FROM spam_users WHERE userid = ? AND collection IN ('PendingAdd', 'Spammer')", myid).Scan(&spammerCount)
	if spammerCount > 0 {
		// Silently succeed - don't reveal spammer status.
		return c.JSON(fiber.Map{"id": 0})
	}

	// Check suppression status
	var newsfeedmodstatus string
	db.Raw("SELECT COALESCE(newsfeedmodstatus, '') FROM users WHERE id = ?", myid).Scan(&newsfeedmodstatus)
	hidden := newsfeedmodstatus == "Suppressed"

	// Get user's lat/lng for geographic positioning
	latlng := user.GetLatLng(myid)
	lat := float64(latlng.Lat)
	lng := float64(latlng.Lng)

	if lat == 0 && lng == 0 {
		// No location - can't create non-alert posts without location
		return c.JSON(fiber.Map{"id": 0})
	}

	// Duplicate prevention: check last post by user
	type LastPost struct {
		ID      uint64  `json:"id"`
		Replyto *uint64 `json:"replyto"`
		Type    string  `json:"type"`
		Message string  `json:"message"`
	}
	var last LastPost
	db.Raw("SELECT id, replyto, type, message FROM newsfeed WHERE userid = ? ORDER BY id DESC LIMIT 1", myid).Scan(&last)

	var lastReplyto uint64
	if last.Replyto != nil {
		lastReplyto = *last.Replyto
	}
	if last.ID > 0 && lastReplyto == req.Replyto && last.Type == "Message" && last.Message == req.Message {
		// Duplicate - return existing ID
		return c.JSON(fiber.Map{"id": last.ID})
	}

	// Get user's display location
	var location *string
	db.Raw("SELECT locations.name FROM users LEFT JOIN locations ON users.lastlocation = locations.id WHERE users.id = ?", myid).Scan(&location)

	// Build position point
	pos := fmt.Sprintf("ST_GeomFromText('POINT(%f %f)', %d)", lng, lat, utils.SRID)

	// Insert the newsfeed entry
	hiddenSQL := "NULL"
	if hidden {
		hiddenSQL = "NOW()"
	}

	var imageid interface{}
	if req.Imageid > 0 {
		imageid = req.Imageid
	}
	var replyto interface{}
	if req.Replyto > 0 {
		replyto = req.Replyto
	}

	result := db.Exec(
		fmt.Sprintf("INSERT INTO newsfeed (type, userid, imageid, replyto, message, position, hidden, location) VALUES ('Message', ?, ?, ?, ?, %s, %s, ?)", pos, hiddenSQL),
		myid, imageid, replyto, req.Message, location)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create newsfeed post")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	// If this is a reply and not hidden, bump the thread
	if id > 0 && req.Replyto > 0 && !hidden {
		bumpThread(db, req.Replyto)
		notifyThreadContributors(db, myid, id, req.Replyto)

		// Mark own notifications for this thread as seen
		db.Exec("UPDATE users_notifications SET seen = 1 WHERE touser = ? AND (newsfeedid = ? OR newsfeedid IN (SELECT id FROM newsfeed WHERE replyto = ?))",
			myid, req.Replyto, req.Replyto)
	}

	return c.JSON(fiber.Map{"id": id})
}

// bumpThread updates timestamps up the reply chain to bring the thread to the top of the feed.
func bumpThread(db *gorm.DB, replyto uint64) {
	bump := replyto
	for bump > 0 {
		db.Exec("UPDATE newsfeed SET timestamp = NOW() WHERE id = ?", bump)
		var parent *uint64
		db.Raw("SELECT replyto FROM newsfeed WHERE id = ?", bump).Scan(&parent)
		if parent != nil && *parent > 0 {
			bump = *parent
		} else {
			bump = 0
		}
	}
}

// notifyThreadContributors notifies users who have recently contributed to a thread.
// Only notifies users who commented in the last 7 days.
func notifyThreadContributors(db *gorm.DB, posterUserid uint64, newPostID uint64, replyto uint64) {
	recent := time.Now().AddDate(0, 0, -7)

	// Collect all post IDs in the thread and contributors
	type PostInfo struct {
		ID      uint64    `json:"id"`
		Userid  uint64    `json:"userid"`
		Addedts time.Time `json:"timestamp"`
	}

	contributed := make(map[uint64]bool)
	ids := []uint64{replyto}
	processed := make(map[uint64]bool)

	for {
		oldLen := len(ids)
		var newIDs []uint64

		for _, pid := range ids {
			if processed[pid] {
				continue
			}
			processed[pid] = true

			var posts []PostInfo
			db.Raw("SELECT id, userid, timestamp FROM newsfeed WHERE replyto = ? OR id = ?", pid, pid).Scan(&posts)

			for _, p := range posts {
				if p.Addedts.After(recent) && p.Userid != posterUserid {
					contributed[p.Userid] = true
				}
				newIDs = append(newIDs, p.ID)
			}
		}

		ids = append(ids, newIDs...)
		// Deduplicate
		seen := make(map[uint64]bool)
		unique := make([]uint64, 0)
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				unique = append(unique, id)
			}
		}
		ids = unique

		if len(ids) == oldLen {
			break
		}
	}

	// Notify contributors
	for uid := range contributed {
		db.Exec("INSERT INTO users_notifications (fromuser, touser, type, newsfeedid) VALUES (?, ?, 'CommentOnYourPost', ?)",
			posterUserid, uid, replyto)
	}
}

// createRefer creates a refer-type reply to a newsfeed post and notifies the original poster.
func createRefer(db *gorm.DB, myid uint64, nfID uint64, referType string) {
	// Get user's location
	latlng := user.GetLatLng(myid)
	lat := float64(latlng.Lat)
	lng := float64(latlng.Lng)

	pos := fmt.Sprintf("ST_GeomFromText('POINT(%f %f)', %d)", lng, lat, utils.SRID)

	db.Exec(fmt.Sprintf("INSERT INTO newsfeed (type, userid, replyto, position) VALUES (?, ?, ?, %s)", pos),
		referType, myid, nfID)

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	// Notify the original poster
	if newID > 0 {
		var originalUserid uint64
		db.Raw("SELECT userid FROM newsfeed WHERE id = ?", nfID).Scan(&originalUserid)
		if originalUserid > 0 && originalUserid != myid {
			db.Exec("INSERT INTO users_notifications (fromuser, touser, type, newsfeedid) VALUES (?, ?, 'CommentOnYourPost', ?)",
				myid, originalUserid, nfID)
		}
	}
}

type PatchRequest struct {
	ID      uint64 `json:"id"`
	Message string `json:"message"`
}

func Edit(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn
	var ownerID uint64
	db.Raw("SELECT userid FROM newsfeed WHERE id = ?", req.ID).Scan(&ownerID)
	if ownerID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Newsfeed post not found")
	}

	if ownerID != myid && !canModifyPost(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized to edit this post")
	}

	db.Exec("UPDATE newsfeed SET message = ? WHERE id = ?", req.Message, req.ID)

	return c.JSON(fiber.Map{"success": true})
}

func Delete(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
	}

	db := database.DBConn
	var ownerID uint64
	db.Raw("SELECT userid FROM newsfeed WHERE id = ?", id).Scan(&ownerID)
	if ownerID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Newsfeed post not found")
	}

	if ownerID != myid && !canModifyPost(myid, id) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized to delete this post")
	}

	// Soft delete
	db.Exec("UPDATE newsfeed SET deleted = NOW(), deletedby = ? WHERE id = ?", myid, id)
	db.Exec("DELETE FROM users_notifications WHERE newsfeedid = ?", id)

	return c.JSON(fiber.Map{"success": true})
}
