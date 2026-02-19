package tryst

import (
	"strconv"

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

// GetTryst handles GET /tryst - list user's trysts or single by ID.
//
// @Summary Get trysts
// @Tags tryst
// @Produce json
// @Param id query integer false "Tryst ID for single"
// @Success 200 {object} map[string]interface{}
// @Router /api/tryst [get]
func GetTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	db := database.DBConn
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)

	if id > 0 {
		// Single tryst.
		var t Tryst
		db.Raw("SELECT * FROM trysts WHERE id = ?", id).Scan(&t)
		if !canSee(myid, &t) {
			return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
		}

		return c.JSON(fiber.Map{
			"ret":    0,
			"status": "Success",
			"tryst": fiber.Map{
				"id":          t.ID,
				"user1":       t.User1,
				"user2":       t.User2,
				"arrangedat":  t.Arrangedat,
				"arrangedfor": t.Arrangedfor,
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
			"id":          t.ID,
			"user1":       t.User1,
			"user2":       t.User2,
			"arrangedat":  t.Arrangedat,
			"arrangedfor": t.Arrangedfor,
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
// @Router /api/tryst [put]
func CreateTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type CreateRequest struct {
		User1       uint64 `json:"user1"`
		User2       uint64 `json:"user2"`
		Arrangedfor string `json:"arrangedfor"`
	}

	var req CreateRequest
	if c.Get("Content-Type") == "application/json" {
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
		return c.JSON(fiber.Map{"ret": 3, "status": "Invalid parameters"})
	}
	if req.User1 == req.User2 {
		return c.JSON(fiber.Map{"ret": 3, "status": "Invalid parameters"})
	}

	db := database.DBConn
	result := db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE arrangedat = NOW()",
		req.User1, req.User2, req.Arrangedfor)

	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 1, "status": "Create failed"})
	}

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchTryst handles PATCH /tryst to update arrangedfor.
//
// @Summary Update tryst
// @Tags tryst
// @Accept json
// @Produce json
// @Router /api/tryst [patch]
func PatchTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type PatchRequest struct {
		ID          uint64 `json:"id"`
		Arrangedfor string `json:"arrangedfor"`
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
	var t Tryst
	db.Raw("SELECT * FROM trysts WHERE id = ?", req.ID).Scan(&t)

	if !canSee(myid, &t) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
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
// @Router /api/tryst [post]
func PostTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type ActionRequest struct {
		ID      uint64 `json:"id"`
		Confirm bool   `json:"confirm"`
		Decline bool   `json:"decline"`
	}

	var req ActionRequest
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
	var t Tryst
	db.Raw("SELECT * FROM trysts WHERE id = ?", req.ID).Scan(&t)

	if !canSee(myid, &t) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
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
// @Router /api/tryst [delete]
func DeleteTryst(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing id"})
	}

	db := database.DBConn
	var t Tryst
	db.Raw("SELECT * FROM trysts WHERE id = ?", id).Scan(&t)

	if !canSee(myid, &t) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	db.Exec("DELETE FROM trysts WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
