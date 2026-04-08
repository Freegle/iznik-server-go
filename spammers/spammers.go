package spammers

import (
	"strconv"
	"strings"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)


// GetSpammers handles GET /spammers with search and pagination.
//
// @Summary List spammers
// @Tags spammers
// @Produce json
// @Param collection query string false "Collection: Spammer, Whitelisted, PendingAdd, PendingRemove"
// @Param search query string false "Search term"
// @Param context query string false "Pagination context (last ID)"
// @Param partner query string false "Partner API key (alternative to session auth)"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/spammers [get]
func GetSpammers(c *fiber.Ctx) error {
	// Partners with a valid key can access the spammer list without a user session.
	partner := c.Query("partner", "")
	if partner != "" {
		db := database.DBConn
		var partnerID uint64
		db.Raw("SELECT id FROM partners_keys WHERE `key` = ?", partner).Scan(&partnerID)

		if partnerID == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Invalid partner key")
		}
		// Valid partner — fall through to the query logic.
	} else {
		// Standard user authentication path.
		myid := user.WhoAmI(c)
		if myid == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Not moderator")
		}

		if !auth.IsSystemMod(myid) {
			// Return empty list for non-moderators rather than an error,
			// so ModTools pages degrade gracefully.
			return c.JSON(fiber.Map{
				"spammers": []fiber.Map{},
				"context":  fiber.Map{},
			})
		}
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

	useridFilter, _ := strconv.ParseUint(c.Query("userid", "0"), 10, 64)
	if useridFilter > 0 {
		where = append(where, "spam_users.userid = ?")
		args = append(args, useridFilter)
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

	if len(rows) == 0 {
		rows = make([]SpamRow, 0)
	}

	ctx := map[string]interface{}{}
	if len(rows) > 0 {
		ctx["id"] = rows[len(rows)-1].ID
	}

	return c.JSON(fiber.Map{
		"ret":      0,
		"status":   "Success",
		"spammers": rows,
		"context":  ctx,
	})
}

// PostSpammer handles POST /spammers to add a spammer entry.
//
// @Summary Add spammer
// @Tags spammers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/spammers [post]
func PostSpammer(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type AddRequest struct {
		Userid     uint64 `json:"userid"`
		Collection string `json:"collection"`
		Reason     string `json:"reason"`
	}

	var req AddRequest
	if strings.Contains(c.Get("Content-Type"), "application/json") {
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
		return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
	}

	isAdmin := user.IsAdminOrSupport(myid)
	hasSpamAdmin := auth.HasPermission(myid, auth.PERM_SPAM_ADMIN)

	// Only admins or SpamAdmin users can add directly as Spammer/Whitelisted.
	// Anyone can report as PendingAdd.
	if !isAdmin && !hasSpamAdmin && req.Collection != utils.SPAM_COLLECTION_PENDING_ADD {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	db := database.DBConn
	// Use the underlying sql.DB to get LastInsertId() directly from the MySQL protocol
	// response — never issue a separate SELECT LAST_INSERT_ID() as it's unsafe under
	// parallel load (GORM's connection pool may assign a different connection).
	sqlDB, err := db.DB()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	sqlResult, err := sqlDB.Exec("REPLACE INTO spam_users (userid, collection, reason, byuserid, heldby, heldat) "+
		"VALUES (?, ?, ?, ?, NULL, NULL)",
		req.Userid, req.Collection, req.Reason, myid)

	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to add spammer")
	}

	var newID uint64
	lastID, err := sqlResult.LastInsertId()
	if err == nil && lastID > 0 {
		newID = uint64(lastID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchSpammer handles PATCH /spammers to update collection/reason.
//
// @Summary Update spammer
// @Tags spammers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/spammers [patch]
func PatchSpammer(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type PatchRequest struct {
		ID         uint64  `json:"id"`
		Collection string  `json:"collection"`
		Reason     string  `json:"reason"`
		Heldby     *uint64 `json:"heldby"`
	}

	var req PatchRequest
	if strings.Contains(c.Get("Content-Type"), "application/json") {
		c.BodyParser(&req)
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing id")
	}

	isAdmin := user.IsAdminOrSupport(myid)
	hasSpamAdmin := auth.HasPermission(myid, auth.PERM_SPAM_ADMIN)

	// Get current state.
	db := database.DBConn
	var current struct {
		Collection string
	}
	db.Raw("SELECT collection FROM spam_users WHERE id = ?", req.ID).Scan(&current)

	if current.Collection == "" {
		return fiber.NewError(fiber.StatusNotFound, "Not found")
	}

	// Permission: admins and SpamAdmin users can do anything.
	// Regular system mods can only move Spammer -> PendingRemove.
	if !isAdmin && !hasSpamAdmin {
		if !auth.IsSystemMod(myid) {
			return fiber.NewError(fiber.StatusForbidden, "Permission denied")
		}
		// Moderators without SpamAdmin can only request removal.
		if !(current.Collection == utils.SPAM_COLLECTION_SPAMMER && req.Collection == utils.SPAM_COLLECTION_PENDING_REMOVE) {
			return fiber.NewError(fiber.StatusForbidden, "Permission denied")
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

// ExportSpammers returns all confirmed spammers with their email addresses.
// Used by partner services (e.g. Trash Nothing) to sync spammer lists.
//
// @Summary Export spammers
// @Tags spammers
// @Produce json
// @Param partner query string false "Partner API key"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/spammers/export [get]
func ExportSpammers(c *fiber.Ctx) error {
	// Accept either partner key or moderator session.
	partner := c.Query("partner", "")
	if partner != "" {
		db := database.DBConn
		var partnerID uint64
		db.Raw("SELECT id FROM partners_keys WHERE `key` = ?", partner).Scan(&partnerID)

		if partnerID == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Invalid partner key")
		}
	} else {
		myid := user.WhoAmI(c)
		if myid == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Not authorized")
		}
		if !auth.IsSystemMod(myid) {
			return fiber.NewError(fiber.StatusForbidden, "Not authorized")
		}
	}

	db := database.DBConn

	type ExportRow struct {
		ID     uint64 `json:"id"`
		Added  string `json:"added"`
		Reason string `json:"reason"`
		Email  string `json:"email"`
	}

	var rows []ExportRow
	db.Raw("SELECT spam_users.id, spam_users.added, reason, email FROM spam_users "+
		"INNER JOIN users_emails ON spam_users.userid = users_emails.userid "+
		"WHERE collection = ?", utils.SPAM_COLLECTION_SPAMMER).Scan(&rows)

	if rows == nil {
		rows = make([]ExportRow, 0)
	}

	return c.JSON(fiber.Map{
		"ret":      0,
		"status":   "Success",
		"spammers": rows,
	})
}

// DeleteSpammer handles DELETE /spammers (admin or SpamAdmin permission).
//
// @Summary Delete spammer
// @Tags spammers
// @Produce json
// @Param id query integer true "Spammer record ID"
// @Security BearerAuth
// @Router /api/spammers [delete]
func DeleteSpammer(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !user.IsAdminOrSupport(myid) && !auth.HasPermission(myid, auth.PERM_SPAM_ADMIN) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	type DeleteRequest struct {
		ID uint64 `json:"id"`
	}

	var req DeleteRequest
	if strings.Contains(c.Get("Content-Type"), "application/json") {
		c.BodyParser(&req)
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing id")
	}

	db := database.DBConn
	db.Exec("DELETE FROM spam_users WHERE id = ?", req.ID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
