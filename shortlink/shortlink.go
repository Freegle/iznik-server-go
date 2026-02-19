package shortlink

import (
	"os"
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

type Shortlink struct {
	ID        uint64  `json:"id" gorm:"primary_key"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Groupid   *uint64 `json:"groupid"`
	Url       *string `json:"url"`
	Clicks    int64   `json:"clicks"`
	Created   string  `json:"created"`
	Nameshort string  `json:"nameshort,omitempty" gorm:"-"`
}

type ClickHistory struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// GetShortlink handles GET /shortlink with optional id and groupid parameters.
//
// @Summary Get shortlinks
// @Description Returns a single shortlink by ID, or lists all shortlinks (optionally filtered by group)
// @Tags shortlink
// @Produce json
// @Param id query integer false "Shortlink ID"
// @Param groupid query integer false "Filter by group ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/shortlink [get]
func GetShortlink(c *fiber.Ctx) error {
	db := database.DBConn
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	groupid, _ := strconv.ParseUint(c.Query("groupid", "0"), 10, 64)

	userSite := os.Getenv("USER_SITE")
	if userSite == "" {
		userSite = "www.ilovefreegle.org"
	}

	if id > 0 {
		// Single shortlink with click history.
		var s Shortlink
		db.Raw("SELECT * FROM shortlinks WHERE id = ?", id).Scan(&s)

		if s.ID == 0 {
			return c.JSON(fiber.Map{"ret": 2, "status": "Not found"})
		}

		resolveShortlinkURL(&s, userSite)

		// Get click history.
		var clicks []ClickHistory
		db.Raw("SELECT DATE(timestamp) AS date, COUNT(*) AS count FROM shortlink_clicks WHERE shortlinkid = ? GROUP BY date ORDER BY date ASC", id).Scan(&clicks)

		return c.JSON(fiber.Map{
			"ret":    0,
			"status": "Success",
			"shortlink": fiber.Map{
				"id":           s.ID,
				"name":         s.Name,
				"type":         s.Type,
				"groupid":      s.Groupid,
				"url":          s.Url,
				"clicks":       s.Clicks,
				"created":      s.Created,
				"nameshort":    s.Nameshort,
				"clickhistory": clicks,
			},
		})
	}

	// List all shortlinks.
	var links []Shortlink
	if groupid > 0 {
		db.Raw("SELECT * FROM shortlinks WHERE groupid = ? ORDER BY LOWER(name) ASC", groupid).Scan(&links)
	} else {
		db.Raw("SELECT * FROM shortlinks ORDER BY LOWER(name) ASC").Scan(&links)
	}

	for i := range links {
		resolveShortlinkURL(&links[i], userSite)
	}

	return c.JSON(fiber.Map{
		"ret":        0,
		"status":     "Success",
		"shortlinks": links,
	})
}

// PostShortlink handles POST /shortlink to create a new shortlink.
//
// @Summary Create a shortlink
// @Tags shortlink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/shortlink [post]
func PostShortlink(c *fiber.Ctx) error {
	db := database.DBConn

	type CreateRequest struct {
		Name    string `json:"name"`
		Groupid uint64 `json:"groupid"`
	}

	var req CreateRequest

	// Support both form and JSON.
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.Name == "" {
		req.Name = c.FormValue("name", c.Query("name", ""))
	}
	if req.Groupid == 0 {
		req.Groupid, _ = strconv.ParseUint(c.FormValue("groupid", c.Query("groupid", "0")), 10, 64)
	}

	if req.Name == "" || req.Groupid == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid parameters"})
	}

	// Check if name already exists.
	var existing uint64
	db.Raw("SELECT id FROM shortlinks WHERE name LIKE ?", req.Name).Scan(&existing)
	if existing > 0 {
		return c.JSON(fiber.Map{"ret": 3, "status": "Name already in use"})
	}

	// Create the shortlink.
	result := db.Exec("INSERT INTO shortlinks (name, type, groupid) VALUES (?, 'Group', ?)", req.Name, req.Groupid)
	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 1, "status": "Failed to create shortlink"})
	}

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"id":     newID,
	})
}

// resolveShortlinkURL computes the URL for a Group-type shortlink based on group settings.
func resolveShortlinkURL(s *Shortlink, userSite string) {
	if s.Type == "Group" && s.Groupid != nil {
		var g struct {
			Nameshort string
			External  *string
			Onhere    int
		}
		database.DBConn.Raw("SELECT nameshort, external, onhere FROM `groups` WHERE id = ?", *s.Groupid).Scan(&g)

		s.Nameshort = g.Nameshort

		if g.External != nil && *g.External != "" {
			s.Url = g.External
		} else if g.Onhere > 0 {
			url := "https://" + userSite + "/explore/" + g.Nameshort
			s.Url = &url
		} else {
			url := "https://groups.yahoo.com/neo/groups/" + g.Nameshort
			s.Url = &url
		}
	}
}
