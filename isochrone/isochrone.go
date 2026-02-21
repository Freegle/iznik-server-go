package isochrone

import (
	"strconv"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type Isochrones struct {
	ID          uint64    `json:"id" gorm:"primary_key"`
	Userid      uint64    `json:"userid"`
	Isochroneid uint64    `json:"isochroneid"`
	Locationid  uint64    `json:"locationid"`
	Transport   string    `json:"transport"`
	Minutes     int       `json:"minutes"`
	Timestamp   time.Time `json:"timestamp"`
	Nickname    string    `json:"nickname"`
	Polygon     string    `json:"polygon"`
}

func ListIsochrones(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	isochrones := []Isochrones{}

	db.Raw("SELECT isochrones_users.id, isochroneid, userid, timestamp, nickname, locationid, transport, minutes, ST_AsText(polygon) AS polygon FROM isochrones_users INNER JOIN isochrones ON isochrones_users.isochroneid = isochrones.id WHERE isochrones_users.userid = ?", myid).Scan(&isochrones)
	return c.JSON(isochrones)
}

const minMinutes = 5
const maxMinutes = 45

// CreateIsochrone handles PUT /isochrone to create or link an isochrone for the user.
//
// @Summary Create isochrone
// @Tags isochrone
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/isochrone [put]
func CreateIsochrone(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type CreateRequest struct {
		Transport  string `json:"transport"`
		Minutes    int    `json:"minutes"`
		Nickname   string `json:"nickname"`
		Locationid uint64 `json:"locationid"`
	}

	var req CreateRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.Transport == "" {
		req.Transport = c.FormValue("transport", c.Query("transport", "Walk"))
	}
	if req.Minutes == 0 {
		req.Minutes, _ = strconv.Atoi(c.FormValue("minutes", c.Query("minutes", "30")))
	}
	if req.Locationid == 0 {
		req.Locationid, _ = strconv.ParseUint(c.FormValue("locationid", c.Query("locationid", "0")), 10, 64)
	}
	if req.Nickname == "" {
		req.Nickname = c.FormValue("nickname", c.Query("nickname", ""))
	}

	// Clamp minutes.
	if req.Minutes < minMinutes {
		req.Minutes = minMinutes
	}
	if req.Minutes > maxMinutes {
		req.Minutes = maxMinutes
	}

	if req.Locationid == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing locationid"})
	}

	db := database.DBConn

	// Validate location exists.
	var locCount int64
	db.Raw("SELECT COUNT(*) FROM locations WHERE id = ?", req.Locationid).Scan(&locCount)
	if locCount == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Location not found"})
	}

	// Find existing isochrone or create one (without polygon - background job fills it).
	var isoID uint64
	db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = ? AND minutes = ?",
		req.Locationid, req.Transport, req.Minutes).Scan(&isoID)

	if isoID == 0 {
		result := db.Exec("INSERT INTO isochrones (locationid, transport, minutes, polygon) VALUES (?, ?, ?, ST_GeomFromText('POINT(0 0)'))",
			req.Locationid, req.Transport, req.Minutes)
		if result.Error != nil {
			return c.JSON(fiber.Map{"ret": 1, "status": "Failed to create isochrone"})
		}
		db.Raw("SELECT LAST_INSERT_ID()").Scan(&isoID)
	}

	// Link user to isochrone (upsert).
	db.Exec("INSERT INTO isochrones_users (userid, isochroneid, nickname) VALUES (?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE nickname = ?, id=LAST_INSERT_ID(id)",
		myid, isoID, req.Nickname, req.Nickname)

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// EditIsochrone handles PATCH /isochrone to update transport/minutes.
//
// @Summary Edit isochrone
// @Tags isochrone
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/isochrone [patch]
func EditIsochrone(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type EditRequest struct {
		ID        uint64 `json:"id"`
		Minutes   int    `json:"minutes"`
		Transport string `json:"transport"`
	}

	var req EditRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}
	if req.Minutes == 0 {
		req.Minutes, _ = strconv.Atoi(c.FormValue("minutes", c.Query("minutes", "0")))
	}
	if req.Transport == "" {
		req.Transport = c.FormValue("transport", c.Query("transport", ""))
	}

	if req.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing id"})
	}

	if req.Minutes < minMinutes {
		req.Minutes = minMinutes
	}
	if req.Minutes > maxMinutes {
		req.Minutes = maxMinutes
	}

	db := database.DBConn

	// Get current isochrone to find locationid.
	var current struct {
		Locationid uint64
		Userid     uint64
	}
	db.Raw("SELECT isochrones.locationid, isochrones_users.userid "+
		"FROM isochrones_users "+
		"INNER JOIN isochrones ON isochrones.id = isochrones_users.isochroneid "+
		"WHERE isochrones_users.id = ?", req.ID).Scan(&current)

	if current.Locationid == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Not found"})
	}

	// Find or create isochrone with new params.
	var isoID uint64
	db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = ? AND minutes = ?",
		current.Locationid, req.Transport, req.Minutes).Scan(&isoID)

	if isoID == 0 {
		db.Exec("INSERT INTO isochrones (locationid, transport, minutes, polygon) VALUES (?, ?, ?, ST_GeomFromText('POINT(0 0)'))",
			current.Locationid, req.Transport, req.Minutes)
		db.Raw("SELECT LAST_INSERT_ID()").Scan(&isoID)
	}

	// Update the link to point to the new isochrone.
	result := db.Exec("UPDATE isochrones_users SET isochroneid = ? WHERE id = ?", isoID, req.ID)
	if result.Error != nil {
		// Handle duplicate entry (timing window).
		db.Exec("DELETE FROM isochrones_users WHERE id = ?", req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteIsochrone handles DELETE /isochrone to remove user's isochrone link.
//
// @Summary Delete isochrone
// @Tags isochrone
// @Produce json
// @Param id query integer true "Isochrone user link ID"
// @Security BearerAuth
// @Router /api/isochrone [delete]
func DeleteIsochrone(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing id"})
	}

	db := database.DBConn

	// Verify ownership: the isochrones_users record must belong to the current user.
	var count int64
	db.Raw("SELECT COUNT(*) FROM isochrones_users WHERE id = ? AND userid = ?", id, myid).Scan(&count)
	if count == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Access denied"})
	}

	db.Exec("DELETE FROM isochrones_users WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
