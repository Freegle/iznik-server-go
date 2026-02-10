package spammers

import (
	"strconv"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// isModerator checks if user is a moderator of any group.
func isModerator(myid uint64) bool {
	var count int64
	database.DBConn.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&count)
	return count > 0
}

func getSystemRole(myid uint64) string {
	var role string
	database.DBConn.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
	return role
}

// GetSpammers handles GET /spammers with search and pagination.
//
// @Summary List spammers
// @Tags spammers
// @Produce json
// @Param collection query string false "Collection: Spammer, Whitelisted, PendingAdd, PendingRemove"
// @Param search query string false "Search term"
// @Param context query string false "Pagination context (last ID)"
// @Success 200 {object} map[string]interface{}
// @Router /api/spammers [get]
func GetSpammers(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
	}

	role := getSystemRole(myid)
	isAdmin := role == "Admin" || role == "Support"

	if !isAdmin && !isModerator(myid) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
	}

	db := database.DBConn
	collection := c.Query("collection", "")
	search := c.Query("search", "")
	contextID, _ := strconv.ParseUint(c.Query("context", "0"), 10, 64)

	where := []string{"1=1"}
	args := []interface{}{}

	if collection != "" {
		where = append(where, "spam_users.collection = ?")
		args = append(args, collection)
	}

	if contextID > 0 {
		where = append(where, "spam_users.id < ?")
		args = append(args, contextID)
	}

	query := "SELECT DISTINCT spam_users.* FROM spam_users " +
		"INNER JOIN users ON spam_users.userid = users.id "

	if search != "" {
		query += "LEFT JOIN users_emails ON users_emails.userid = spam_users.userid "
		searchLike := "%" + search + "%"
		where = append(where, "(users_emails.email LIKE ? OR users.fullname LIKE ?)")
		args = append(args, searchLike, searchLike)
	}

	query += "WHERE " + strings.Join(where, " AND ") +
		" ORDER BY spam_users.id DESC LIMIT 10"

	type SpamRow struct {
		ID         uint64  `json:"id"`
		Userid     uint64  `json:"userid"`
		Collection string  `json:"collection"`
		Reason     string  `json:"reason"`
		Added      string  `json:"added"`
		Byuserid   *uint64 `json:"byuserid"`
		Heldby     *uint64 `json:"heldby"`
		Heldat     *string `json:"heldat"`
	}

	var rows []SpamRow
	db.Raw(query, args...).Scan(&rows)

	result := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		entry := map[string]interface{}{
			"id":         r.ID,
			"userid":     r.Userid,
			"collection": r.Collection,
			"reason":     r.Reason,
			"added":      r.Added,
			"heldby":     r.Heldby,
			"heldat":     r.Heldat,
		}

		// Enrich user.
		var displayname, email string
		db.Raw("SELECT COALESCE(fullname, CONCAT(COALESCE(firstname,''), ' ', COALESCE(lastname,'')), 'Unknown') FROM users WHERE id = ?", r.Userid).Scan(&displayname)
		db.Raw("SELECT email FROM users_emails WHERE userid = ? LIMIT 1", r.Userid).Scan(&email)
		entry["user"] = map[string]interface{}{
			"id":          r.Userid,
			"displayname": strings.TrimSpace(displayname),
			"email":       email,
		}

		// Enrich byuser.
		if r.Byuserid != nil && *r.Byuserid > 0 {
			var byName string
			db.Raw("SELECT COALESCE(fullname, CONCAT(COALESCE(firstname,''), ' ', COALESCE(lastname,'')), 'Unknown') FROM users WHERE id = ?", *r.Byuserid).Scan(&byName)
			entry["byuser"] = map[string]interface{}{
				"id":          *r.Byuserid,
				"displayname": strings.TrimSpace(byName),
			}
		}

		result[i] = entry
	}

	ctx := map[string]interface{}{}
	if len(rows) > 0 {
		ctx["id"] = rows[len(rows)-1].ID
	}

	return c.JSON(fiber.Map{
		"ret":      0,
		"status":   "Success",
		"spammers": result,
		"context":  ctx,
	})
}

// PostSpammer handles POST /spammers to add a spammer entry.
//
// @Summary Add spammer
// @Tags spammers
// @Accept json
// @Produce json
// @Router /api/spammers [post]
func PostSpammer(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type AddRequest struct {
		Userid     uint64 `json:"userid"`
		Collection string `json:"collection"`
		Reason     string `json:"reason"`
	}

	var req AddRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.Userid == 0 {
		req.Userid, _ = strconv.ParseUint(c.FormValue("userid", c.Query("userid", "0")), 10, 64)
	}
	if req.Collection == "" {
		req.Collection = c.FormValue("collection", c.Query("collection", ""))
	}
	if req.Reason == "" {
		req.Reason = c.FormValue("reason", c.Query("reason", ""))
	}

	if req.Userid == 0 || req.Collection == "" {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid parameters"})
	}

	role := getSystemRole(myid)
	isAdmin := role == "Admin" || role == "Support"

	// Only admins can add directly as Spammer/Whitelisted. Anyone can report as PendingAdd.
	if !isAdmin && req.Collection != "PendingAdd" {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	db := database.DBConn
	result := db.Exec("REPLACE INTO spam_users (userid, collection, reason, byuserid, heldby, heldat) "+
		"VALUES (?, ?, ?, ?, NULL, NULL)",
		req.Userid, req.Collection, req.Reason, myid)

	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 1, "status": "Failed to add spammer"})
	}

	var newID uint64
	db.Raw("SELECT id FROM spam_users WHERE userid = ? ORDER BY id DESC LIMIT 1", req.Userid).Scan(&newID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchSpammer handles PATCH /spammers to update collection/reason.
//
// @Summary Update spammer
// @Tags spammers
// @Accept json
// @Produce json
// @Router /api/spammers [patch]
func PatchSpammer(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type PatchRequest struct {
		ID         uint64  `json:"id"`
		Collection string  `json:"collection"`
		Reason     string  `json:"reason"`
		Heldby     *uint64 `json:"heldby"`
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

	role := getSystemRole(myid)
	isAdmin := role == "Admin" || role == "Support"

	// Get current state.
	db := database.DBConn
	var current struct {
		Collection string
	}
	db.Raw("SELECT collection FROM spam_users WHERE id = ?", req.ID).Scan(&current)

	if current.Collection == "" {
		return c.JSON(fiber.Map{"ret": 2, "status": "Not found"})
	}

	// Permission: admins can do anything, moderators can only request removal.
	if !isAdmin {
		isMod := isModerator(myid)
		if !isMod {
			return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
		}
		// Moderators can only move Spammer -> PendingRemove.
		if !(current.Collection == "Spammer" && req.Collection == "PendingRemove") {
			return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
		}
	}

	// Build update.
	if req.Collection != "" {
		db.Exec("UPDATE spam_users SET collection = ?, reason = ?, byuserid = ?, "+
			"heldby = ?, heldat = CASE WHEN ? IS NOT NULL THEN NOW() ELSE NULL END "+
			"WHERE id = ?",
			req.Collection, req.Reason, myid, req.Heldby, req.Heldby, req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteSpammer handles DELETE /spammers (admin only).
//
// @Summary Delete spammer
// @Tags spammers
// @Produce json
// @Param id query integer true "Spammer record ID"
// @Router /api/spammers [delete]
func DeleteSpammer(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	role := getSystemRole(myid)
	isAdmin := role == "Admin" || role == "Support"
	if !isAdmin {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing id"})
	}

	db := database.DBConn
	db.Exec("DELETE FROM spam_users WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
