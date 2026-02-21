package group

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

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
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	isAdmin := systemrole == utils.SYSTEMROLE_SUPPORT || systemrole == utils.SYSTEMROLE_ADMIN

	isModOrOwner := false
	if !isAdmin {
		var memberRole string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", myid, req.ID).Scan(&memberRole)
		isModOrOwner = memberRole == utils.ROLE_MODERATOR || memberRole == utils.ROLE_OWNER
	}

	if !isAdmin && !isModOrOwner {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

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
	}
	if req.Rules != nil {
		db.Exec("UPDATE `groups` SET rules = ? WHERE id = ?", string(*req.Rules), req.ID)
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
			db.Exec("UPDATE `groups` SET poly = ? WHERE id = ?", *req.Poly, req.ID)
		}
		if req.Polyofficial != nil {
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
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	isAdmin := systemrole == utils.SYSTEMROLE_SUPPORT || systemrole == utils.SYSTEMROLE_ADMIN

	if !isAdmin {
		var modCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Owner', 'Moderator')", myid).Scan(&modCount)
		if modCount == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Must be a moderator to create groups")
		}
	}

	result := db.Exec("INSERT INTO `groups` (nameshort, namedisplay, type, region, publish, onhere) VALUES (?, ?, ?, 'UK', 1, 1)",
		req.Name, req.Name, req.GroupType)
	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create group")
	}

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

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
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Owner', 'Approved')", myid, newID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// RemoveFacebook removes the Facebook page link from a group (mod/owner or admin/support only).
func RemoveFacebook(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type RemoveFBRequest struct {
		ID  uint64 `json:"id"`
		UID string `json:"uid"`
	}

	var req RemoveFBRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Group ID required")
	}

	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	isAdmin := systemrole == utils.SYSTEMROLE_ADMIN || systemrole == utils.SYSTEMROLE_SUPPORT

	if !isAdmin {
		var role string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", myid, req.ID).Scan(&role)
		if role != utils.ROLE_MODERATOR && role != utils.ROLE_OWNER {
			return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
		}
	}

	db.Exec("UPDATE `groups` SET facebookid = NULL WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
