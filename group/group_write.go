package group

import (
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
	Onlovejunk            *int     `json:"onlovejunk"`
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
