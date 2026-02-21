package admin

import (
	"strconv"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

type Admin struct {
	ID        uint64     `json:"id"`
	Createdby *uint64    `json:"createdby"`
	Groupid   *uint64    `json:"groupid"`
	Subject   *string    `json:"subject"`
	Text      *string    `json:"text"`
	Created   *time.Time `json:"created"`
	Complete  *time.Time `json:"complete"`
	Heldby    *uint64    `json:"heldby"`
	Pending   bool       `json:"pending"`
}


// GetAdmin handles GET /admin/:id - get a single admin by ID.
func GetAdmin(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid admin ID")
	}

	if !user.IsModOfAnyGroup(myid) && !user.IsAdminOrSupport(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Must be a moderator")
	}

	db := database.DBConn
	var admin Admin
	db.Raw("SELECT id, createdby, groupid, subject, text, created, complete, heldby, pending FROM admins WHERE id = ?", id).Scan(&admin)

	if admin.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Admin not found")
	}

	return c.JSON(admin)
}

// ListAdmins handles GET /admin - list admins for groups the user moderates.
func ListAdmins(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	groupidParam, _ := strconv.ParseUint(c.Query("groupid", "0"), 10, 64)
	pendingParam := c.Query("pending", "")

	// Build query: admins for groups the user moderates, not yet complete.
	query := "SELECT a.id, a.createdby, a.groupid, a.subject, a.text, a.created, a.complete, a.heldby, a.pending " +
		"FROM admins a INNER JOIN memberships m ON m.groupid = a.groupid AND m.userid = ? AND m.role IN ('Owner','Moderator') " +
		"WHERE a.complete IS NULL"
	args := []interface{}{myid}

	if groupidParam > 0 {
		query += " AND a.groupid = ?"
		args = append(args, groupidParam)
	}

	if pendingParam == "true" {
		query += " AND a.pending = 1"
	} else if pendingParam == "false" {
		query += " AND a.pending = 0"
	}

	query += " ORDER BY a.id DESC"

	var admins []Admin
	db.Raw(query, args...).Scan(&admins)

	if admins == nil {
		admins = make([]Admin, 0)
	}

	return c.JSON(admins)
}

type PostAdminRequest struct {
	ID      uint64 `json:"id"`
	Action  string `json:"action"`
	GroupID uint64 `json:"groupid"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

// PostAdmin handles POST /admin - action-based handler for Create, Hold, Release.
func PostAdmin(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostAdminRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	switch req.Action {
	case "Hold":
		if req.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "id is required")
		}

		// Check mod of the admin's group.
		var adminGroupID uint64
		db.Raw("SELECT COALESCE(groupid, 0) FROM admins WHERE id = ?", req.ID).Scan(&adminGroupID)

		if !user.IsModOfGroup(myid, adminGroupID) {
			return fiber.NewError(fiber.StatusForbidden, "Must be a moderator of the admin's group")
		}

		db.Exec("UPDATE admins SET heldby = ? WHERE id = ?", myid, req.ID)
		return c.JSON(fiber.Map{"success": true})

	case "Release":
		if req.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "id is required")
		}

		var adminGroupID uint64
		db.Raw("SELECT COALESCE(groupid, 0) FROM admins WHERE id = ?", req.ID).Scan(&adminGroupID)

		if !user.IsModOfGroup(myid, adminGroupID) {
			return fiber.NewError(fiber.StatusForbidden, "Must be a moderator of the admin's group")
		}

		db.Exec("UPDATE admins SET heldby = NULL WHERE id = ?", req.ID)
		return c.JSON(fiber.Map{"success": true})

	default:
		// Create new admin.
		if req.GroupID == 0 && !user.IsAdminOrSupport(myid) {
			return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
		}

		if req.GroupID > 0 && !user.IsModOfGroup(myid, req.GroupID) {
			return fiber.NewError(fiber.StatusForbidden, "Must be a moderator of the group")
		}

		if req.Subject == "" {
			return fiber.NewError(fiber.StatusBadRequest, "subject is required")
		}

		result := db.Exec("INSERT INTO admins (createdby, groupid, subject, text, created) VALUES (?, ?, ?, ?, NOW())",
			myid, utils.NilIfZero(req.GroupID), req.Subject, req.Text)

		if result.Error != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create admin")
		}

		var id uint64
		db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

		return c.JSON(fiber.Map{"id": id})
	}
}

type PatchAdminRequest struct {
	ID       uint64  `json:"id"`
	Subject  *string `json:"subject,omitempty"`
	Text     *string `json:"text,omitempty"`
	Complete *string `json:"complete,omitempty"`
	Pending  *bool   `json:"pending,omitempty"`
}

// PatchAdmin handles PATCH /admin - update an admin.
func PatchAdmin(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchAdminRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	var adminGroupID uint64
	db.Raw("SELECT COALESCE(groupid, 0) FROM admins WHERE id = ?", req.ID).Scan(&adminGroupID)

	if !user.IsModOfGroup(myid, adminGroupID) {
		return fiber.NewError(fiber.StatusForbidden, "Must be a moderator of the admin's group")
	}

	if req.Subject != nil {
		db.Exec("UPDATE admins SET subject = ? WHERE id = ?", *req.Subject, req.ID)
	}
	if req.Text != nil {
		db.Exec("UPDATE admins SET text = ? WHERE id = ?", *req.Text, req.ID)
	}
	if req.Complete != nil {
		db.Exec("UPDATE admins SET complete = NOW() WHERE id = ?", req.ID)
	}
	if req.Pending != nil {
		var val int
		if *req.Pending {
			val = 1
		}
		db.Exec("UPDATE admins SET pending = ? WHERE id = ?", val, req.ID)
	}

	return c.JSON(fiber.Map{"success": true})
}

type DeleteAdminRequest struct {
	ID uint64 `json:"id"`
}

// DeleteAdmin handles DELETE /admin - delete an admin.
func DeleteAdmin(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Support both body and query parameter for ID.
	var id uint64
	var req DeleteAdminRequest
	if err := c.BodyParser(&req); err == nil && req.ID > 0 {
		id = req.ID
	} else {
		id, _ = strconv.ParseUint(c.Query("id", "0"), 10, 64)
	}

	if id == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	var adminGroupID uint64
	db.Raw("SELECT COALESCE(groupid, 0) FROM admins WHERE id = ?", id).Scan(&adminGroupID)

	if !user.IsModOfGroup(myid, adminGroupID) {
		return fiber.NewError(fiber.StatusForbidden, "Must be a moderator of the admin's group")
	}

	db.Exec("DELETE FROM admins WHERE id = ?", id)

	return c.JSON(fiber.Map{"success": true})
}
