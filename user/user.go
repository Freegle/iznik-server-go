package user

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"strconv"
	"sync"
	"time"
)

type Aboutme struct {
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type User struct {
	ID          uint64      `json:"id" gorm:"primary_key"`
	Firstname   string      `json:"firstname"`
	Lastname    string      `json:"lastname"`
	Fullname    string      `json:"fullname"`
	Displayname string      `json:"displayname"`
	Profile     UserProfile `json:"profile"`
	Lastaccess  time.Time   `json:"lastaccess"`
	Info        UserInfo    `json:"info"`
	Supporter   bool        `json:"supporter"`
	Lat         float32     `json:"lat"` // Exact for logged in user, approx for others.
	Lng         float32     `json:"lng"`
	Aboutme     Aboutme     `json:"aboutme"`

	// Only returned for logged-in user.
	Email       string          `json:"email"`
	Emails      []UserEmail     `json:"emails"`
	Memberships []Membership    `json:"memberships"`
	Systemrole  string          `json:"systemrole"`
	Settings    json.RawMessage `json:"settings"` // This is JSON stored in the DB as a string.
}

type Tabler interface {
	TableName() string
}

func (UserProfileRecord) TableName() string {
	return "users_images"
}

type UserProfileRecord struct {
	ID         uint64 `json:"id" gorm:"primary_key"`
	Profileid  uint64
	Url        string
	Archived   int
	Useprofile bool `json:"-"`
}

type Membership struct {
	ID                  uint64 `json:"id" gorm:"primary_key"`
	Groupid             uint64 `json:"groupid"`
	Emailfrequency      int    `json:"emailfrequency"`
	Eventsallowed       int    `json:"eventsallowed"`
	Volunteeringallowed int    `json:"volunteeringallowed"`
	Role                string `json:"role"`
	Nameshort           string `json:"nameshort"`
	Namefull            string `json:"namefull"`
	Namedisplay         string `json:"namedisplay"`
	Bbox                string `json:"bbox"`
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

			if user.ID == id {
				return c.JSON(user)
			}
		}
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

			wg.Add(1)
			go func() {
				defer wg.Done()
				user = GetUserById(id, id)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				db := database.DBConn
				db.Raw("SELECT memberships.id, role, groupid, emailfrequency, eventsallowed, volunteeringallowed, nameshort, namefull, ST_AsText(ST_ENVELOPE(polyindex)) AS bbox FROM memberships INNER JOIN `groups` ON groups.id = memberships.groupid WHERE userid = ? AND collection = ?", id, "Approved").Scan(&memberships)

				for ix, r := range memberships {
					if len(r.Namefull) > 0 {
						memberships[ix].Namedisplay = r.Namefull
					} else {
						memberships[ix].Namedisplay = r.Nameshort
					}
				}
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
				user.Email = emails[0].Email
			}

			if user.ID == id {

				return c.JSON(user)
			}
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "User not found")
}

func GetUserById(id uint64, myid uint64) User {
	db := database.DBConn

	var user, user2, user3 User
	var aboutme Aboutme

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		// This provides enough information about a message to display a summary on the browse page.
		var settingsq = ""

		if id == myid {
			settingsq = "settings, "
		}

		if !db.Raw("SELECT users.id, firstname, lastname, fullname, lastaccess, "+settingsq+
			"(CASE WHEN spam_users.id IS NOT NULL AND spam_users.collection = 'Spammer' THEN 1 ELSE 0 END) AS spammer "+
			"FROM users LEFT JOIN spam_users ON spam_users.userid = users.id "+
			"WHERE users.id = ?", id).Scan(&user).RecordNotFound() {
			if len(user.Fullname) > 0 {
				user.Displayname = user.Fullname
			} else {
				user.Displayname = user.Firstname + " " + user.Lastname
			}

			user.Displayname = utils.TidyName(user.Displayname)
		}

		var profileRecord UserProfileRecord

		db.Raw("SELECT ui.id AS profileid, ui.url AS url, ui.archived, "+
			"CASE WHEN JSON_EXTRACT(settings, '$.useprofile') IS NULL THEN 1 ELSE JSON_EXTRACT(settings, '$.useprofile') END AS useprofile "+
			"FROM users_images ui INNER JOIN users ON users.id = ui.userid "+
			"WHERE userid = ? ORDER BY ui.id DESC LIMIT 1", id).Scan(&profileRecord)

		if profileRecord.Useprofile {
			ProfileSetPath(profileRecord.Profileid, profileRecord.Url, profileRecord.Archived, &user.Profile)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		user2.Info = GetUserInfo(id, myid)
	}()

	// We return the approximate location of the user.
	var lat, lng float64

	wg.Add(1)
	go func() {
		defer wg.Done()
		latlng := GetLatLng(id)
		lat, lng = utils.Blur((float64)(latlng.Lat), (float64)(latlng.Lng), utils.BLUR_USER)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT * FROM users_aboutme WHERE userid = ? ORDER BY timestamp DESC LIMIT 1", id).Scan(&aboutme)
	}()

	wg.Add(1)
	go func() {
		// Get whether they are a supporter - a mod, someone who has donated, or someone who has volunteered.
		defer wg.Done()
		start := time.Now().AddDate(0, 0, -utils.SUPPORTER_PERIOD).Format("2006-01-02")

		db.Raw("SELECT (CASE WHEN "+
			"((users.systemrole != 'User' OR "+
			"EXISTS(SELECT id FROM users_donations WHERE userid = ? AND users_donations.timestamp >= ?) OR "+
			"EXISTS(SELECT id FROM microactions WHERE userid = ? AND microactions.timestamp >= ?)) AND "+
			"(CASE WHEN JSON_EXTRACT(users.settings, '$.hidesupporter') IS NULL THEN 0 ELSE JSON_EXTRACT(users.settings, '$.hidesupporter') END) = 0) "+
			"THEN 1 ELSE 0 END) "+
			"AS supporter "+
			"FROM users "+
			"WHERE users.id = ?", id, start, id, start, id).Scan(&user3)
	}()

	wg.Wait()

	user.Lat = (float32)(lat)
	user.Lng = (float32)(lng)

	user.Info = user2.Info
	user.Aboutme = aboutme
	user.Supporter = user3.Supporter

	return user
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
	// Fetch all these in parallel for speed.
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw("SELECT users.id, locations.lat AS lastlat, locations.lng as lastlng, "+
			"JSON_EXTRACT(JSON_EXTRACT(settings, '$.mylocation'), '$.lat') AS mylat,"+
			"JSON_EXTRACT(JSON_EXTRACT(settings, '$.mylocation'), '$.lng') as mylng "+
			"FROM users "+
			"LEFT JOIN locations ON locations.id = users.lastlocation "+
			"LEFT JOIN spam_users ON spam_users.userid = users.id "+
			"WHERE users.id = ?", id).Scan(&ul)
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

	if ul.Mylng != 0 || ul.Mylat != 0 {
		ret.Lat = ul.Mylat
		ret.Lng = ul.Mylng
	} else if ul.Lastlat != 0 || ul.Lastlng != 0 {
		ret.Lat = ul.Lastlat
		ret.Lng = ul.Lastlng
	} else if ulmsg.Lastlat != 0 || ulmsg.Lastlng != 0 {
		ret.Lat = ulmsg.Lastlat
		ret.Lng = ulmsg.Lastlng
	} else if ulgroups.Lastlat != 0 || ulgroups.Lastlng != 0 {
		ret.Lat = ulgroups.Lastlat
		ret.Lng = ulgroups.Lastlng
	}

	return ret
}
