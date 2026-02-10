package team

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type Team struct {
	ID           uint64  `json:"id" gorm:"primary_key"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	Type         string  `json:"type"`
	Email        *string `json:"email"`
	Active       int     `json:"active"`
	Wikiurl      *string `json:"wikiurl"`
	Supporttools int     `json:"supporttools"`
}

type TeamMember struct {
	Userid        uint64  `json:"userid"`
	Description   *string `json:"description"`
	Added         string  `json:"added"`
	Nameoverride  *string `json:"nameoverride"`
	Imageoverride *string `json:"imageoverride"`
}

// hasTeamsPermission checks if user is Admin/Support (simplified PERM_TEAMS check).
func hasTeamsPermission(myid uint64) bool {
	var role string
	database.DBConn.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
	return role == "Admin" || role == "Support"
}

// GetTeam handles GET /team - list all, single by id, or Volunteers pseudo-team.
//
// @Summary Get teams
// @Tags team
// @Produce json
// @Param id query integer false "Team ID"
// @Param name query string false "Team name (use 'Volunteers' for pseudo-team)"
// @Success 200 {object} map[string]interface{}
// @Router /api/team [get]
func GetTeam(c *fiber.Ctx) error {
	db := database.DBConn
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	name := c.Query("name", "")

	// Volunteers pseudo-team.
	if name == "Volunteers" {
		return getVolunteers(c)
	}

	// Get by name.
	if name != "" {
		db.Raw("SELECT id FROM teams WHERE name LIKE ?", name).Scan(&id)
		if id == 0 {
			return c.JSON(fiber.Map{"ret": 2, "status": "Not found"})
		}
	}

	if id > 0 {
		// Single team with members.
		var t Team
		db.Raw("SELECT * FROM teams WHERE id = ?", id).Scan(&t)
		if t.ID == 0 {
			return c.JSON(fiber.Map{"ret": 2, "status": "Not found"})
		}

		var members []TeamMember
		db.Raw("SELECT userid, description, added, nameoverride, imageoverride "+
			"FROM teams_members WHERE teamid = ?", id).Scan(&members)

		memberList := make([]map[string]interface{}, len(members))
		for i, m := range members {
			entry := map[string]interface{}{
				"id":          m.Userid,
				"description": m.Description,
				"added":       m.Added,
			}

			// Get display name (nameoverride takes precedence).
			if m.Nameoverride != nil && *m.Nameoverride != "" {
				entry["displayname"] = *m.Nameoverride
			} else {
				var displayname string
				db.Raw("SELECT COALESCE(fullname, CONCAT(COALESCE(firstname,''), ' ', COALESCE(lastname,'')), 'Unknown') FROM users WHERE id = ?",
					m.Userid).Scan(&displayname)
				entry["displayname"] = strings.TrimSpace(displayname)
			}

			// Get profile image.
			entry["profile"] = getUserProfile(m.Userid, m.Imageoverride)

			memberList[i] = entry
		}

		return c.JSON(fiber.Map{
			"ret":    0,
			"status": "Success",
			"team": fiber.Map{
				"id":           t.ID,
				"name":         t.Name,
				"description":  t.Description,
				"type":         t.Type,
				"email":        t.Email,
				"active":       t.Active,
				"wikiurl":      t.Wikiurl,
				"supporttools": t.Supporttools,
				"members":      memberList,
			},
		})
	}

	// List all teams.
	var teams []Team
	db.Raw("SELECT * FROM teams ORDER BY LOWER(name) ASC").Scan(&teams)

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"teams":  teams,
	})
}

// getVolunteers returns the pseudo-team of all moderators who opt-in.
func getVolunteers(c *fiber.Ctx) error {
	db := database.DBConn

	type VolRow struct {
		Userid    uint64
		Firstname *string
		Lastname  *string
		Fullname  *string
		Added     string
		Settings  *string
	}

	var vols []VolRow
	db.Raw("SELECT DISTINCT memberships.userid, users.firstname, users.lastname, users.fullname, "+
		"users.added, users.settings "+
		"FROM memberships "+
		"INNER JOIN `groups` ON `groups`.id = memberships.groupid "+
		"AND memberships.role IN ('Moderator', 'Owner') "+
		"INNER JOIN users ON users.id = memberships.userid "+
		"WHERE `groups`.type = 'Freegle'").Scan(&vols)

	members := []map[string]interface{}{}
	for _, v := range vols {
		// Check settings for showmod and useprofile.
		if v.Settings == nil {
			continue
		}
		settings := *v.Settings
		// Simple JSON field check - showmod must be true.
		if !strings.Contains(settings, `"showmod":true`) && !strings.Contains(settings, `"showmod": true`) {
			continue
		}

		displayname := ""
		if v.Fullname != nil && *v.Fullname != "" {
			displayname = *v.Fullname
		} else {
			parts := []string{}
			if v.Firstname != nil && *v.Firstname != "" {
				parts = append(parts, *v.Firstname)
			}
			if v.Lastname != nil && *v.Lastname != "" {
				parts = append(parts, *v.Lastname)
			}
			displayname = strings.Join(parts, " ")
		}

		if displayname == "" {
			displayname = "Unknown"
		}

		members = append(members, map[string]interface{}{
			"userid":      v.Userid,
			"added":       v.Added,
			"displayname": displayname,
			"profile":     getUserProfile(v.Userid, nil),
		})
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"team": fiber.Map{
			"name":    "Volunteers",
			"members": members,
		},
	})
}

// getUserProfile gets the profile image for a user.
func getUserProfile(userid uint64, imageOverride *string) map[string]interface{} {
	if imageOverride != nil && *imageOverride != "" {
		return map[string]interface{}{
			"url":     *imageOverride,
			"turl":    *imageOverride,
			"default": false,
		}
	}

	imageDomain := os.Getenv("IMAGE_DOMAIN")
	if imageDomain == "" {
		imageDomain = "images.ilovefreegle.org"
	}

	db := database.DBConn
	var imgID uint64
	db.Raw("SELECT id FROM users_images WHERE userid = ? ORDER BY id DESC LIMIT 1", userid).Scan(&imgID)

	if imgID > 0 {
		return map[string]interface{}{
			"url":     fmt.Sprintf("https://%s/uimg_%d.jpg", imageDomain, imgID),
			"turl":    fmt.Sprintf("https://%s/tuimg_%d.jpg", imageDomain, imgID),
			"default": false,
		}
	}

	return map[string]interface{}{
		"url":     "https://www.gravatar.com/avatar/?s=200",
		"turl":    "https://www.gravatar.com/avatar/?s=100",
		"default": true,
	}
}

// PostTeam handles POST /team to create a new team.
//
// @Summary Create team
// @Tags team
// @Accept json
// @Produce json
// @Router /api/team [post]
func PostTeam(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	if !hasTeamsPermission(myid) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	type CreateRequest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Email       string `json:"email"`
	}

	var req CreateRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.Name == "" {
		req.Name = c.FormValue("name", c.Query("name", ""))
	}

	if req.Name == "" {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing name"})
	}

	db := database.DBConn
	result := db.Exec("INSERT INTO teams (name, email, description) VALUES (?, ?, ?)",
		req.Name, req.Email, req.Description)
	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 1, "status": "Create failed"})
	}

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchTeam handles PATCH /team to update team or manage members.
//
// @Summary Update team or manage members
// @Tags team
// @Accept json
// @Produce json
// @Router /api/team [patch]
func PatchTeam(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	if !hasTeamsPermission(myid) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	type PatchRequest struct {
		ID          uint64 `json:"id"`
		Action      string `json:"action"`
		Userid      uint64 `json:"userid"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Email       string `json:"email"`
		Wikiurl     string `json:"wikiurl"`
	}

	var req PatchRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}

	if req.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing id"})
	}

	db := database.DBConn

	switch req.Action {
	case "Add":
		if req.Userid == 0 {
			return c.JSON(fiber.Map{"ret": 2, "status": "Missing userid"})
		}
		db.Exec("REPLACE INTO teams_members (userid, teamid, description) VALUES (?, ?, ?)",
			req.Userid, req.ID, req.Description)
	case "Remove":
		if req.Userid == 0 {
			return c.JSON(fiber.Map{"ret": 2, "status": "Missing userid"})
		}
		db.Exec("DELETE FROM teams_members WHERE userid = ? AND teamid = ?",
			req.Userid, req.ID)
	default:
		// Update team attributes.
		if req.Name != "" {
			db.Exec("UPDATE teams SET name = ? WHERE id = ?", req.Name, req.ID)
		}
		if req.Description != "" {
			db.Exec("UPDATE teams SET description = ? WHERE id = ?", req.Description, req.ID)
		}
		if req.Email != "" {
			db.Exec("UPDATE teams SET email = ? WHERE id = ?", req.Email, req.ID)
		}
		if req.Wikiurl != "" {
			db.Exec("UPDATE teams SET wikiurl = ? WHERE id = ?", req.Wikiurl, req.ID)
		}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteTeam handles DELETE /team.
//
// @Summary Delete team
// @Tags team
// @Produce json
// @Param id query integer true "Team ID"
// @Router /api/team [delete]
func DeleteTeam(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	if !hasTeamsPermission(myid) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing id"})
	}

	db := database.DBConn
	db.Exec("DELETE FROM teams WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
