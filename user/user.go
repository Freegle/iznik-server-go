package user

import (
	"encoding/json"
	"errors"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/location"
	log2 "github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"strconv"
	"sync"
	"time"
)

type Aboutme struct {
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type User struct {
	ID              uint64      `json:"id" gorm:"primary_key"`
	Firstname       *string     `json:"firstname"`
	Lastname        *string     `json:"lastname"`
	Fullname        *string     `json:"fullname"`
	Displayname     string      `json:"displayname" gorm:"-"`
	Profile         UserProfile `json:"profile" gorm:"-"`
	Lastaccess      time.Time   `json:"lastaccess"`
	Info            UserInfo    `json:"info" gorm:"-"`
	Supporter       bool        `json:"supporter" gorm:"-"`
	Donated         *time.Time  `json:"donated" gorm:"-"`
	Spammer         bool        `json:"spammer" gorm:"-"`
	Showmod         bool        `json:"showmod" gorm:"-"`
	Lat             float32     `json:"lat" gorm:"-"` // Exact for logged in user, approx for others.
	Lng             float32     `json:"lng" gorm:"-"`
	Aboutme         Aboutme     `json:"aboutme" gorm:"-"`
	Added           time.Time   `json:"added"`
	ExpectedReplies int         `json:"expectedreplies" gorm:"-"`
	ExpectedChats   []uint64    `json:"expectedchats" gorm:"-"`
	Ljuserid        *uint64     `json:"ljuserid"`
	Deleted         *time.Time  `json:"deleted"`
	Forgotten       *time.Time  `json:"forgotten"`
	Lastlocation    *uint64     `json:"lastlocation"`

	// Only returned for logged-in user.
	Email              string          `json:"email" gorm:"-"`
	Emails             []UserEmail     `json:"emails" gorm:"-"`
	Memberships        []Membership    `json:"memberships" gorm:"-"`
	Systemrole         string          `json:"systemrole""`
	Settings           json.RawMessage `json:"settings"` // This is JSON stored in the DB as a string.
	Relevantallowed    bool            `json:"relevantallowed"`
	Newslettersallowed bool            `json:"newslettersallowed"`
	Bouncing           bool            `json:"bouncing"`
	Trustlevel         *string         `json:"trustlevel"`
	Marketingconsent   bool            `json:"marketingconsent"`
	Source             *string         `json:"source"`
}

type Tabler interface {
	TableName() string
}

func (UserProfileRecord) TableName() string {
	return "users_images"
}

type UserProfileRecord struct {
	ID           uint64 `json:"id" gorm:"primary_key"`
	Profileid    uint64
	Url          string
	Archived     int
	Useprofile   bool            `json:"-"`
	Externaluid  string          `json:"externaluid"`
	Ouruid       string          `json:"ouruid"`
	Externalmods json.RawMessage `json:"externalmods"`
}

// This corresponds to the DB table.
func (MembershipTable) TableName() string {
	return "memberships"
}

type MembershipTable struct {
	ID                  uint64    `json:"id" gorm:"primary_key"`
	Groupid             uint64    `json:"groupid"`
	Userid              uint64    `json:"userid"`
	Added               time.Time `json:"added"`
	Collection          string    `json:"collection"`
	Emailfrequency      int       `json:"emailfrequency"`
	Eventsallowed       int       `json:"eventsallowed"`
	Volunteeringallowed int       `json:"volunteeringallowed"`
	Role                string    `json:"role"`
}

// This is the membership we return to the client.  It includes some information not stored in the DB.
type Membership struct {
	MembershipTable
	Nameshort                string `json:"nameshort"`
	Namefull                 string `json:"namefull"`
	Namedisplay              string `json:"namedisplay"`
	Bbox                     string `json:"bbox"`
	Microvolunteeringallowed int    `json:"microvolunteeringallowed"`
}

func (MembershipHistory) TableName() string {
	return "memberships_history"
}

type MembershipHistory struct {
	ID                 uint64    `json:"id" gorm:"primary_key"`
	Groupid            uint64    `json:"groupid"`
	Userid             uint64    `json:"userid"`
	Added              time.Time `json:"added"`
	Collection         string    `json:"collection"`
	Processingrequired bool      `json:"processingrequired"`
}

type Search struct {
	ID         uint64    `json:"id" gorm:"primary_key"`
	Date       time.Time `json:"date"`
	Userid     uint64    `json:"userid"`
	Term       string    `json:"term"`
	Maxmsg     uint64    `json:"maxmsg"`
	Locationid uint64    `json:"locationid"`
}

func GetUser(c *fiber.Ctx) error {
	if c.Params("id") != "" {
		// Looking for a specific user.
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			myid := WhoAmI(c)

			user := GetUserById(id, myid)

			// Hide
			user.Systemrole = ""
			user.Settings = nil
			user.Relevantallowed = false
			user.Newslettersallowed = false
			user.Bouncing = false
			user.Marketingconsent = false
			user.Source = nil

			if user.ID == id {
				return c.JSON(user)
			}
		}

		return fiber.NewError(fiber.StatusNotFound, "User not found")
	} else {
		// Looking for the currently logged-in user as authenticated by the Authorization header JWT (if present).
		id := WhoAmI(c)

		if id > 0 {
			// We want to get information in parallel.
			var wg sync.WaitGroup
			var memberships []Membership
			var user User
			var latlng utils.LatLng
			var emails []UserEmail

			db := database.DBConn

			wg.Add(1)
			go func() {
				defer wg.Done()
				user = GetUserById(id, id)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				memberships = GetMemberships(id)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				latlng = GetLatLng(id)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				emails = getEmails(id)
			}()

			// Now wait for these parallel requests to complete.
			wg.Wait()
			user.Memberships = memberships
			user.Lat = latlng.Lat
			user.Lng = latlng.Lng
			user.Emails = emails

			if len(emails) > 0 {
				// First email is preferred (by construction) or best guess.
				//
				// Find first email that is not ourDomain
				for _, email := range emails {
					if user.Email == "" && utils.OurDomain(email.Email) == 0 {
						user.Email = email.Email
					}
				}
			}

			if user.Settings == nil {
				user.Settings = json.RawMessage("{}")
			}

			if user.ID == id {
				return c.JSON(user)
			}
		}

		return fiber.NewError(fiber.StatusNotFound, "Not logged in")
	}
}

func GetExpectedReplies(id uint64) []uint64 {
	var expectedReplies []uint64

	db := database.DBConn

	start := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")
	db.Raw("SELECT DISTINCT(chatid) FROM users_expected "+
		"INNER JOIN users ON users.id = users_expected.expectee "+
		"INNER JOIN chat_messages ON chat_messages.id = users_expected.chatmsgid "+
		"WHERE expectee = ? AND "+
		"chat_messages.date >= ? AND replyexpected = 1 AND replyreceived = 0 AND TIMESTAMPDIFF(MINUTE, chat_messages.date, users.lastaccess) >= ?",
		id,
		start,
		utils.CHAT_REPLY_GRACE).Pluck("count", &expectedReplies)

	return expectedReplies
}

func GetMemberships(id uint64) []Membership {
	db := database.DBConn

	var memberships []Membership
	db.Raw("SELECT memberships.id, added, role, groupid, emailfrequency, eventsallowed, volunteeringallowed, microvolunteering AS microvolunteeringallowed, nameshort, namefull, ST_AsText(ST_ENVELOPE(polyindex)) AS bbox FROM memberships INNER JOIN `groups` ON groups.id = memberships.groupid WHERE userid = ? AND collection = ?", id, "Approved").Scan(&memberships)

	for ix, r := range memberships {
		if len(r.Namefull) > 0 {
			memberships[ix].Namedisplay = r.Namefull
		} else {
			memberships[ix].Namedisplay = r.Nameshort
		}
	}

	return memberships
}

func GetUserById(id uint64, myid uint64) User {
	db := database.DBConn

	var user User
	var info UserInfo
	var aboutme Aboutme
	var profileRecord UserProfileRecord
	var expectedReplies []uint64

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		// This provides enough information about a message to display a summary on the browse page.
		var settingsq = ""

		if id == myid {
			settingsq = "settings, "
		}

		err := db.Raw("SELECT users.id, firstname, lastname, fullname, lastaccess, users.added, systemrole, relevantallowed, newslettersallowed, marketingconsent, trustlevel, bouncing, deleted, forgotten, source, "+settingsq+
			"(CASE WHEN spam_users.id IS NOT NULL AND spam_users.collection = 'Spammer' THEN 1 ELSE 0 END) AS spammer, "+
			"CASE WHEN systemrole IN ('Moderator', 'Support', 'Admin') AND JSON_EXTRACT(users.settings, '$.showmod') IS NULL THEN 1 ELSE JSON_EXTRACT(users.settings, '$.showmod') END AS showmod "+
			"FROM users LEFT JOIN spam_users ON spam_users.userid = users.id "+
			"WHERE users.id = ? ", id).First(&user).Error

		if !errors.Is(err, gorm.ErrRecordNotFound) {
			if user.Deleted == nil {
				if user.Fullname != nil {
					user.Displayname = *user.Fullname
				} else {
					user.Displayname = ""

					if user.Firstname != nil {
						user.Displayname += *user.Firstname

						if user.Lastname != nil {
							user.Displayname += " " + *user.Lastname
						}
					} else if user.Lastname != nil {
						user.Displayname = *user.Lastname
					}
				}

				user.Displayname = utils.TidyName(user.Displayname)
			} else {
				// Censor name for deleted user.
				user.Displayname = "Deleted User #" + strconv.FormatUint(id, 10)
				user.Firstname = nil
				user.Lastname = nil
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		profileRecord = GetProfileRecord(id)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		info = GetUserInfo(id, myid)
	}()

	// We return the approximate location of the user.
	var lat, lng float64

	wg.Add(1)
	go func() {
		defer wg.Done()
		latlng := GetLatLng(id)

		if (latlng.Lat != 0) || (latlng.Lng != 0) {
			lat, lng = utils.Blur((float64)(latlng.Lat), (float64)(latlng.Lng), utils.BLUR_USER)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT * FROM users_aboutme WHERE userid = ? ORDER BY timestamp DESC LIMIT 1", id).Scan(&aboutme)
	}()

	var supporter struct {
		Supporter bool       `json:"supporter"`
		Donated   *time.Time `json:"donated"`
	}

	wg.Add(1)
	go func() {
		// Get whether they are a supporter - a mod, someone who has donated, or someone who has volunteered.
		// Also get whether they have ever donated - that's used for our own user.
		defer wg.Done()
		start := time.Now().AddDate(0, 0, -utils.SUPPORTER_PERIOD).Format("2006-01-02")

		db.Raw("SELECT (CASE WHEN "+
			"((users.systemrole != 'User' OR "+
			"EXISTS(SELECT id FROM users_donations WHERE userid = ? AND users_donations.timestamp >= ?) OR "+
			"EXISTS(SELECT id FROM microactions WHERE userid = ? AND microactions.timestamp >= ?)) AND "+
			"(CASE WHEN JSON_EXTRACT(users.settings, '$.hidesupporter') IS NULL THEN 0 ELSE JSON_EXTRACT(users.settings, '$.hidesupporter') END) = 0) "+
			"THEN 1 ELSE 0 END) "+
			"AS supporter, "+
			"(SELECT MAX(timestamp) FROM users_donations WHERE userid = ?) AS donated "+
			"FROM users "+
			"WHERE users.id = ?", id, start, id, start, id, id).Scan(&supporter)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		expectedReplies = GetExpectedReplies(id)
	}()

	wg.Wait()

	if user.Deleted == nil && profileRecord.Useprofile {
		ProfileSetPath(profileRecord.Profileid, profileRecord.Url, profileRecord.Externaluid, profileRecord.Externalmods, profileRecord.Archived, &user.Profile)
	}

	user.Lat = (float32)(lat)
	user.Lng = (float32)(lng)

	user.Info = info

	if user.Deleted == nil {
		user.Aboutme = aboutme
	}

	user.Supporter = supporter.Supporter

	if id == myid {
		// We can see our own donor status.
		user.Donated = supporter.Donated
	}

	if user.Deleted == nil {
		user.ExpectedReplies = len(expectedReplies)
		user.ExpectedChats = expectedReplies
	}

	return user
}

func GetProfileRecord(id uint64) UserProfileRecord {
	db := database.DBConn
	var profile UserProfileRecord

	db.Raw("SELECT ui.id AS profileid, ui.url AS url, ui.archived, ui.externaluid, ui.externalmods, "+
		"CASE WHEN JSON_EXTRACT(settings, '$.useprofile') IS NULL THEN 1 ELSE JSON_EXTRACT(settings, '$.useprofile') END AS useprofile "+
		"FROM users_images ui INNER JOIN users ON users.id = ui.userid "+
		"WHERE userid = ? ORDER BY ui.id DESC LIMIT 1", id).Scan(&profile)

	return profile
}

func GetLatLng(id uint64) utils.LatLng {
	var ret utils.LatLng

	ret.Lat = 0
	ret.Lng = 0

	db := database.DBConn

	type userLoc struct {
		ID      uint64 `gorm:"primary_key"`
		Mylat   float32
		Mylng   float32
		Lastlat float32
		Lastlng float32
	}

	var ul, ulmsg, ulgroups userLoc

	// We look for the location in the following descending order:
	// - mylocation in settings, which we need to decode
	// - lastlocation in user
	// - last messages posted on a group with a location
	// - most recently joined group
	//
	// Tests show that the first query is fast to fetch, whereas the others are less so.  The first will handle
	// a user with a known location, so it's a good mainline case to keep fast.
	// If it doesn't give us what we need them , then fetch the others in parallel.
	db.Raw("SELECT users.id, locations.lat AS lastlat, locations.lng as lastlng, "+
		"CAST(JSON_EXTRACT(JSON_EXTRACT(settings, '$.mylocation'), '$.lat') AS DECIMAL(10,6)) AS mylat,"+
		"CAST(JSON_EXTRACT(JSON_EXTRACT(settings, '$.mylocation'), '$.lng') AS DECIMAL(10,6)) as mylng "+
		"FROM users "+
		"LEFT JOIN locations ON locations.id = users.lastlocation "+
		"LEFT JOIN spam_users ON spam_users.userid = users.id "+
		"WHERE users.id = ?", id).Scan(&ul)

	if ul.Mylng != 0 || ul.Mylat != 0 {
		ret.Lat = ul.Mylat
		ret.Lng = ul.Mylng
	} else {
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Raw("SELECT messages.fromuser AS id, locations.lat AS lastlat, locations.lng AS lastlng FROM "+
				"locations INNER JOIN messages ON messages.locationid = locations.id "+
				"WHERE messages.fromuser = ? "+
				"ORDER BY arrival DESC LIMIT 1", id).Scan(&ulmsg)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Raw("SELECT groups.id, groups.lat AS lastlat, groups.lng AS lastlng FROM  "+
				"`groups` INNER JOIN memberships ON groups.id = memberships.groupid "+
				"WHERE memberships.userid = ? "+
				"ORDER BY added DESC LIMIT 1", id).Scan(&ulgroups)
		}()

		wg.Wait()

		if ul.Lastlat != 0 || ul.Lastlng != 0 {
			ret.Lat = ul.Lastlat
			ret.Lng = ul.Lastlng
		} else if ulmsg.Lastlat != 0 || ulmsg.Lastlng != 0 {
			ret.Lat = ulmsg.Lastlat
			ret.Lng = ulmsg.Lastlng
		} else if ulgroups.Lastlat != 0 || ulgroups.Lastlng != 0 {
			ret.Lat = ulgroups.Lastlat
			ret.Lng = ulgroups.Lastlng
		}
	}

	return ret
}

func GetSearchesForUser(c *fiber.Ctx) error {
	db := database.DBConn
	myid := WhoAmI(c)

	if c.Params("id") != "" {
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil && id == myid {
			var searches []Search

			// Show the last few.  Slightly hacky search to make sure we show the most recent searches.
			db.Raw("SELECT * FROM"+
				"(SELECT * FROM users_searches WHERE userid = ? AND deleted = 0 ORDER BY id desc LIMIT 100) t "+
				"GROUP BY t.term ORDER BY t.id DESC LIMIT 10;", id).Find(&searches)

			return c.JSON(searches)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "User not found")
}

func GetPublicLocation(c *fiber.Ctx) error {
	var ret Publiclocation
	var groupname string
	var groupid uint64
	var loc string

	if c.Params("id") != "" {
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			var wg sync.WaitGroup

			latlng := GetLatLng(id)

			wg.Add(1)
			go func() {
				defer wg.Done()
				// Get a public area based on this.
				l := location.ClosestPostcode(latlng.Lat, latlng.Lng)
				loc = l.Areaname
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()

				// Get the closest group.
				group := location.ClosestSingleGroup(float64(latlng.Lat), float64(latlng.Lng), utils.NEARBY)

				if group != nil {
					groupname = group.Namedisplay
					groupid = group.ID
				}
			}()

			wg.Wait()
		}
	}

	if len(loc) > 0 {
		ret.Location = loc
		ret.Groupname = groupname
		ret.Groupid = groupid

		ret.Display = ret.Location

		if len(ret.Groupname) > 0 {
			ret.Display = ret.Location + ", " + ret.Groupname
		}
	}

	return c.JSON(ret)
}

func AddMembership(userid uint64, groupid uint64, role string, collection string, emailfrequency int, eventsallowed int, volunteeringallowed int, reason string) bool {
	db := database.DBConn

	ret := false

	// See if we're already a member, and whether we're banned.
	var wg = sync.WaitGroup{}
	var membership MembershipTable
	var banned uint64

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Where("userid = ? AND groupid = ?", userid, groupid).Limit(1).Find(&membership)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT userid FROM users_banned WHERE userid = ? AND groupid = ?", userid, groupid).Limit(1).Find(&banned)
	}()

	wg.Wait()

	if banned == 0 {
		ret = true

		if membership.ID == 0 {
			ret = false

			membership.Userid = userid
			membership.Groupid = groupid
			membership.Added = time.Now()
			membership.Role = role
			membership.Collection = collection
			membership.Emailfrequency = emailfrequency
			membership.Eventsallowed = eventsallowed
			membership.Volunteeringallowed = volunteeringallowed

			db.Create(&membership)

			if membership.ID > 0 {
				ret = true

				var wg2 = sync.WaitGroup{}

				wg2.Add(1)
				go func() {
					defer wg2.Done()

					// Add to membership history for abuse detection.
					var history MembershipHistory

					history.Userid = userid
					history.Groupid = groupid
					history.Added = membership.Added
					history.Collection = collection

					// Set processingrequired; the PHP code will spot that.
					history.Processingrequired = true

					db.Create(&history)
				}()

				wg2.Add(1)
				go func() {
					// Log the membership.
					defer wg2.Done()
					log2.Log(log2.LogEntry{
						Type:    log2.LOG_TYPE_GROUP,
						Subtype: log2.LOG_SUBTYPE_JOINED,
						User:    &userid,
						Byuser:  &userid,
						Groupid: &groupid,
						Text:    &reason,
					})
				}()

				wg2.Wait()

				// At the moment we only add members from the FD client, so we don't need to change the system role.

				// TODO Background:
				// - Welcome email
				// - Check user for spam
				// - Check for comments which trigger member review.
			}
		}
	}

	return ret
}
