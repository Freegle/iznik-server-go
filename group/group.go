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

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const MODERATOR = "Moderator"
const OWNER = "Owner"
const FREEGLE = "Freegle"

// Full group details.
type Group struct {
	ID                   uint64           `json:"id" gorm:"primary_key"`
	Nameshort            string           `json:"nameshort"`
	Namefull             string           `json:"namefull"`
	Namedisplay          string           `json:"namedisplay"`
	Settings             json.RawMessage  `json:"settings"` // This is JSON stored in the DB as a string.
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
	Showjoin             int              `json:"showjoin"`

	// Polygon fields (only populated when polygon=true query param)
	Poly           *string `json:"poly,omitempty" gorm:"-"`
	Polyofficial   *string `json:"polyofficial,omitempty" gorm:"-"`
	Postvisibility *string `json:"postvisibility,omitempty" gorm:"-"`

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
	Region      string  `json:"region"`
	Contactmail string  `json:"-"`
	Modsemail   string  `json:"modsemail"`
	Showjoin    int     `json:"showjoin"`

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

		err := q.Raw("SELECT `groups`.*, CAST(JSON_EXTRACT(groups.settings, '$.showjoin') AS UNSIGNED) AS showjoin FROM `groups` WHERE id = ? AND type = ?", id, FREEGLE).First(&group).Error
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
		}

		// Fetch TN key if requested and user is moderator of this group.
		if c.Query("tnkey") == "true" {
			myid := user.WhoAmI(c)
			if myid > 0 {
				var role string
				db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND role IN ('Moderator', 'Owner')", myid, id).Scan(&role)

				if role != "" {
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
			var systemrole string
			db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
			isAdminOrSupport = systemrole == utils.SYSTEMROLE_SUPPORT || systemrole == utils.SYSTEMROLE_ADMIN
		}
	}

	var groups []GroupEntry

	if isAdminOrSupport {
		// Support mode: return all groups (not just published/onhere) with extra fields.
		db.Raw("SELECT id, nameshort, namefull, lat, lng, altlat, altlng, onmap, publish, region, contactmail, "+
			"CAST(JSON_EXTRACT(groups.settings, '$.showjoin') AS UNSIGNED) AS showjoin, "+
			"founded, lastmoderated, lastmodactive, lastautoapprove, activeownercount, activemodcount, "+
			"backupmodsactive, backupownersactive, affiliationconfirmed, affiliationconfirmedby "+
			"FROM `groups` WHERE type = ?", FREEGLE).Scan(&groups)
	} else {
		db.Raw("SELECT id, nameshort, namefull, lat, lng, onmap, publish, region, contactmail, CAST(JSON_EXTRACT(groups.settings, '$.showjoin') AS UNSIGNED) AS showjoin FROM `groups` WHERE publish = 1 AND onhere = 1 AND type = ?", FREEGLE).Scan(&groups)
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
