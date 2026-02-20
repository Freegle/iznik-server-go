package location

import (
	"fmt"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// isSystemMod checks if the user has system-level Moderator, Support, or Admin role.
func isSystemMod(myid uint64) bool {
	db := database.DBConn
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	return systemrole == "Moderator" || systemrole == "Support" || systemrole == "Admin"
}

// isGroupMod checks if the user is a Moderator or Owner of the given group.
func isGroupMod(myid uint64, groupid uint64) bool {
	db := database.DBConn
	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", myid, groupid).Scan(&role)
	return role == "Moderator" || role == "Owner"
}

type CreateLocationRequest struct {
	Name    string `json:"name"`
	Polygon string `json:"polygon"`
}

// CreateLocation handles PUT /locations - create a new location (system mod/admin only).
func CreateLocation(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isSystemMod(myid) {
		return fiber.NewError(fiber.StatusForbidden, "System moderator or admin role required")
	}

	var req CreateLocationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Name == "" || req.Polygon == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name and polygon are required")
	}

	canon := strings.ToLower(req.Name)

	db := database.DBConn
	result := db.Exec(
		fmt.Sprintf("INSERT INTO locations (name, type, geometry, canon, popularity) VALUES (?, 'Polygon', ST_GeomFromText(?, %d), ?, 0)", utils.SRID),
		req.Name, req.Polygon, canon,
	)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create location")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return c.JSON(fiber.Map{"id": id})
}

type UpdateLocationRequest struct {
	ID      uint64  `json:"id"`
	Name    *string `json:"name,omitempty"`
	Polygon *string `json:"polygon,omitempty"`
}

// UpdateLocation handles PATCH /locations - update a location (system mod/admin only).
func UpdateLocation(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isSystemMod(myid) {
		return fiber.NewError(fiber.StatusForbidden, "System moderator or admin role required")
	}

	var req UpdateLocationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	if req.Polygon != nil && *req.Polygon != "" {
		db.Exec(
			fmt.Sprintf("UPDATE locations SET geometry = ST_GeomFromText(?, %d) WHERE id = ?", utils.SRID),
			*req.Polygon, req.ID,
		)
	}

	if req.Name != nil && *req.Name != "" {
		canon := strings.ToLower(*req.Name)
		db.Exec("UPDATE locations SET name = ?, canon = ? WHERE id = ?", *req.Name, canon, req.ID)
	}

	return c.JSON(fiber.Map{"success": true})
}

type ExcludeLocationRequest struct {
	ID        uint64 `json:"id"`
	GroupID   uint64 `json:"groupid"`
	Action    string `json:"action"`
	Byname    bool   `json:"byname"`
	MessageID uint64 `json:"messageid"`
}

// ExcludeLocation handles POST /locations with action=Exclude - exclude a location from a group (group mod only).
func ExcludeLocation(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req ExcludeLocationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Action != "Exclude" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid action")
	}

	if req.ID == 0 || req.GroupID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id and groupid are required")
	}

	if !isGroupMod(myid, req.GroupID) {
		return fiber.NewError(fiber.StatusForbidden, "Must be a moderator or owner of the group")
	}

	db := database.DBConn

	// Exclude the specified location.
	db.Exec("INSERT IGNORE INTO locations_excluded (locationid, groupid, userid) VALUES (?, ?, ?)",
		req.ID, req.GroupID, myid)

	// If byname, also exclude all locations with the same name.
	if req.Byname {
		var name string
		db.Raw("SELECT name FROM locations WHERE id = ?", req.ID).Scan(&name)
		if name != "" {
			var otherIDs []uint64
			db.Raw("SELECT id FROM locations WHERE name = ? AND id != ?", name, req.ID).Pluck("id", &otherIDs)
			for _, otherID := range otherIDs {
				db.Exec("INSERT IGNORE INTO locations_excluded (locationid, groupid, userid) VALUES (?, ?, ?)",
					otherID, req.GroupID, myid)
			}
		}
	}

	return c.JSON(fiber.Map{"success": true})
}
