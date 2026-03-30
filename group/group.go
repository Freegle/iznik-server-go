package group

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const FREEGLE = utils.GROUP_TYPE_FREEGLE

// Full group details.
type Group struct {
	ID                   uint64           `json:"id" gorm:"primary_key"`
	Nameshort            string           `json:"nameshort"`
	Namefull             string           `json:"namefull"`
	Namedisplay          string           `json:"namedisplay"`
	Settings             json.RawMessage  `json:"settings"` // This is JSON stored in the DB as a string.
	Rules                json.RawMessage  `json:"rules"`    // Group rules, nullable JSON.
	Region               string           `json:"region"`
	Logo                 string           `json:"logo"`
	Publish              int              `json:"publish"`
	Ontn                 int              `json:"ontn"`
	Membercount          int              `json:"membercount"`
	Modcount             int              `json:"modcount"`
	Lat                  float32          `json:"lat"`
	Lng                  float32          `json:"lng"`
	Altlat               float32          `json:"altlat"`
	Altlng               float32          `json:"altlng"`
	GroupProfile         GroupProfile     `gorm:"ForeignKey:groupid" json:"-"`
	GroupProfileStr      string           `json:"profile"`
	Onmap                int              `json:"onmap"`
	Tagline              string           `json:"tagline"`
	Description          string           `json:"description"`
	Contactmail          string           `json:"-"`
	Modsemail            string           `json:"modsemail"`
	Fundingtarget        int              `json:"fundingtarget"`
	Affiliationconfirmed time.Time        `json:"affiliationconfirmed"`
	Founded              time.Time        `json:"founded"`
	GroupSponsors        []GroupSponsor   `gorm:"ForeignKey:groupid" json:"sponsors"`
	GroupVolunteers      []GroupVolunteer `gorm:"-" json:"showmods"`
	Showjoin               int              `json:"showjoin"`
	Bbox                   string           `json:"bbox,omitempty" gorm:"column:bbox"`
	Type                   string           `json:"type"`
	Overridemoderation     string           `json:"overridemoderation"`
	Autofunctionoverride   int              `json:"autofunctionoverride"`
	Microvolunteering      int              `json:"microvolunteering"`
	Microvolunteeringoptions json.RawMessage `json:"microvolunteeringoptions"`
	Mentored               int              `json:"mentored" gorm:"column:mentored"`
	Onhere                 int              `json:"onhere" gorm:"column:onhere"`
	Onlovejunk             int              `json:"onlovejunk" gorm:"column:onlovejunk"`
	Myrole                 string           `json:"myrole,omitempty" gorm:"-"`

	// Polygon fields (only populated when polygon=true query param)
	Poly           *string `json:"poly,omitempty" gorm:"-"`
	Polyofficial   *string `json:"polyofficial,omitempty" gorm:"-"`
	Postvisibility *string `json:"postvisibility,omitempty" gorm:"-"`
	Cga            *string `json:"cga,omitempty" gorm:"-"`
	Dpa            *string `json:"dpa,omitempty" gorm:"-"`

	// TN key fields (only populated when tnkey=true and user is moderator)
	Tnkey *string `json:"tnkey,omitempty" gorm:"-"`
	Tnur  *string `json:"tnur,omitempty" gorm:"-"`
}

// Summary group details.
type GroupEntry struct {
	ID          uint64  `json:"id" gorm:"primary_key"`
	Nameshort   string  `json:"nameshort"`
	Namefull    string  `json:"namefull"`
	Namedisplay string  `json:"namedisplay"`
	Lat         float32 `json:"lat"`
	Lng         float32 `json:"lng"`
	Altlat      float32 `json:"altlat"`
	Altlng      float32 `json:"altlng"`
	Publish     int     `json:"publish"`
	Onmap       int     `json:"onmap"`
	Onhere      int     `json:"onhere" gorm:"column:onhere"`
	Ontn        int     `json:"ontn" gorm:"column:ontn"`
	Onlovejunk  int     `json:"onlovejunk" gorm:"column:onlovejunk"`
	Region      string  `json:"region"`
	Contactmail string  `json:"-"`
	Modsemail   string  `json:"modsemail"`
	Showjoin    int     `json:"showjoin"`
	Mentored    int     `json:"mentored" gorm:"column:mentored"`

	// Support-only fields (only populated when support=true and user is Admin/Support)
	Founded                *time.Time `json:"founded,omitempty" gorm:"column:founded"`
	Lastmoderated          *time.Time `json:"lastmoderated,omitempty" gorm:"column:lastmoderated"`
	Lastmodactive          *time.Time `json:"lastmodactive,omitempty" gorm:"column:lastmodactive"`
	Lastautoapprove        *time.Time `json:"lastautoapprove,omitempty" gorm:"column:lastautoapprove"`
	Activeownercount       *int       `json:"activeownercount,omitempty" gorm:"column:activeownercount"`
	Activemodcount         *int       `json:"activemodcount,omitempty" gorm:"column:activemodcount"`
	Backupmodsactive       *int       `json:"backupmodsactive,omitempty" gorm:"column:backupmodsactive"`
	Backupownersactive     *int       `json:"backupownersactive,omitempty" gorm:"column:backupownersactive"`
	Affiliationconfirmed   *time.Time `json:"affiliationconfirmed,omitempty" gorm:"column:affiliationconfirmed"`
	Affiliationconfirmedby *uint64    `json:"affiliationconfirmedby,omitempty" gorm:"column:affiliationconfirmedby"`
	Recentautoapproves     *int       `json:"recentautoapproves,omitempty" gorm:"-"`
	Recentmanualapproves   *int       `json:"recentmanualapproves,omitempty" gorm:"-"`
	Recentautoapprovespct  *float64   `json:"recentautoapprovespercent,omitempty" gorm:"-"`

	// Polygon fields (only populated when polygon=true query param)
	Poly         *string `json:"poly,omitempty" gorm:"-"`
	Polyofficial *string `json:"polyofficial,omitempty" gorm:"-"`
	Cga          *string `json:"cga,omitempty" gorm:"-"`
	Dpa          *string `json:"dpa,omitempty" gorm:"-"`
}

type RepostSettings struct {
	Offer    int `json:"offer"`
	Wanted   int `json:"wanted"`
	Max      int `json:"max"`
	Chaseups int `json:"chaseups"`
}

func GetGroup(c *fiber.Ctx) error {
	//time.Sleep(30 * time.Second)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}

	// showmods and sponsors params control whether to include those fields.
	// Default behavior (no params) loads both for backward compatibility.
	showmodsParam := c.Query("showmods")
	sponsorsParam := c.Query("sponsors")

	wantShowmods := showmodsParam != "false"
	wantSponsors := sponsorsParam != "false"
	wantFilteredSponsors := sponsorsParam == "true"

	db := database.DBConn
	var group Group
	var volunteers []GroupVolunteer
	var filteredSponsors []GroupSponsor
	found := false

	// Get group, volunteers, and sponsors info in parallel for speed.
	var wg sync.WaitGroup

	if wantShowmods {
		wg.Add(1)

		go func() {
			defer wg.Done()
			volunteers = GetGroupVolunteers(id)
		}()
	}

	if wantFilteredSponsors {
		wg.Add(1)

		go func() {
			defer wg.Done()
			db.Raw("SELECT * FROM groups_sponsorship WHERE groupid = ? AND startdate <= NOW() AND enddate >= DATE(NOW()) AND visible = 1 ORDER BY amount DESC", id).Scan(&filteredSponsors)
		}()
	}

	wg.Add(1)

	go func() {
		defer wg.Done()

		// Return the group even if publish = 0 or onhere = 0 because they have the actual id, so they must really
		// want it.  This can happen if a user has a message on a group that is then set to publish = 0, for example.
		q := db.Preload("GroupProfile")

		if !wantFilteredSponsors && wantSponsors {
			// Load all sponsors via GORM Preload (no date/visible filtering) - backward compatible default.
			q = q.Preload("GroupSponsors")
		}

		err := q.Raw("SELECT `groups`.*, CAST(JSON_EXTRACT(groups.settings, '$.showjoin') AS UNSIGNED) AS showjoin, ST_AsText(ST_ENVELOPE(polyindex)) AS bbox FROM `groups` WHERE id = ?", id).First(&group).Error
		found = !errors.Is(err, gorm.ErrRecordNotFound)

		if found {
			if group.GroupProfile.ID > 0 {
				group.GroupProfileStr = "https://" + os.Getenv("IMAGE_DOMAIN") + "/gimg_" + strconv.FormatUint(group.GroupProfile.ID, 10) + ".jpg"
			}

			if len(group.Namefull) > 0 {
				group.Namedisplay = group.Namefull
			} else {
				group.Namedisplay = group.Nameshort
			}

			if len(group.Contactmail) > 0 {
				group.Modsemail = group.Contactmail
			} else {
				group.Modsemail = group.Nameshort + "-volunteers@" + os.Getenv("GROUP_DOMAIN")
			}
		}
	}()

	wg.Wait()

	if found {
		if wantShowmods {
			group.GroupVolunteers = volunteers
		}

		if wantFilteredSponsors {
			group.GroupSponsors = filteredSponsors
		}

		// Fetch polygon data if requested.
		if c.Query("polygon") == "true" {
			type PolyResult struct {
				Poly           *string `gorm:"column:poly"`
				Polyofficial   *string `gorm:"column:polyofficial"`
				Postvisibility *string `gorm:"column:postvisibility"`
			}
			var polyResult PolyResult
			db.Raw("SELECT poly, polyofficial, ST_AsText(postvisibility) as postvisibility FROM `groups` WHERE id = ?", id).Scan(&polyResult)
			group.Poly = polyResult.Poly
			group.Polyofficial = polyResult.Polyofficial
			group.Postvisibility = polyResult.Postvisibility
			group.Cga = polyResult.Polyofficial
			group.Dpa = polyResult.Poly
		}

		// Set myrole for the current user.
		myid := user.WhoAmI(c)
		if myid > 0 {
			var myrole string
			db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = ?", myid, id, utils.COLLECTION_APPROVED).Scan(&myrole)
			if myrole != "" {
				group.Myrole = myrole
			} else {
				group.Myrole = "Non-member"
			}
		} else {
			group.Myrole = "Non-member"
		}

		// Fetch TN key if requested and user is moderator of this group.
		if c.Query("tnkey") == "true" {
			if myid > 0 && auth.IsModOfGroup(myid, id) {
				tnkey := os.Getenv("TNKEY")
				if tnkey != "" {
					var email string
					db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC, id ASC LIMIT 1", myid).Scan(&email)

					if email != "" {
						tnURL := fmt.Sprintf("https://trashnothing.com/modtools/api/group-settings-url?key=%s&moderator_email=%s&group_id=%s",
							url.QueryEscape(tnkey),
							url.QueryEscape(email),
							url.QueryEscape(group.Nameshort))

						client := &http.Client{Timeout: 10 * time.Second}
						resp, err := client.Get(tnURL)
						if err == nil {
							defer resp.Body.Close()
							body, err := io.ReadAll(resp.Body)
							if err == nil {
								var tnResult map[string]interface{}
								if json.Unmarshal(body, &tnResult) == nil {
									if v, ok := tnResult["key"].(string); ok {
										group.Tnkey = &v
									}
									if v, ok := tnResult["url"].(string); ok {
										group.Tnur = &v
									}
								}
							}
						}
					}
				}
			}
		}

		return c.JSON(group)
	} else {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}
}

func ListGroups(c *fiber.Ctx) error {
	db := database.DBConn

	support := c.Query("support") == "true"

	// Check if user is Admin or Support when support=true is requested.
	isAdminOrSupport := false
	if support {
		myid := user.WhoAmI(c)
		if myid > 0 {
			isAdminOrSupport = auth.IsAdminOrSupport(myid)
		}
	}

	var groups []GroupEntry

	if isAdminOrSupport {
		// Support mode: return all groups (not just published/onhere) with extra fields.
		db.Raw("SELECT id, nameshort, namefull, lat, lng, altlat, altlng, onmap, onhere, ontn, onlovejunk, publish, region, contactmail, mentored, "+
			"CAST(JSON_EXTRACT(groups.settings, '$.showjoin') AS UNSIGNED) AS showjoin, "+
			"founded, lastmoderated, lastmodactive, lastautoapprove, activeownercount, activemodcount, "+
			"backupmodsactive, backupownersactive, affiliationconfirmed, affiliationconfirmedby "+
			"FROM `groups` WHERE type = ?", FREEGLE).Scan(&groups)
	} else {
		db.Raw("SELECT id, nameshort, namefull, lat, lng, onmap, publish, region, contactmail, mentored, CAST(JSON_EXTRACT(groups.settings, '$.showjoin') AS UNSIGNED) AS showjoin FROM `groups` WHERE publish = 1 AND onhere = 1 AND type = ?", FREEGLE).Scan(&groups)
	}

	// For support mode, fetch recent auto-approve and manual-approve counts in parallel.
	type approveCount struct {
		Groupid uint64 `gorm:"column:groupid"`
		Count   int    `gorm:"column:count"`
	}

	var autoApproves []approveCount
	var manualApproves []approveCount

	if isAdminOrSupport {
		start := time.Now().AddDate(0, 0, -31).Format("2006-01-02")

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			db.Raw("SELECT COUNT(*) AS count, groupid FROM logs WHERE timestamp >= ? AND type = ? AND subtype = ? GROUP BY groupid",
				start, "Message", "Autoapproved").Scan(&autoApproves)
		}()

		go func() {
			defer wg.Done()
			db.Raw("SELECT COUNT(*) AS count, groupid FROM logs WHERE timestamp >= ? AND type = ? AND subtype = ? GROUP BY groupid",
				start, "Message", "Approved").Scan(&manualApproves)
		}()

		wg.Wait()

		// Build lookup maps for O(1) access.
		autoMap := make(map[uint64]int, len(autoApproves))
		for _, a := range autoApproves {
			autoMap[a.Groupid] = a.Count
		}
		manualMap := make(map[uint64]int, len(manualApproves))
		for _, a := range manualApproves {
			manualMap[a.Groupid] = a.Count
		}

		for ix := range groups {
			autoCount := autoMap[groups[ix].ID]
			// Manual approves includes auto-approves (they have both Approved and Autoapproved logs),
			// so subtract auto-approves to get the true manual count.
			manualCount := manualMap[groups[ix].ID] - autoCount
			if manualCount < 0 {
				manualCount = 0
			}

			groups[ix].Recentautoapproves = &autoCount
			groups[ix].Recentmanualapproves = &manualCount

			var pct float64
			total := autoCount + manualCount
			if total > 0 {
				pct = float64(100*autoCount) / float64(total)
			}
			groups[ix].Recentautoapprovespct = &pct
		}
	}

	// Fetch polygon data if requested.
	if c.Query("polygon") == "true" && len(groups) > 0 {
		type PolyRow struct {
			ID           uint64  `gorm:"column:id"`
			Poly         *string `gorm:"column:poly"`
			Polyofficial *string `gorm:"column:polyofficial"`
		}

		ids := make([]uint64, len(groups))
		for i, g := range groups {
			ids[i] = g.ID
		}

		var polyRows []PolyRow
		db.Raw("SELECT id, poly, polyofficial FROM `groups` WHERE id IN ?", ids).Scan(&polyRows)

		polyMap := make(map[uint64]*PolyRow, len(polyRows))
		for i := range polyRows {
			polyMap[polyRows[i].ID] = &polyRows[i]
		}

		for ix := range groups {
			if pr, ok := polyMap[groups[ix].ID]; ok {
				groups[ix].Poly = pr.Poly
				groups[ix].Polyofficial = pr.Polyofficial
				groups[ix].Cga = pr.Polyofficial
				groups[ix].Dpa = pr.Poly
			}
		}
	}

	for ix, group := range groups {
		if len(group.Namefull) > 0 {
			groups[ix].Namedisplay = group.Namefull
		} else {
			groups[ix].Namedisplay = group.Nameshort
		}

		if len(group.Contactmail) > 0 {
			groups[ix].Modsemail = group.Contactmail
		} else {
			groups[ix].Modsemail = group.Nameshort + "-volunteers@" + os.Getenv("GROUP_DOMAIN")
		}
	}

	if len(groups) == 0 {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	} else {
		return c.JSON(groups)
	}
}

// =============================================================================
// Merged from group/group_write.go
// =============================================================================

// validateGeometry checks if a WKT geometry string is valid using MySQL's ST_IsValid.
// Returns true if valid, false if invalid or unparseable.
func validateGeometry(wkt string) bool {
	db := database.DBConn

	var valid *int
	result := db.Raw("SELECT ST_IsValid(ST_GeomFromText(?))", wkt).Scan(&valid)

	if result.Error != nil || valid == nil {
		return false
	}

	return *valid == 1
}

// logGroupEdit inserts an audit log entry for group edit operations.
func logGroupEdit(groupid uint64, byuser uint64, text string) {
	db := database.DBConn
	db.Exec("INSERT INTO logs (timestamp, type, subtype, groupid, byuser, text) VALUES (NOW(), ?, ?, ?, ?, ?)",
		log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT, groupid, byuser, text)
}

type PatchGroupRequest struct {
	ID                    uint64   `json:"id"`
	Tagline               *string  `json:"tagline"`
	Namefull              *string  `json:"namefull"`
	Welcomemail           *string  `json:"welcomemail"`
	Description           *string  `json:"description"`
	Region                *string  `json:"region"`
	AffiliationConfirmed  *string  `json:"affiliationconfirmed"`
	Onhere                *int     `json:"onhere"`
	Publish               *int     `json:"publish"`
	Microvolunteering     *int     `json:"microvolunteering"`
	Mentored              *int     `json:"mentored"`
	Ontn                  *int     `json:"ontn"`
	Onlovejunk            *int              `json:"onlovejunk"`
	Settings              *json.RawMessage  `json:"settings"`
	Rules                 *json.RawMessage  `json:"rules"`
	// Admin/Support only fields
	Lat                   *float64 `json:"lat"`
	Lng                   *float64 `json:"lng"`
	Altlat                *float64 `json:"altlat"`
	Altlng                *float64 `json:"altlng"`
	Nameshort             *string  `json:"nameshort"`
	Licenserequired       *int     `json:"licenserequired"`
	Poly                  *string  `json:"poly"`
	Polyofficial          *string  `json:"polyofficial"`
	Showonyahoo           *int     `json:"showonyahoo"`
}

func PatchGroup(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	// Verify group exists
	var groupCount int64
	db.Raw("SELECT COUNT(*) FROM `groups` WHERE id = ?", req.ID).Scan(&groupCount)
	if groupCount == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}

	// Check authorization: must be mod/owner of the group OR admin/support
	if !auth.IsModOfGroup(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	isAdmin := auth.IsAdminOrSupport(myid)

	// Apply mod/owner settable fields
	if req.Tagline != nil {
		db.Exec("UPDATE `groups` SET tagline = ? WHERE id = ?", *req.Tagline, req.ID)
	}
	if req.Namefull != nil {
		db.Exec("UPDATE `groups` SET namefull = ? WHERE id = ?", *req.Namefull, req.ID)
	}
	if req.Welcomemail != nil {
		db.Exec("UPDATE `groups` SET welcomemail = ? WHERE id = ?", *req.Welcomemail, req.ID)
	}
	if req.Description != nil {
		db.Exec("UPDATE `groups` SET description = ? WHERE id = ?", *req.Description, req.ID)
	}
	if req.Region != nil {
		db.Exec("UPDATE `groups` SET region = ? WHERE id = ?", *req.Region, req.ID)
	}
	if req.AffiliationConfirmed != nil {
		db.Exec("UPDATE `groups` SET affiliationconfirmed = ?, affiliationconfirmedby = ? WHERE id = ?",
			*req.AffiliationConfirmed, myid, req.ID)
	}
	if req.Onhere != nil {
		db.Exec("UPDATE `groups` SET onhere = ? WHERE id = ?", *req.Onhere, req.ID)
	}
	if req.Publish != nil {
		db.Exec("UPDATE `groups` SET publish = ? WHERE id = ?", *req.Publish, req.ID)
	}
	if req.Microvolunteering != nil {
		db.Exec("UPDATE `groups` SET microvolunteering = ? WHERE id = ?", *req.Microvolunteering, req.ID)
	}
	if req.Mentored != nil {
		db.Exec("UPDATE `groups` SET mentored = ? WHERE id = ?", *req.Mentored, req.ID)
	}
	if req.Ontn != nil {
		db.Exec("UPDATE `groups` SET ontn = ? WHERE id = ?", *req.Ontn, req.ID)
	}
	if req.Onlovejunk != nil {
		db.Exec("UPDATE `groups` SET onlovejunk = ? WHERE id = ?", *req.Onlovejunk, req.ID)
	}
	if req.Settings != nil {
		db.Exec("UPDATE `groups` SET settings = ? WHERE id = ?", string(*req.Settings), req.ID)
		logGroupEdit(req.ID, myid, "Settings")
	}
	if req.Rules != nil {
		db.Exec("UPDATE `groups` SET rules = ? WHERE id = ?", string(*req.Rules), req.ID)
		logGroupEdit(req.ID, myid, "Rules")
	}

	// Admin/Support only fields
	if isAdmin {
		if req.Lat != nil {
			db.Exec("UPDATE `groups` SET lat = ? WHERE id = ?", *req.Lat, req.ID)
		}
		if req.Lng != nil {
			db.Exec("UPDATE `groups` SET lng = ? WHERE id = ?", *req.Lng, req.ID)
		}
		if req.Altlat != nil {
			db.Exec("UPDATE `groups` SET altlat = ? WHERE id = ?", *req.Altlat, req.ID)
		}
		if req.Altlng != nil {
			db.Exec("UPDATE `groups` SET altlng = ? WHERE id = ?", *req.Altlng, req.ID)
		}
		if req.Nameshort != nil {
			db.Exec("UPDATE `groups` SET nameshort = ? WHERE id = ?", *req.Nameshort, req.ID)
		}
		if req.Licenserequired != nil {
			db.Exec("UPDATE `groups` SET licenserequired = ? WHERE id = ?", *req.Licenserequired, req.ID)
		}
		if req.Poly != nil {
			if !validateGeometry(*req.Poly) {
				return fiber.NewError(fiber.StatusBadRequest, "Invalid poly geometry")
			}
			db.Exec("UPDATE `groups` SET poly = ? WHERE id = ?", *req.Poly, req.ID)
		}
		if req.Polyofficial != nil {
			if !validateGeometry(*req.Polyofficial) {
				return fiber.NewError(fiber.StatusBadRequest, "Invalid polyofficial geometry")
			}
			db.Exec("UPDATE `groups` SET polyofficial = ? WHERE id = ?", *req.Polyofficial, req.ID)
		}
		if req.Showonyahoo != nil {
			db.Exec("UPDATE `groups` SET showonyahoo = ? WHERE id = ?", *req.Showonyahoo, req.ID)
		}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

type CreateGroupRequest struct {
	Name      string   `json:"name"`
	GroupType string   `json:"grouptype"`
	Lat       *float64 `json:"lat,omitempty"`
	Lng       *float64 `json:"lng,omitempty"`
}

// CreateGroup creates a new group. Requires moderator/owner on any group, or admin/support.
// @Summary Create a new group
// @Tags group
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /group [post]
func CreateGroup(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req CreateGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}

	if req.GroupType == "" {
		req.GroupType = "Freegle"
	}

	db := database.DBConn

	// Check authorization: admin/support OR moderator/owner of any group.
	isAdmin := auth.IsAdminOrSupport(myid)

	if !isAdmin {
		var modCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN (?, ?)", myid, utils.ROLE_OWNER, utils.ROLE_MODERATOR).Scan(&modCount)
		if modCount == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Must be a moderator to create groups")
		}
	}

	// Use the underlying sql.DB to get LastInsertId() directly from the MySQL protocol
	// response — never issue a separate SELECT LAST_INSERT_ID() as it's unsafe under
	// parallel load (GORM's connection pool may assign a different connection).
	sqlDB, err := db.DB()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	sqlResult, err := sqlDB.Exec("INSERT INTO `groups` (nameshort, namedisplay, type, region, publish, onhere) VALUES (?, ?, ?, 'UK', 1, 1)",
		req.Name, req.Name, req.GroupType)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create group")
	}

	var newID uint64
	lastID, err := sqlResult.LastInsertId()
	if err == nil && lastID > 0 {
		newID = uint64(lastID)
	}

	// Admin/support can set lat/lng.
	if isAdmin {
		if req.Lat != nil {
			db.Exec("UPDATE `groups` SET lat = ? WHERE id = ?", *req.Lat, newID)
		}
		if req.Lng != nil {
			db.Exec("UPDATE `groups` SET lng = ? WHERE id = ?", *req.Lng, newID)
		}
	}

	// Creator becomes Owner.
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, ?, ?)", myid, newID, utils.ROLE_OWNER, utils.COLLECTION_APPROVED)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

