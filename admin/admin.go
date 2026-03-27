package admin

import (
	"strconv"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

type Admin struct {
	ID            uint64     `json:"id"`
	Createdby     *uint64    `json:"createdby"`
	Groupid       *uint64    `json:"groupid"`
	Subject       *string    `json:"subject"`
	Text          *string    `json:"text"`
	CTA_Text      *string    `json:"ctatext"`
	CTA_Link      *string    `json:"ctalink"`
	Created       *time.Time `json:"created"`
	Complete      *time.Time `json:"complete"`
	Heldby        *uint64    `json:"heldby"`
	Pending       bool       `json:"pending"`
	Essential     bool       `json:"essential"`
	Template      *string    `json:"template"`
	Editprotected bool       `json:"editprotected"`
}


// GetAdmin handles GET /admin/:id - get a single admin by ID.
//
// @Summary Get a specific admin message
// @Tags admin
// @Produce json
// @Param id path integer true "Admin ID"
// @Success 200 {object} map[string]interface{}
// @Router /modtools/admin/{id} [get]
func GetAdmin(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid admin ID")
	}

	if !auth.IsSystemMod(myid) && !user.IsAdminOrSupport(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Must be a moderator")
	}

	db := database.DBConn
	var admin Admin
	db.Raw("SELECT id, createdby, groupid, subject, text, ctatext, ctalink, created, complete, heldby, pending, essential, template, editprotected FROM admins WHERE id = ?", id).Scan(&admin)

	if admin.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Admin not found")
	}

	return c.JSON(admin)
}

// ListAdmins handles GET /admin - list admins for groups the user moderates.
//
// @Summary List admin messages
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /modtools/admin [get]
func ListAdmins(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	groupidParam, _ := strconv.ParseUint(c.Query("groupid", "0"), 10, 64)
	pendingParam := c.Query("pending", "")

	// Build query: admins for groups the user moderates, not yet complete.
	// System Admin/Support users can see all admins.
	var query string
	var args []interface{}

	if auth.IsAdminOrSupport(myid) {
		query = "SELECT a.id, a.createdby, a.groupid, a.subject, a.text, a.ctatext, a.ctalink, a.created, a.complete, a.heldby, a.pending, a.essential, a.template, a.editprotected " +
			"FROM admins a WHERE a.complete IS NULL"
	} else {
		// V1 parity: filter by active mod groups (checks settings.active flag),
		// not just role. This prevents showing admins for groups the mod has
		// stepped back from.
		activeGroupIDs := user.GetActiveModGroupIDs(myid)
		if len(activeGroupIDs) == 0 {
			return c.JSON(make([]Admin, 0))
		}
		query = "SELECT a.id, a.createdby, a.groupid, a.subject, a.text, a.ctatext, a.ctalink, a.created, a.complete, a.heldby, a.pending, a.essential, a.template, a.editprotected " +
			"FROM admins a WHERE a.complete IS NULL AND a.groupid IN (?)"
		args = append(args, activeGroupIDs)
	}

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
	ID            uint64  `json:"id"`
	Action        string  `json:"action"`
	GroupID       uint64  `json:"groupid"`
	Subject       string  `json:"subject"`
	Text          string  `json:"text"`
	CTA_Text      *string `json:"ctatext,omitempty"`
	CTA_Link      *string `json:"ctalink,omitempty"`
	Essential     *bool   `json:"essential,omitempty"`
	Template      *string `json:"template,omitempty"`
	Editprotected *bool   `json:"editprotected,omitempty"`
}

// PostAdmin handles POST /admin - action-based handler for Create, Hold, Release.
//
// @Summary Create an admin message
// @Tags admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /modtools/admin [post]
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

		essential := true
		if req.Essential != nil {
			essential = *req.Essential
		}

		template := ""
		if req.Template != nil {
			template = *req.Template
		}

		// Use the underlying sql.DB to get LastInsertId() directly from the MySQL protocol
		// response — never issue a separate SELECT LAST_INSERT_ID() as it's unsafe under
		// parallel load (GORM's connection pool may assign a different connection).
		sqlDB, err := db.DB()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Database error")
		}
		sqlResult, err := sqlDB.Exec("INSERT INTO admins (createdby, groupid, subject, text, ctatext, ctalink, essential, template, editprotected, created) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())",
			myid, utils.NilIfZero(req.GroupID), req.Subject, req.Text, req.CTA_Text, req.CTA_Link, essential, template, req.Editprotected != nil && *req.Editprotected)

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create admin")
		}

		var id uint64
		lastID, err := sqlResult.LastInsertId()
		if err == nil && lastID > 0 {
			id = uint64(lastID)
		}

		return c.JSON(fiber.Map{"id": id})
	}
}

type PatchAdminRequest struct {
	ID            uint64  `json:"id"`
	Subject       *string `json:"subject,omitempty"`
	Text          *string `json:"text,omitempty"`
	Complete      *string `json:"complete,omitempty"`
	Pending       *bool   `json:"pending,omitempty"`
	CTA_Text      *string `json:"ctatext,omitempty"`
	CTA_Link      *string `json:"ctalink,omitempty"`
	Essential     *bool   `json:"essential,omitempty"`
	Template      *string `json:"template,omitempty"`
	Editprotected *bool   `json:"editprotected,omitempty"`
}

// PatchAdmin handles PATCH /admin - update an admin.
//
// @Summary Update an admin message
// @Tags admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /modtools/admin [patch]
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
	if req.CTA_Text != nil {
		db.Exec("UPDATE admins SET ctatext = ? WHERE id = ?", *req.CTA_Text, req.ID)
	}
	if req.CTA_Link != nil {
		db.Exec("UPDATE admins SET ctalink = ? WHERE id = ?", *req.CTA_Link, req.ID)
	}
	if req.Essential != nil {
		db.Exec("UPDATE admins SET essential = ? WHERE id = ?", *req.Essential, req.ID)
	}
	if req.Template != nil {
		db.Exec("UPDATE admins SET template = ? WHERE id = ?", *req.Template, req.ID)
	}
	if req.Editprotected != nil {
		db.Exec("UPDATE admins SET editprotected = ? WHERE id = ?", *req.Editprotected, req.ID)
	}

	// Track who edited and when.
	db.Exec("UPDATE admins SET editedat = NOW(), editedby = ? WHERE id = ?", myid, req.ID)

	return c.JSON(fiber.Map{"success": true})
}

type DeleteAdminRequest struct {
	ID uint64 `json:"id"`
}

// DeleteAdmin handles DELETE /admin - delete an admin.
//
// @Summary Delete an admin message
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /modtools/admin [delete]
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
