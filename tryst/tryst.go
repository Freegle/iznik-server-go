package tryst

import (
	"strings"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type Tryst struct {
	ID             uint64  `json:"id" gorm:"primary_key"`
	User1          uint64  `json:"user1"`
	User2          uint64  `json:"user2"`
	Arrangedat     string  `json:"arrangedat"`
	Arrangedfor    *string `json:"arrangedfor"`
	User1confirmed *string `json:"user1confirmed"`
	User2confirmed *string `json:"user2confirmed"`
	User1declined  *string `json:"user1declined"`
	User2declined  *string `json:"user2declined"`
}

// canSee checks if a user is one of the two participants.
func canSee(myid uint64, t *Tryst) bool {
	return t.ID > 0 && (t.User1 == myid || t.User2 == myid)
}

// calendarLink generates a Google Calendar link for a tryst.
// The link creates a 1-hour event starting at the arrangedfor time.
func calendarLink(arrangedfor *string) string {
	if arrangedfor == nil || *arrangedfor == "" {
		return ""
	}

	// GORM may return datetime as either "2006-01-02 15:04:05" or "2006-01-02T15:04:05Z".
	t, err := time.Parse("2006-01-02 15:04:05", *arrangedfor)
	if err != nil {
		t, err = time.Parse(time.RFC3339, *arrangedfor)
		if err != nil {
			return ""
		}
	}

	start := t.UTC().Format("20060102T150405Z")
	end := t.Add(time.Hour).UTC().Format("20060102T150405Z")

	return fmt.Sprintf(
		"https://www.google.com/calendar/render?action=TEMPLATE&text=%s&dates=%s/%s&details=%s&sf=true&output=xml",
		url.QueryEscape("Freegle Handover"),
		start,
		end,
		url.QueryEscape("Arrange handover of Freegle item"),
	)
}

// GetTryst handles GET /tryst - list user's trysts or single by ID.
//
// @Summary Get trysts
// @Tags tryst
// @Produce json
// @Param id query integer false "Tryst ID for single"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/tryst [get]
func GetTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)

	if id > 0 {
		// Single tryst.
		var t Tryst
		db.Raw("SELECT * FROM trysts WHERE id = ?", id).Scan(&t)
		if !canSee(myid, &t) {
			return fiber.NewError(fiber.StatusForbidden, "Permission denied")
		}

		return c.JSON(fiber.Map{
			"ret":    0,
			"status": "Success",
			"tryst": fiber.Map{
				"id":           t.ID,
				"user1":        t.User1,
				"user2":        t.User2,
				"arrangedat":   t.Arrangedat,
				"arrangedfor":  t.Arrangedfor,
				"calendarLink": calendarLink(t.Arrangedfor),
			},
		})
	}

	// List all future trysts for user.
	var trysts []Tryst
	db.Raw("SELECT * FROM trysts WHERE (user1 = ? OR user2 = ?) AND arrangedfor >= NOW()",
		myid, myid).Scan(&trysts)

	result := make([]map[string]interface{}, len(trysts))
	for i, t := range trysts {
		result[i] = map[string]interface{}{
			"id":           t.ID,
			"user1":        t.User1,
			"user2":        t.User2,
			"arrangedat":   t.Arrangedat,
			"arrangedfor":  t.Arrangedfor,
			"calendarLink": calendarLink(t.Arrangedfor),
		}
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"trysts": result,
	})
}

// CreateTryst handles PUT /tryst to create a new tryst.
//
// @Summary Create tryst
// @Tags tryst
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/tryst [put]
func CreateTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type CreateRequest struct {
		User1       uint64 `json:"user1"`
		User2       uint64 `json:"user2"`
		Arrangedfor string `json:"arrangedfor"`
	}

	var req CreateRequest
	if strings.Contains(c.Get("Content-Type"), "application/json") {
		c.BodyParser(&req)
	}
	if req.User1 == 0 {
		req.User1, _ = strconv.ParseUint(c.FormValue("user1", c.Query("user1", "0")), 10, 64)
	}
	if req.User2 == 0 {
		req.User2, _ = strconv.ParseUint(c.FormValue("user2", c.Query("user2", "0")), 10, 64)
	}
	if req.Arrangedfor == "" {
		req.Arrangedfor = c.FormValue("arrangedfor", c.Query("arrangedfor", ""))
	}

	if req.User1 == 0 || req.User2 == 0 || req.Arrangedfor == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
	}
	if req.User1 == req.User2 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid parameters")
	}

	// Caller must be one of the two participants.
	if myid != req.User1 && myid != req.User2 {
		return fiber.NewError(fiber.StatusForbidden, "Must be a participant")
	}

	db := database.DBConn

	// Verify a chat exists between the two users.
	var chatCount int64
	db.Raw("SELECT COUNT(*) FROM chat_rooms WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)",
		req.User1, req.User2, req.User2, req.User1).Scan(&chatCount)
	if chatCount == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "No chat exists between these users")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	sqlResult, err := sqlDB.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE arrangedat = NOW()",
		req.User1, req.User2, req.Arrangedfor)

	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Create failed")
	}

	newIDInt, _ := sqlResult.LastInsertId()
	newID := uint64(newIDInt)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchTryst handles PATCH /tryst to update arrangedfor.
//
// @Summary Update tryst
// @Tags tryst
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/tryst [patch]
func PatchTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type PatchRequest struct {
		ID          uint64 `json:"id"`
		Arrangedfor string `json:"arrangedfor"`
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

	db := database.DBConn
	var t Tryst
	db.Raw("SELECT * FROM trysts WHERE id = ?", req.ID).Scan(&t)

	if !canSee(myid, &t) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	if req.Arrangedfor != "" {
		db.Exec("UPDATE trysts SET arrangedfor = ? WHERE id = ?", req.Arrangedfor, req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// PostTryst handles POST /tryst for confirm/decline actions.
//
// @Summary Confirm or decline tryst
// @Tags tryst
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/tryst [post]
func PostTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type ActionRequest struct {
		ID      uint64 `json:"id"`
		Confirm bool   `json:"confirm"`
		Decline bool   `json:"decline"`
	}

	var req ActionRequest
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
	var t Tryst
	db.Raw("SELECT * FROM trysts WHERE id = ?", req.ID).Scan(&t)

	if !canSee(myid, &t) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	// Determine which user column to update.
	isUser1 := t.User1 == myid

	if req.Confirm {
		if isUser1 {
			db.Exec("UPDATE trysts SET user1confirmed = NOW() WHERE id = ?", req.ID)
		} else {
			db.Exec("UPDATE trysts SET user2confirmed = NOW() WHERE id = ?", req.ID)
		}
	}

	if req.Decline {
		if isUser1 {
			db.Exec("UPDATE trysts SET user1declined = NOW() WHERE id = ?", req.ID)
		} else {
			db.Exec("UPDATE trysts SET user2declined = NOW() WHERE id = ?", req.ID)
		}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteTryst handles DELETE /tryst.
//
// @Summary Delete tryst
// @Tags tryst
// @Produce json
// @Param id query integer true "Tryst ID"
// @Security BearerAuth
// @Router /api/tryst [delete]
func DeleteTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Accept id from query string or JSON body (frontend sends DELETE with JSON body).
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		type DeleteRequest struct {
			ID uint64 `json:"id"`
		}
		var req DeleteRequest
		if err := c.BodyParser(&req); err == nil {
			id = req.ID
		}
	}
	if id == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing id")
	}

	db := database.DBConn
	var t Tryst
	db.Raw("SELECT * FROM trysts WHERE id = ?", id).Scan(&t)

	if !canSee(myid, &t) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	db.Exec("DELETE FROM trysts WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
