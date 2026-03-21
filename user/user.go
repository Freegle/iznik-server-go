package user

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"crypto/rand"
	"math/big"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/location"
	log2 "github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"gorm.io/gorm"
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
	DonatedType     *string     `json:"donatedtype" gorm:"-"`
	Comments        []Comment   `json:"comments,omitempty" gorm:"-"`
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
	Lastlocation    *uint64          `json:"lastlocation"`
	Privateposition *PrivatePosition `json:"privateposition,omitempty" gorm:"-"`

	// Only returned for logged-in user.
	Email              string          `json:"email" gorm:"-"`
	Emails             []UserEmail     `json:"emails" gorm:"-"`
	Memberships        []Membership          `json:"memberships" gorm:"-"`
	MessageHistory     []UserMessageHistory  `json:"messagehistory,omitempty" gorm:"-"`
	Systemrole         string          `json:"systemrole""`
	Settings           json.RawMessage `json:"settings"` // This is JSON stored in the DB as a string.
	Relevantallowed    bool            `json:"relevantallowed"`
	Newslettersallowed bool            `json:"newslettersallowed"`
	Bouncing           bool            `json:"bouncing"`
	Trustlevel         *string         `json:"trustlevel"`
	Marketingconsent   bool            `json:"marketingconsent"`
	Source             *string         `json:"source"`
	Modmails           uint64          `json:"modmails" gorm:"-"`
	Suspectreason      *string         `json:"suspectreason,omitempty" gorm:"-"`
	Activedistance     *float64        `json:"activedistance" gorm:"-"`
	Chatmodstatus      *string         `json:"chatmodstatus,omitempty" gorm:"->"`
	Newsfeedmodstatus  *string         `json:"newsfeedmodstatus,omitempty" gorm:"->"`
	Tnuserid           *uint64         `json:"tnuserid,omitempty" gorm:"->"`
	Lastpush           *time.Time      `json:"lastpush,omitempty" gorm:"-"`
	Donations          []UserDonation  `json:"donations,omitempty" gorm:"-"`
	Loginlink          string          `json:"loginlink,omitempty" gorm:"-"`
}

type UserDonation struct {
	ID              uint64     `json:"id"`
	Userid          *uint64    `json:"userid"`
	Timestamp       time.Time  `json:"timestamp"`
	GrossAmount     float64    `json:"GrossAmount"`
	Source          string     `json:"source"`
	TransactionType *string    `json:"TransactionType"`
	Giftaidconsent  bool       `json:"giftaidconsent"`
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
	Type                     string `json:"type"`
	Bbox                     string `json:"bbox"`
	Microvolunteeringallowed int    `json:"microvolunteeringallowed"`
}

type UserMessageHistory struct {
	ID         uint64    `json:"id"`
	Subject    string    `json:"subject"`
	Type       string    `json:"type"`
	Arrival    time.Time `json:"arrival"`
	Groupid    uint64    `json:"groupid"`
	Collection string    `json:"collection"`
	Daysago    int       `json:"daysago"`
	Outcome    *string   `json:"outcome"`
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

func hideSensitiveFields(user *User, myid uint64) {
	// Hide sensitive fields for non-logged in user or different user.
	// Systemrole is not hidden — it's public information (mod/admin status)
	// and is needed by the frontend for crown icons in mod logs.
	if myid != user.ID {
		user.Settings = nil
		user.Relevantallowed = false
		user.Newslettersallowed = false
		user.Bouncing = false
		user.Marketingconsent = false
		user.Source = nil
		// Mod-only fields: only visible to mods of a shared group.
		if !IsModOfUser(myid, user.ID) {
			user.Chatmodstatus = nil
			user.Newsfeedmodstatus = nil
			user.Tnuserid = nil
		}
	}
}

func GetUserByEmail(c *fiber.Ctx) error {
	email := c.Params("email")

	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email parameter required")
	}

	// Looking up a user by email
	db := database.DBConn
	var userId uint64

	// Join with users table to ensure the user exists and isn't deleted
	err := db.Raw("SELECT users.id FROM users "+
		"INNER JOIN users_emails ON users_emails.userid = users.id "+
		"WHERE users_emails.email = ? AND users.deleted IS NULL "+
		"LIMIT 1", email).Scan(&userId).Error

	if err != nil || userId == 0 {
		return c.JSON(fiber.Map{
			"exists": false,
		})
	}

	return c.JSON(fiber.Map{
		"exists": true,
	})
}

func GetUser(c *fiber.Ctx) error {
	modtools := c.Query("modtools") == "true"

	if c.Params("id") != "" {
		// Check if this is a comma-separated list of IDs (batch request).
		idsParam := c.Params("id")
		if strings.Contains(idsParam, ",") {
			// Batch request for multiple users.
			ids := strings.Split(idsParam, ",")
			myid := WhoAmI(c)

			if len(ids) > 30 {
				return fiber.NewError(fiber.StatusBadRequest, "Too many users requested")
			}

			users := GetUsersByIds(ids, myid, modtools)
			return c.JSON(users)
		}

		// Looking for a specific user.
		id, err := strconv.ParseUint(idsParam, 10, 64)

		if err == nil {
			myid := WhoAmI(c)

			user := GetUserById(id, myid)

			if user.ID != id {
				return fiber.NewError(fiber.StatusNotFound, "User not found")
			}

			hideSensitiveFields(&user, myid)
			enrichUserForModtools(&user, id, myid, modtools)

			return c.JSON(user)
		}

		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
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
	db.Raw("SELECT memberships.id, added, role, groupid, emailfrequency, eventsallowed, volunteeringallowed, microvolunteering AS microvolunteeringallowed, nameshort, namefull, groups.type, ST_AsText(ST_ENVELOPE(polyindex)) AS bbox FROM memberships INNER JOIN `groups` ON groups.id = memberships.groupid WHERE userid = ? AND collection = ?", id, "Approved").Scan(&memberships)

	for ix, r := range memberships {
		if len(r.Namefull) > 0 {
			memberships[ix].Namedisplay = r.Namefull
		} else {
			memberships[ix].Namedisplay = r.Nameshort
		}
	}

	return memberships
}

// GetActiveModGroupIDs returns group IDs where the user is an active moderator/owner.
// A moderator is "active" unless their membership settings JSON has active=0.
// A moderator is "active" unless their membership settings JSON has active=0.
func GetActiveModGroupIDs(userid uint64) []uint64 {
	db := database.DBConn
	var groupIDs []uint64
	result := db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved' "+
		"AND (settings IS NULL OR JSON_EXTRACT(settings, '$.active') IS NULL OR JSON_EXTRACT(settings, '$.active') != 0)",
		userid).Pluck("groupid", &groupIDs)
	if result.Error != nil {
		log.Printf("Failed to get active mod group IDs for user %d: %v", userid, result.Error)
	}
	return groupIDs
}

// HasWiderReview checks if a user participates in wider chat review, i.e. they are an active
// moderator on at least one group that has widerchatreview=1 in its settings.
// Checks if any of their active groups has widerchatreview=1 in settings.
func HasWiderReview(userid uint64) bool {
	db := database.DBConn
	activeGroupIDs := GetActiveModGroupIDs(userid)
	if len(activeGroupIDs) == 0 {
		return false
	}
	var count int64
	db.Raw("SELECT COUNT(*) FROM `groups` WHERE id IN ? AND JSON_EXTRACT(settings, '$.widerchatreview') = 1",
		activeGroupIDs).Scan(&count)
	return count > 0
}

func GetUserMessageHistory(userid uint64) []UserMessageHistory {
	db := database.DBConn

	var history []UserMessageHistory
	db.Raw("SELECT m.id, m.subject, m.type, "+
		"GREATEST(COALESCE(mp.date, m.arrival), COALESCE(mp.date, m.arrival)) AS arrival, "+
		"mg.groupid, mg.collection, "+
		"(SELECT outcome FROM messages_outcomes WHERE messages_outcomes.msgid = m.id ORDER BY timestamp DESC LIMIT 1) AS outcome "+
		"FROM messages m "+
		"INNER JOIN messages_groups mg ON m.id = mg.msgid "+
		"LEFT JOIN messages_postings mp ON mp.msgid = m.id "+
		"WHERE m.fromuser = ? AND mg.deleted = 0 AND m.deleted IS NULL "+
		"ORDER BY arrival DESC", userid).Scan(&history)

	now := time.Now()
	for ix, h := range history {
		history[ix].Daysago = int(now.Sub(h.Arrival).Hours() / 24)
	}

	return history
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

		err := db.Raw("SELECT users.id, firstname, lastname, fullname, lastaccess, users.added, systemrole, relevantallowed, newslettersallowed, marketingconsent, trustlevel, bouncing, deleted, forgotten, source, "+
			"chatmodstatus, newsfeedmodstatus, tnuserid, "+settingsq+
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
		Supporter     bool       `json:"supporter"`
		Donated       *time.Time `json:"donated"`
		DonatedType   *string    `json:"donatedtype"`
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
			"(SELECT MAX(timestamp) FROM users_donations WHERE userid = ?) AS donated, "+
			"(SELECT type FROM users_donations WHERE userid = ? ORDER BY timestamp DESC LIMIT 1) AS donatedtype "+
			"FROM users "+
			"WHERE users.id = ?", id, start, id, start, id, id, id).Scan(&supporter)
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
		user.DonatedType = supporter.DonatedType
	}

	if user.Deleted == nil {
		user.ExpectedReplies = len(expectedReplies)
		user.ExpectedChats = expectedReplies
	}

	return user
}

// GetUsersByIds fetches multiple users in parallel by their IDs.
func GetUsersByIds(ids []string, myid uint64, modtools bool) []User {
	var mu sync.Mutex
	users := []User{}

	var wg sync.WaitGroup
	wg.Add(len(ids))

	for _, idStr := range ids {
		go func(idStr string) {
			defer wg.Done()

			id, err := strconv.ParseUint(idStr, 10, 64)
			if err != nil {
				return
			}

			user := GetUserById(id, myid)
			hideSensitiveFields(&user, myid)

			if user.ID == id {
				mu.Lock()
				users = append(users, user)
				mu.Unlock()
			}
		}(idStr)
	}

	wg.Wait()

	// Enrich each user with modtools data (memberships, emails, etc.)
	// and fetch comments in a single batch.
	if modtools && myid > 0 && len(users) > 0 {
		for i := range users {
			enrichUserForModtools(&users[i], users[i].ID, myid, modtools)
		}

		userids := make([]uint64, len(users))
		for i, u := range users {
			userids[i] = u.ID
		}
		comments := GetComments(userids, myid)
		for i := range users {
			if c, ok := comments[users[i].ID]; ok {
				users[i].Comments = c
			}
		}
	}

	return users
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

// DeleteUserSearch soft-deletes a user search by setting deleted=1.
// The user can only delete their own searches, or admin/support can delete any.
//
// @Summary Delete a user search
// @Tags usersearch
// @Produce json
// @Param id query integer true "Search ID"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/usersearch [delete]
func DeleteUserSearch(c *fiber.Ctx) error {
	myid := WhoAmI(c)
	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	id, err := strconv.ParseUint(c.Query("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"ret": 2, "status": "Invalid id"})
	}

	db := database.DBConn

	// Check ownership.
	var search Search
	if err := db.Raw("SELECT * FROM users_searches WHERE id = ?", id).Scan(&search).Error; err != nil || search.ID == 0 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	if search.Userid != myid {
		// Check if admin/support.
		if !auth.IsAdminOrSupport(myid) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
		}
	}

	// Soft-delete: mark all searches with the same userid and term as deleted.
	db.Exec("UPDATE users_searches SET deleted = 1 WHERE userid = ? AND term = ?", search.Userid, search.Term)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
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

// SearchUsers searches across users by name, email, ID, yahooid, or login UID.
// Requires Admin or Support role.
func SearchUsers(c *fiber.Ctx) error {
	myid := WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	if !auth.IsAdminOrSupport(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	q := c.Query("q")
	if q == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Search term required")
	}

	numericID, _ := strconv.ParseUint(q, 10, 64)

	// If query is purely numeric, do a fast direct ID lookup first.
	if numericID > 0 {
		var exists uint64
		db.Raw("SELECT id FROM users WHERE id = ?", numericID).Scan(&exists)
		if exists > 0 {
			// Found by ID — skip the slow LIKE searches.
			return c.JSON(fiber.Map{"users": []uint64{exists}})
		}
	}

	// Use prefix match (term%) for canon/fullname/yahooid/uid,
	// and reversed prefix match for backwards column. Substring match (%term%)
	// on email only. This is faster (uses indexes) and more precise.
	prefixTerm := q + "%"
	emailLikeTerm := "%" + q + "%"
	reversed := reverseString(q)
	backwardsTerm := reversed + "%"

	var userIDs []uint64
	db.Raw("SELECT DISTINCT userid FROM ("+
		"(SELECT userid FROM users_emails WHERE email LIKE ? OR canon LIKE ? OR backwards LIKE ?) "+
		"UNION "+
		"(SELECT id AS userid FROM users WHERE fullname LIKE ?) "+
		"UNION "+
		"(SELECT id AS userid FROM users WHERE yahooid LIKE ?) "+
		"UNION "+
		"(SELECT id AS userid FROM users WHERE id = ?) "+
		"UNION "+
		"(SELECT userid FROM users_logins WHERE uid LIKE ?) "+
		") t ORDER BY userid ASC LIMIT 100",
		emailLikeTerm, prefixTerm, backwardsTerm, prefixTerm, prefixTerm, numericID, prefixTerm).Pluck("userid", &userIDs)

	return c.JSON(fiber.Map{"users": userIDs})
}


func generateRandomKey(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// enrichUserForModtools adds modtools-specific data to a user when modtools=true.
// enrichUserForModtools adds modtools-specific data to a user when modtools=true.
// This includes memberships, emails, messagehistory, location, comments, donations, etc.
func enrichUserForModtools(u *User, id uint64, myid uint64, modtools bool) {
	db := database.DBConn

	var memberships []Membership
	var emails []UserEmail
	var messageHistory []UserMessageHistory
	var privatePos utils.LatLng
	var publicLoc *Publiclocation
	var modmails uint64
	var wg sync.WaitGroup

	// Fetch memberships for authenticated requests only.
	if myid > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			memberships = GetMemberships(id)
		}()
	}

	// Emails: only if caller is mod of user.
	if myid > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if IsModOfUser(myid, id) || id == myid {
				emails = getEmails(id)
			}
		}()
	}

	if modtools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messageHistory = GetUserMessageHistory(id)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			privatePos = GetLatLng(id)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			publicLoc = GetPublicLocationForUser(id)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			modGroupIDs := GetActiveModGroupIDs(myid)
			if len(modGroupIDs) > 0 {
				db.Raw("SELECT COUNT(*) FROM users_modmails WHERE userid = ? AND groupid IN (?)", id, modGroupIDs).Scan(&modmails)
			}
		}()
	}

	var lastpush *time.Time
	if modtools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Raw("SELECT MAX(lastsent) FROM users_push_notifications WHERE userid = ?", id).Scan(&lastpush)
		}()
	}

	var activedistance *float64
	callerIsMod := false
	if modtools && myid > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			callerIsMod = IsModOfUser(myid, id)
		}()
	}

	var suspectReasonResult string
	if modtools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Raw("SELECT reviewreason FROM memberships WHERE userid = ? AND reviewreason IS NOT NULL AND reviewreason != '' LIMIT 1", id).Scan(&suspectReasonResult)
		}()
	}

	if modtools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			type groupLatLng struct {
				Lat float64
				Lng float64
			}
			var locs []groupLatLng
			db.Raw("SELECT DISTINCT g.lat, g.lng FROM memberships_history mh "+
				"INNER JOIN `groups` g ON mh.groupid = g.id "+
				"WHERE mh.userid = ? AND DATEDIFF(NOW(), mh.added) <= 31 "+
				"AND g.publish = 1 AND g.onmap = 1 AND g.lat != 0 AND g.lng != 0",
				id).Scan(&locs)
			if len(locs) >= 2 {
				var swlat, swlng, nelat, nelng float64
				swlat, swlng = locs[0].Lat, locs[0].Lng
				nelat, nelng = locs[0].Lat, locs[0].Lng
				for _, loc := range locs[1:] {
					if loc.Lat < swlat {
						swlat = loc.Lat
					}
					if loc.Lng < swlng {
						swlng = loc.Lng
					}
					if loc.Lat > nelat {
						nelat = loc.Lat
					}
					if loc.Lng > nelng {
						nelng = loc.Lng
					}
				}
				dist := utils.Haversine(swlat, swlng, nelat, nelng)
				rounded := math.Round(dist)
				activedistance = &rounded
			}
		}()
	}

	wg.Wait()

	u.Memberships = memberships
	u.MessageHistory = messageHistory
	u.Modmails = modmails

	if callerIsMod || myid == id || auth.IsAdminOrSupport(myid) {
		if suspectReasonResult != "" {
			u.Suspectreason = &suspectReasonResult
		}
		u.Activedistance = activedistance
		u.Lastpush = lastpush
	}

	if modtools {
		if privatePos.Lat != 0 || privatePos.Lng != 0 {
			var locName string
			db.Raw("SELECT JSON_UNQUOTE(JSON_EXTRACT(JSON_EXTRACT(settings, '$.mylocation'), '$.name')) "+
				"FROM users WHERE id = ? AND settings IS NOT NULL", id).Scan(&locName)

			if locName == "" || locName == "null" {
				locName = ""
				if u.Lastlocation != nil && *u.Lastlocation > 0 {
					db.Raw("SELECT name FROM locations WHERE id = ?", *u.Lastlocation).Scan(&locName)
				}
			}

			if locName == "" && (privatePos.Lat != 0 || privatePos.Lng != 0) {
				db.Raw("SELECT name FROM locations WHERE type = 'Postcode' "+
					"AND lat BETWEEN ? AND ? AND lng BETWEEN ? AND ? "+
					"ORDER BY ((lat - ?)*(lat - ?) + (lng - ?)*(lng - ?)) ASC LIMIT 1",
					float64(privatePos.Lat)-0.1, float64(privatePos.Lat)+0.1,
					float64(privatePos.Lng)-0.1, float64(privatePos.Lng)+0.1,
					privatePos.Lat, privatePos.Lat, privatePos.Lng, privatePos.Lng).Scan(&locName)
			}

			u.Privateposition = &PrivatePosition{
				Lat:  privatePos.Lat,
				Lng:  privatePos.Lng,
				Name: locName,
				Loc:  locName,
			}
		}
		if publicLoc != nil {
			u.Info.Publiclocation = publicLoc
		}
	}

	if len(emails) > 0 {
		u.Emails = emails
		for _, email := range emails {
			if u.Email == "" && utils.OurDomain(email.Email) == 0 {
				u.Email = email.Email
			}
		}
	}

	if modtools && myid > 0 {
		comments := GetComments([]uint64{id}, myid)
		if c, ok := comments[id]; ok {
			u.Comments = c
		}

		if auth.HasPermission(myid, auth.PERM_GIFTAID) {
			var donations []UserDonation
			db.Raw("SELECT id, userid, timestamp, GrossAmount, source, TransactionType, giftaidconsent FROM users_donations WHERE userid = ? ORDER BY timestamp DESC", id).Scan(&donations)
			if len(donations) > 0 {
				u.Donations = donations
			}
		}

		if auth.IsAdminOrSupport(myid) {
			// Generate login link for impersonation.
			// Admin can impersonate anyone, support can impersonate non-mods.
			isAdmin := auth.IsAdmin(myid)
			canImpersonate := isAdmin || !auth.IsSystemMod(id)
			if canImpersonate {
				var key string
				db.Raw("SELECT credentials FROM users_logins WHERE userid = ? AND type = 'Link' LIMIT 1", id).Scan(&key)
				if key == "" {
					key = generateRandomKey(32)
					db.Exec("INSERT INTO users_logins (userid, type, credentials) VALUES (?, 'Link', ?)", id, key)
				}
				if key != "" {
					userSite := os.Getenv("USER_SITE")
					if userSite == "" {
						userSite = "www.ilovefreegle.org"
					}
					u.Loginlink = fmt.Sprintf("https://%s/?u=%d&k=%s", userSite, id, key)
				}
			}
		}
	}
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

					// Set processingrequired for background processing (welcome email, spam check, etc).
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

				// Welcome email, spam check, and member review are handled by the
				// background cron (memberships_processing) which picks up rows
				// with processingrequired=1 in memberships_history.
			}
		}
	}

	return ret
}

type UserPostRequest struct {
	Action    string  `json:"action"`
	Engageid  uint64  `json:"engageid"`
	Ratee     uint64  `json:"ratee"`
	Rating    *string `json:"rating"`
	Reason    *string `json:"reason"`
	Text      *string `json:"text"`
	Ratingid  uint64  `json:"ratingid"`
	ID        uint64  `json:"id"`
	Email     string  `json:"email"`
	Primary   *bool   `json:"primary"`
	ID1       uint64  `json:"id1"`
	ID2       uint64  `json:"id2"`
}

func PostUser(c *fiber.Ctx) error {
	myid := WhoAmI(c)

	var req UserPostRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	// Engaged doesn't require login.
	if req.Engageid > 0 {
		return handleEngaged(c, db, req.Engageid)
	}

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	switch req.Action {
	case "Rate":
		return handleRate(c, db, myid, req)
	case "RatingReviewed":
		return handleRatingReviewed(c, db, myid, req)
	case "AddEmail":
		return handleAddEmail(c, db, myid, req)
	case "RemoveEmail":
		return handleRemoveEmail(c, db, myid, req)
	case "Unbounce":
		return handleUnbounce(c, myid, req)
	case "Merge":
		return handleMerge(c, myid, req)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action")
	}
}

func handleEngaged(c *fiber.Ctx, db *gorm.DB, engageid uint64) error {
	// Record engagement success.
	var mailid uint64
	db.Raw("SELECT mailid FROM engage WHERE id = ?", engageid).Scan(&mailid)

	if mailid > 0 {
		db.Exec("UPDATE engage SET succeeded = NOW() WHERE id = ?", engageid)
		db.Exec("UPDATE engage_mails SET action = action + 1, rate = COALESCE(100 * action / shown, 0) WHERE id = ?", mailid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleRate(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Ratee == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ratee is required")
	}

	// Validate rating value.
	if req.Rating != nil && *req.Rating != utils.RATING_UP && *req.Rating != utils.RATING_DOWN {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid rating value")
	}

	// Can't rate yourself.
	if req.Ratee == myid {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot rate yourself")
	}

	// Determine if review is required (down-vote with reason and text).
	reviewRequired := false
	if req.Rating != nil && *req.Rating == utils.RATING_DOWN && req.Reason != nil && req.Text != nil {
		reviewRequired = true
	}

	db.Exec("REPLACE INTO ratings (rater, ratee, rating, reason, text, timestamp, reviewrequired) VALUES (?, ?, ?, ?, ?, NOW(), ?)",
		myid, req.Ratee, req.Rating, req.Reason, req.Text, reviewRequired)

	// Update lastupdated for both users.
	db.Exec("UPDATE users SET lastupdated = NOW() WHERE id IN (?, ?)", myid, req.Ratee)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleRatingReviewed(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Ratingid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ratingid is required")
	}

	// Verify the caller is admin/support or a mod of a group the ratee belongs to.
	if !auth.IsAdminOrSupport(myid) {
		var count int64
		db.Raw(`SELECT COUNT(*) FROM ratings r
			JOIN memberships m1 ON m1.userid = r.ratee
			JOIN memberships m2 ON m2.groupid = m1.groupid AND m2.userid = ?
			WHERE r.id = ? AND m2.role IN ('Moderator', 'Owner')`, myid, req.Ratingid).Scan(&count)
		if count == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Not authorized to review this rating")
		}
	}

	db.Exec("UPDATE ratings SET reviewrequired = 0 WHERE id = ?", req.Ratingid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

func handleAddEmail(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email is required")
	}

	email := strings.TrimSpace(req.Email)
	targetID := req.ID
	if targetID == 0 {
		targetID = myid
	}

	// Only allow if admin/support or own account.
	if targetID != myid {
		if !auth.IsAdminOrSupport(myid) {
			return fiber.NewError(fiber.StatusForbidden, "You cannot administer those users")
		}
	}

	// Check if email is already in use by another user.
	var existingUID uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? AND userid IS NOT NULL", email).Scan(&existingUID)

	if existingUID > 0 && existingUID != targetID {
		// Email is used by a different user.
		if !auth.IsAdminOrSupport(myid) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"ret": 3, "status": "Email already used"})
		}
		// Admin/support: remove from original user before reassigning.
		db.Exec("DELETE FROM users_emails WHERE email = ? AND userid = ?", email, existingUID)
	}

	// Add the email.
	isPrimary := true
	if req.Primary != nil {
		isPrimary = *req.Primary
	}

	var primaryVal int
	if isPrimary {
		primaryVal = 1
	}

	result := db.Exec("INSERT INTO users_emails (userid, email, preferred, validated, canon) VALUES (?, ?, ?, NOW(), ?)",
		targetID, email, primaryVal, CanonicalizeEmail(email))

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"ret": 4, "status": "Email add failed"})
	}

	var emailID uint64
	db.Raw("SELECT id FROM users_emails WHERE userid = ? AND email = ? ORDER BY id DESC LIMIT 1", targetID, email).Scan(&emailID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "emailid": emailID})
}

func handleRemoveEmail(c *fiber.Ctx, db *gorm.DB, myid uint64, req UserPostRequest) error {
	if req.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email is required")
	}

	targetID := req.ID
	if targetID == 0 {
		targetID = myid
	}

	// Only allow if admin/support or own account.
	if targetID != myid {
		if !auth.IsAdminOrSupport(myid) {
			return fiber.NewError(fiber.StatusForbidden, "You cannot administer those users")
		}
	}

	// Verify email belongs to this user.
	var emailUserid uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? AND userid = ?", req.Email, targetID).Scan(&emailUserid)

	if emailUserid == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"ret": 3, "status": "Not on same user"})
	}

	db.Exec("DELETE FROM users_emails WHERE email = ? AND userid = ?", req.Email, targetID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// CanonicalizeEmail returns a canonical form of the email for deduplication.
func CanonicalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email
	}
	// Remove dots and plus-addressing from local part for Gmail-style canonicalization.
	local := strings.ReplaceAll(parts[0], ".", "")
	if idx := strings.Index(local, "+"); idx >= 0 {
		local = local[:idx]
	}
	return local + "@" + parts[1]
}

// UserPutRequest is the body for PUT /user (signup).
type UserPutRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Firstname   string `json:"firstname"`
	Lastname    string `json:"lastname"`
	Displayname string `json:"displayname"`
	GroupID     uint64 `json:"groupid"`
}

// UserPatchRequest is the body for PATCH /user (profile update).
type UserPatchRequest struct {
	ID                  uint64           `json:"id"`
	Displayname         *string          `json:"displayname,omitempty"`
	Settings            *json.RawMessage `json:"settings,omitempty"`
	Onholidaytill       *string          `json:"onholidaytill,omitempty"`
	Relevantallowed     *int             `json:"relevantallowed,omitempty"`
	Newslettersallowed  *int             `json:"newslettersallowed,omitempty"`
	Aboutme             *string          `json:"aboutme,omitempty"`
	Newsfeedmodstatus   *string          `json:"newsfeedmodstatus,omitempty"`
	Email               *string          `json:"email,omitempty"`
	Source              *string          `json:"source,omitempty"`
}

// UserDeleteRequest is the body for DELETE /user.
type UserDeleteRequest struct {
	ID uint64 `json:"id"`
}

// PutUser creates a new user (signup).
//
// @Summary Create/signup a new user
// @Tags user
// @Accept json
// @Produce json
// @Param body body UserPutRequest true "Signup details"
// @Success 200 {object} map[string]interface{}
// @Router /user [put]
func PutUser(c *fiber.Ctx) error {
	var req UserPutRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email is required")
	}

	email := strings.TrimSpace(req.Email)
	db := database.DBConn

	// Check if email already exists.
	var existingUID uint64
	db.Raw("SELECT userid FROM users_emails WHERE email = ? LIMIT 1", email).Scan(&existingUID)

	if existingUID > 0 {
		// If they provided a correct password, treat signup as login — avoids
		// forcing users to switch to the login screen and re-enter credentials.
		if req.Password != "" && auth.VerifyPassword(existingUID, req.Password) {
			persistent, jwtString, err := auth.CreateSessionAndJWT(existingUID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "Failed to create session")
			}
			return c.JSON(fiber.Map{
				"ret":        0,
				"status":     "Success",
				"id":         existingUID,
				"persistent": persistent,
				"jwt":        jwtString,
			})
		}

		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"ret":    2,
			"status": "That email is already in use",
		})
	}

	// Build display name from parts.
	fullname := strings.TrimSpace(req.Displayname)
	if fullname == "" {
		parts := []string{}
		if req.Firstname != "" {
			parts = append(parts, req.Firstname)
		}
		if req.Lastname != "" {
			parts = append(parts, req.Lastname)
		}
		fullname = strings.Join(parts, " ")
	}

	var firstname *string
	var lastname *string
	if req.Firstname != "" {
		firstname = &req.Firstname
	}
	if req.Lastname != "" {
		lastname = &req.Lastname
	}

	// Create user.  Use raw database/sql to get LastInsertId() from the
	// same result — avoids the GORM connection-pool race where a separate
	// SELECT LAST_INSERT_ID() query could land on a different connection.
	sqlDB, err := db.DB()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get DB connection")
	}

	sqlResult, err := sqlDB.Exec("INSERT INTO users (fullname, firstname, lastname, added) VALUES (?, ?, ?, NOW())",
		fullname, firstname, lastname)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create user")
	}

	newUserIDInt, err := sqlResult.LastInsertId()
	if err != nil || newUserIDInt == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get new user ID")
	}
	newUserID := uint64(newUserIDInt)

	// Add email.
	canon := CanonicalizeEmail(email)
	db.Exec("INSERT INTO users_emails (userid, email, preferred, validated, canon) VALUES (?, ?, 1, NOW(), ?)",
		newUserID, email, canon)

	// Generate random password if none provided (for email-only signup).
	// The client shows this to the user in the welcome modal.
	password := req.Password
	if password == "" {
		password = utils.RandomHex(4) // 8 char random hex password
	}

	// Hash with sha1+salt and store.
	salt := os.Getenv("PASSWORD_SALT")
	if salt == "" {
		salt = "zzzz"
	}
	h := sha1.New()
	h.Write([]byte(password + salt))
	hashed := hex.EncodeToString(h.Sum(nil))
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?)",
		newUserID, newUserID, hashed, salt)

	// If groupid provided, add membership.
	if req.GroupID > 0 {
		db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Approved')",
			newUserID, req.GroupID)
	}

	// Create a session. Series is a numeric value; token is a random string.
	token := utils.RandomHex(16)
	db.Exec("INSERT INTO sessions (userid, series, token, lastactive) VALUES (?, ?, ?, NOW())",
		newUserID, newUserID, token)

	var sessionID uint64
	db.Raw("SELECT id FROM sessions WHERE userid = ? ORDER BY id DESC LIMIT 1", newUserID).Scan(&sessionID)

	// Generate JWT.
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":        fmt.Sprint(newUserID),
		"sessionid": fmt.Sprint(sessionID),
		"exp":       time.Now().Unix() + 30*24*60*60, // 30 days
	})

	jwtString, err := jwtToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to generate JWT")
	}

	resp := fiber.Map{
		"ret":    0,
		"status": "Success",
		"id":     newUserID,
		"persistent": fiber.Map{
			"id":     sessionID,
			"series": newUserID,
			"token":  token,
			"userid": newUserID,
		},
		"jwt": jwtString,
	}

	// Return the generated password so the client can show it in the welcome modal.
	if req.Password == "" {
		resp["password"] = password
	}

	return c.JSON(resp)
}

// PatchUser updates user profile fields.
//
// @Summary Update user profile
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user [patch]
func PatchUser(c *fiber.Ctx) error {
	myid := WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req UserPatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	// Handle newsfeedmodstatus for another user (mod action).
	if req.Newsfeedmodstatus != nil && req.ID > 0 && req.ID != myid {
		// Verify caller is admin/support or mod of a shared group.
		if !auth.IsAdminOrSupport(myid) {
			// Check if they share a group where the caller is a mod.
			var sharedModGroup int64
			db.Raw("SELECT COUNT(*) FROM memberships m1 "+
				"INNER JOIN memberships m2 ON m1.groupid = m2.groupid "+
				"WHERE m1.userid = ? AND m2.userid = ? AND m1.role IN ('Owner', 'Moderator')",
				myid, req.ID).Scan(&sharedModGroup)

			if sharedModGroup == 0 {
				return fiber.NewError(fiber.StatusForbidden, "Not authorized to moderate this user")
			}
		}

		db.Exec("UPDATE users SET newsfeedmodstatus = ? WHERE id = ?", *req.Newsfeedmodstatus, req.ID)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// All other updates apply to the logged-in user.
	if req.Displayname != nil {
		db.Exec("UPDATE users SET fullname = ?, firstname = NULL, lastname = NULL WHERE id = ?",
			*req.Displayname, myid)
	}

	if req.Settings != nil {
		settingsJSON, err := json.Marshal(req.Settings)
		if err == nil {
			db.Exec("UPDATE users SET settings = ? WHERE id = ?", string(settingsJSON), myid)
		}
	}

	if req.Onholidaytill != nil {
		if *req.Onholidaytill == "" {
			db.Exec("UPDATE users SET onholidaytill = NULL WHERE id = ?", myid)
		} else {
			db.Exec("UPDATE users SET onholidaytill = ? WHERE id = ?", *req.Onholidaytill, myid)
		}
	}

	if req.Relevantallowed != nil {
		db.Exec("UPDATE users SET relevantallowed = ? WHERE id = ?", *req.Relevantallowed, myid)
	}

	if req.Newslettersallowed != nil {
		db.Exec("UPDATE users SET newslettersallowed = ? WHERE id = ?", *req.Newslettersallowed, myid)
	}

	if req.Aboutme != nil {
		// Insert a new aboutme entry. The most recent is fetched via ORDER BY timestamp DESC LIMIT 1.
		db.Exec("INSERT INTO users_aboutme (userid, text, timestamp) VALUES (?, ?, NOW())", myid, *req.Aboutme)
	}

	if req.Newsfeedmodstatus != nil {
		// Self-update (no req.ID or req.ID == myid).
		db.Exec("UPDATE users SET newsfeedmodstatus = ? WHERE id = ?", *req.Newsfeedmodstatus, myid)
	}

	if req.Email != nil && *req.Email != "" {
		// Queue email verification rather than adding directly.
		// New addresses must be verified before being linked to the account.
		if err := queue.QueueTask(queue.TaskEmailVerify, map[string]interface{}{
			"user_id": myid,
			"email":   strings.TrimSpace(*req.Email),
		}); err != nil {
			// Log but don't fail the whole request.
			fmt.Printf("Failed to queue email verify for user %d: %v\n", myid, err)
		}
	}

	if req.Source != nil {
		db.Exec("UPDATE users SET source = ? WHERE id = ?", *req.Source, myid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteUser purges/deletes a user.
//
// @Summary Delete/purge a user
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user [delete]
func DeleteUser(c *fiber.Ctx) error {
	myid := WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Parse the target user ID from body or query.
	var req UserDeleteRequest
	_ = c.BodyParser(&req) // Ignore parse errors - body is optional, query param fallback below.

	if req.ID == 0 {
		// Try query parameter.
		if idStr := c.Query("id"); idStr != "" {
			fmt.Sscanf(idStr, "%d", &req.ID)
		}
	}

	targetID := req.ID
	if targetID == 0 {
		// Self-delete.
		targetID = myid
	}

	if targetID != myid {
		// Deleting another user requires admin/support.
		if !auth.IsAdminOrSupport(myid) {
			return fiber.NewError(fiber.StatusForbidden, "Only admin/support can delete other users")
		}

		// Cannot delete moderators/owners — they must demote themselves first.
		var targetModRole string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') LIMIT 1", targetID).Scan(&targetModRole)

		if targetModRole != "" {
			return fiber.NewError(fiber.StatusForbidden, "Cannot delete a moderator/owner — they must demote first")
		}
	} else {
		// Self-delete checks: moderators must demote first, spammers cannot self-delete.
		var modRole string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') LIMIT 1", myid).Scan(&modRole)

		if modRole != "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"ret":    2,
				"status": "Please demote yourself to a member first",
			})
		}

		var spammerCount int64
		db.Raw("SELECT COUNT(*) FROM spam_users WHERE userid = ? AND collection IN ('Spammer', 'PendingAdd')", myid).Scan(&spammerCount)

		if spammerCount > 0 {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"ret":    3,
				"status": "We can't do this.",
			})
		}
	}

	// Remove memberships so the user no longer appears in group member lists.
	db.Exec("DELETE FROM memberships WHERE userid = ? AND collection = 'Approved'", targetID)

	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", targetID)

	// Log the deletion (type='User', subtype='Deleted').
	db.Exec("INSERT INTO logs (timestamp, type, subtype, user, byuser) VALUES (NOW(), ?, ?, ?, ?)",
		log2.LOG_TYPE_USER, log2.LOG_SUBTYPE_DELETED, targetID, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleUnbounce resets the bouncing flag on a user. Admin/Support only.
func handleUnbounce(c *fiber.Ctx, myid uint64, req UserPostRequest) error {
	db := database.DBConn

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	// Require admin/support.
	if !auth.IsAdminOrSupport(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Only admin/support can unbounce users")
	}

	db.Exec("UPDATE users SET bouncing = 0 WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// handleMerge merges user id2 into user id1. Admin/Support only.
func handleMerge(c *fiber.Ctx, myid uint64, req UserPostRequest) error {
	db := database.DBConn

	if req.ID1 == 0 || req.ID2 == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id1 and id2 are required")
	}

	if req.ID1 == req.ID2 {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot merge a user with themselves")
	}

	// Require admin/support.
	if !auth.IsAdminOrSupport(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Only admin/support can merge users")
	}

	// Move references from id2 to id1 - run independent writes in parallel.
	var wg sync.WaitGroup
	wg.Add(5)

	go func() {
		defer wg.Done()
		db.Exec("UPDATE messages SET fromuser = ? WHERE fromuser = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE chat_rooms SET user1 = ? WHERE user1 = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE chat_rooms SET user2 = ? WHERE user2 = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE chat_messages SET userid = ? WHERE userid = ?", req.ID1, req.ID2)
	}()
	go func() {
		defer wg.Done()
		db.Exec("UPDATE users_emails SET userid = ? WHERE userid = ?", req.ID1, req.ID2)
	}()

	wg.Wait()

	// Memberships must be sequential: move non-duplicates, then delete remaining, then mark deleted.
	db.Exec("UPDATE IGNORE memberships SET userid = ? WHERE userid = ?", req.ID1, req.ID2)
	db.Exec("DELETE FROM memberships WHERE userid = ?", req.ID2)
	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", req.ID2)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// All endpoints in this file are mod-only: the caller must be a moderator of
// a group the target user belongs to (or Admin/Support).  Each returns a flat
// array — no nested enrichment.

func requireModOfUser(c *fiber.Ctx) (myid, targetid uint64, err error) {
	myid = WhoAmI(c)
	if myid == 0 {
		return 0, 0, fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}
	targetid, parseErr := strconv.ParseUint(c.Params("id"), 10, 64)
	if parseErr != nil || targetid == 0 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	if !IsModOfUser(myid, targetid) {
		return 0, 0, fiber.NewError(fiber.StatusForbidden, "Not a moderator for this user")
	}
	return myid, targetid, nil
}

// GetUserChatrooms returns chat rooms for a target user.
//
// @Summary Get chat rooms for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/chatrooms [get]
func GetUserChatrooms(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type ChatroomRow struct {
		ID       uint64     `json:"id"`
		Chattype string     `json:"chattype"`
		User1    uint64     `json:"user1"`
		User2    uint64     `json:"user2"`
		Groupid  uint64     `json:"groupid"`
		Lastdate *time.Time `json:"lastdate"`
	}

	var rooms []ChatroomRow
	db.Raw("SELECT id, chattype, user1, user2, COALESCE(groupid, 0) AS groupid, latestmessage AS lastdate "+
		"FROM chat_rooms WHERE (user1 = ? OR user2 = ?) "+
		"ORDER BY latestmessage DESC",
		targetid, targetid).Scan(&rooms)

	if rooms == nil {
		rooms = []ChatroomRow{}
	}

	return c.JSON(rooms)
}

// GetUserEmailHistory returns recent emails sent to a user.
//
// @Summary Get email history for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/emailhistory [get]
func GetUserEmailHistory(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type EmailHistoryRow struct {
		ID        uint64     `json:"id"`
		Timestamp *time.Time `json:"timestamp"`
		Eximid    *string    `json:"eximid"`
		From      *string    `json:"from"`
		To        *string    `json:"to"`
		Subject   *string    `json:"subject"`
		Status    *string    `json:"status"`
	}

	var emails []EmailHistoryRow
	db.Raw("SELECT id, timestamp, eximid, `from`, `to`, subject, status "+
		"FROM logs_emails WHERE userid = ? ORDER BY id DESC LIMIT 100",
		targetid).Scan(&emails)

	if emails == nil {
		emails = []EmailHistoryRow{}
	}

	return c.JSON(emails)
}

// GetUserBans returns ban records for a user.
//
// @Summary Get bans for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/bans [get]
func GetUserBans(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type BanRow struct {
		Groupid uint64     `json:"groupid"`
		Group   string     `json:"group"`
		Date    *time.Time `json:"date"`
		Byuser  *uint64    `json:"byuser"`
		Byemail *string    `json:"byemail"`
	}

	var bans []BanRow
	db.Raw("SELECT ub.groupid, "+
		"COALESCE(g.namefull, g.nameshort) AS `group`, "+
		"ub.date, ub.byuser, "+
		"(SELECT ue.email FROM users_emails ue WHERE ue.userid = ub.byuser AND ue.preferred = 1 LIMIT 1) AS byemail "+
		"FROM users_banned ub "+
		"LEFT JOIN `groups` g ON g.id = ub.groupid "+
		"WHERE ub.userid = ? ORDER BY ub.date DESC",
		targetid).Scan(&bans)

	if bans == nil {
		bans = []BanRow{}
	}

	return c.JSON(bans)
}

// GetUserNewsfeed returns ChitChat posts by a user.
//
// @Summary Get newsfeed posts for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/newsfeed [get]
func GetUserNewsfeed(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type NewsfeedRow struct {
		ID        uint64     `json:"id"`
		Timestamp *time.Time `json:"timestamp"`
		Message   *string    `json:"message"`
		Hidden    *time.Time `json:"hidden"`
		Hiddenby  *uint64    `json:"hiddenby"`
		Deleted   *time.Time `json:"deleted"`
		Deletedby *uint64    `json:"deletedby"`
	}

	var posts []NewsfeedRow
	db.Raw("SELECT id, timestamp, message, hidden, hiddenby, deleted, deletedby "+
		"FROM newsfeed WHERE userid = ? "+
		"ORDER BY id DESC",
		targetid).Scan(&posts)

	if posts == nil {
		posts = []NewsfeedRow{}
	}

	return c.JSON(posts)
}

// GetUserApplied returns recent group applications (last 31 days).
//
// @Summary Get recent group applications for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/applied [get]
func GetUserApplied(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type AppliedRow struct {
		Groupid     uint64     `json:"groupid"`
		Nameshort   string     `json:"nameshort"`
		Namefull    string     `json:"namefull"`
		Namedisplay string     `json:"namedisplay" gorm:"column:namedisplay"`
		Added       *time.Time `json:"added"`
	}

	var applied []AppliedRow
	db.Raw("SELECT mh.groupid, g.nameshort, COALESCE(g.namefull, '') AS namefull, COALESCE(g.namefull, g.nameshort) AS namedisplay, mh.added "+
		"FROM memberships_history mh "+
		"INNER JOIN `groups` g ON g.id = mh.groupid "+
		"WHERE mh.userid = ? AND DATEDIFF(NOW(), mh.added) <= 31 "+
		"AND g.publish = 1 AND g.onmap = 1 "+
		"ORDER BY mh.added DESC",
		targetid).Scan(&applied)

	if applied == nil {
		applied = []AppliedRow{}
	}

	return c.JSON(applied)
}

// GetUserMembershipHistory returns full membership history.
//
// @Summary Get membership history for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/membershiphistory [get]
func GetUserMembershipHistory(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type MembershipHistoryRow struct {
		Timestamp   *time.Time `json:"timestamp"`
		Type        string     `json:"type"`
		Groupid     uint64     `json:"groupid"`
		Nameshort   string     `json:"nameshort"`
		Namefull    string     `json:"namefull"`
		Namedisplay string     `json:"namedisplay" gorm:"column:namedisplay"`
	}

	var history []MembershipHistoryRow
	db.Raw("SELECT l.timestamp, l.subtype AS type, l.groupid, "+
		"g.nameshort, COALESCE(g.namefull, '') AS namefull, COALESCE(g.namefull, g.nameshort) AS namedisplay "+
		"FROM logs l "+
		"INNER JOIN `groups` g ON g.id = l.groupid "+
		"WHERE l.user = ? AND l.type = 'Group' "+
		"AND l.subtype IN ('Joined','Approved','Rejected','Applied','Left') "+
		"ORDER BY l.id DESC",
		targetid).Scan(&history)

	if history == nil {
		history = []MembershipHistoryRow{}
	}

	return c.JSON(history)
}


// GetUserLogins returns login history for a user.
//
// @Summary Get login history for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/logins [get]
func GetUserLogins(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type LoginRow struct {
		ID        uint64     `json:"id"`
		Userid    uint64     `json:"userid"`
		Type      string     `json:"type"`
		Added     *time.Time `json:"added"`
		Lastaccess *time.Time `json:"lastaccess"`
	}

	var logins []LoginRow
	db.Raw("SELECT id, userid, type, added, lastaccess FROM users_logins "+
		"WHERE userid = ? ORDER BY lastaccess DESC LIMIT 50",
		targetid).Scan(&logins)

	if logins == nil {
		logins = []LoginRow{}
	}

	return c.JSON(logins)
}

