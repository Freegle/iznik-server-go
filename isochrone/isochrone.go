package isochrone

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
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

func (Isochrones) TableName() string {
	return "isochrones"
}

// validTransports is the whitelist of allowed transport types.
var validTransports = map[string]bool{
	"Walk":  true,
	"Cycle": true,
	"Drive": true,
}

func ListIsochrones(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	fmt.Printf("[ListIsochrones] user=%d method=%s url=%s\n", myid, c.Method(), c.OriginalURL())

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	isochrones := []Isochrones{}

	db.Raw("SELECT isochrones_users.id, isochroneid, userid, timestamp, nickname, locationid, transport, minutes, ST_AsText(polygon) AS polygon FROM isochrones_users INNER JOIN isochrones ON isochrones_users.isochroneid = isochrones.id WHERE isochrones_users.userid = ?", myid).Scan(&isochrones)

	if len(isochrones) == 0 {
		// Auto-create a default isochrone using the user's last known location
		// when none exist.
		var locationid uint64
		db.Raw("SELECT lastlocation FROM users WHERE id = ? AND lastlocation IS NOT NULL", myid).Scan(&locationid)

		if locationid > 0 {
			// Find or create isochrone with default params (Walk, 15 minutes).
			var isoID uint64
			db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = 'Walk' AND minutes = 15",
				locationid).Scan(&isoID)

			if isoID == 0 {
				// Use the location's own geometry as placeholder polygon.
				// For postcodes this is a real POLYGON; background job replaces with actual isochrone contour.
				result := db.Exec("INSERT INTO isochrones (locationid, transport, minutes, polygon) "+
					"SELECT ?, 'Walk', 15, geometry FROM locations WHERE id = ?",
					locationid, locationid)
				if result.Error != nil {
					log.Printf("Failed to auto-create isochrone for user %d location %d: %v", myid, locationid, result.Error)
					return c.JSON(isochrones)
				}
				db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = 'Walk' AND minutes = 15 ORDER BY id DESC LIMIT 1",
					locationid).Scan(&isoID)
			}

			if isoID > 0 {
				// Link user to isochrone.
				result := db.Exec("INSERT INTO isochrones_users (userid, isochroneid) VALUES (?, ?) "+
					"ON DUPLICATE KEY UPDATE isochroneid = VALUES(isochroneid)",
					myid, isoID)
				if result.Error != nil {
					log.Printf("Failed to link user %d to isochrone %d: %v", myid, isoID, result.Error)
				}

				// Re-fetch the isochrones.
				db.Raw("SELECT isochrones_users.id, isochroneid, userid, timestamp, nickname, locationid, transport, minutes, ST_AsText(polygon) AS polygon FROM isochrones_users INNER JOIN isochrones ON isochrones_users.isochroneid = isochrones.id WHERE isochrones_users.userid = ?", myid).Scan(&isochrones)
			}
		}
	}

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
	if strings.Contains(c.Get("Content-Type"), "application/json") {
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
		}
	}
	if req.Transport == "" {
		req.Transport = c.FormValue("transport", c.Query("transport", "Walk"))
	}
	if req.Minutes == 0 {
		req.Minutes, _ = strconv.Atoi(c.FormValue("minutes", c.Query("minutes", "15")))
	}
	if req.Locationid == 0 {
		req.Locationid, _ = strconv.ParseUint(c.FormValue("locationid", c.Query("locationid", "0")), 10, 64)
	}
	if req.Nickname == "" {
		req.Nickname = c.FormValue("nickname", c.Query("nickname", ""))
	}

	// Validate transport against whitelist.
	if !validTransports[req.Transport] {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid transport - must be Walk, Cycle, or Drive")
	}

	// Clamp minutes.
	if req.Minutes < minMinutes {
		req.Minutes = minMinutes
	}
	if req.Minutes > maxMinutes {
		req.Minutes = maxMinutes
	}

	if req.Locationid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing locationid")
	}

	db := database.DBConn

	// Validate location exists.
	var locCount int64
	db.Raw("SELECT COUNT(*) FROM locations WHERE id = ?", req.Locationid).Scan(&locCount)
	if locCount == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Location not found")
	}

	// Find existing isochrone or create one (without polygon - background job fills it).
	var isoID uint64
	db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = ? AND minutes = ?",
		req.Locationid, req.Transport, req.Minutes).Scan(&isoID)

	if isoID == 0 {
		// Use the location's own geometry as placeholder polygon.
		result := db.Exec("INSERT INTO isochrones (locationid, transport, minutes, polygon) "+
			"SELECT ?, ?, ?, geometry FROM locations WHERE id = ?",
			req.Locationid, req.Transport, req.Minutes, req.Locationid)
		if result.Error != nil {
			log.Printf("Failed to create isochrone for location %d: %v", req.Locationid, result.Error)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create isochrone")
		}
		db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = ? AND minutes = ? ORDER BY id DESC LIMIT 1",
			req.Locationid, req.Transport, req.Minutes).Scan(&isoID)
	}

	// Link user to isochrone (upsert).
	db.Exec("INSERT INTO isochrones_users (userid, isochroneid, nickname) VALUES (?, ?, ?) "+
		"ON DUPLICATE KEY UPDATE nickname = VALUES(nickname)",
		myid, isoID, req.Nickname)

	var newID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? AND isochroneid = ? ORDER BY id DESC LIMIT 1",
		myid, isoID).Scan(&newID)

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

	fmt.Printf("[EditIsochrone] user=%d method=%s url=%s body=%q\n", myid, c.Method(), c.OriginalURL(), string(c.Body()))

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type EditRequest struct {
		ID        uint64 `json:"id"`
		Minutes   int    `json:"minutes"`
		Transport string `json:"transport"`
	}

	var req EditRequest
	if strings.Contains(c.Get("Content-Type"), "application/json") {
		// BodyParser uses strict typing and fails if the frontend sends numeric
		// fields as strings (e.g. "15" instead of 15). Try it first, then fall
		// back to a flexible map-based parse for any fields it missed.
		_ = c.BodyParser(&req)

		if req.ID == 0 || req.Minutes == 0 || req.Transport == "" {
			var raw map[string]interface{}
			if json.Unmarshal(c.Body(), &raw) == nil {
				if req.ID == 0 {
					if v, ok := raw["id"]; ok {
						req.ID, _ = strconv.ParseUint(fmt.Sprintf("%v", v), 10, 64)
					}
				}
				if req.Minutes == 0 {
					if v, ok := raw["minutes"]; ok {
						req.Minutes, _ = strconv.Atoi(fmt.Sprintf("%v", v))
					}
				}
				if req.Transport == "" {
					if v, ok := raw["transport"]; ok {
						req.Transport = fmt.Sprintf("%v", v)
					}
				}
			}
		}
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
		return fiber.NewError(fiber.StatusBadRequest, "Missing id")
	}

	// Validate transport if provided - must be Walk/Cycle/Drive.
	if req.Transport != "" && !validTransports[req.Transport] {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid transport - must be Walk, Cycle, or Drive")
	}

	if req.Minutes < minMinutes {
		req.Minutes = minMinutes
	}
	if req.Minutes > maxMinutes {
		req.Minutes = maxMinutes
	}

	db := database.DBConn

	// Get current isochrone to find locationid and current transport.
	var current struct {
		Locationid uint64
		Userid     uint64
		Transport  string
	}
	db.Raw("SELECT isochrones.locationid, isochrones_users.userid, isochrones.transport "+
		"FROM isochrones_users "+
		"INNER JOIN isochrones ON isochrones.id = isochrones_users.isochroneid "+
		"WHERE isochrones_users.id = ?", req.ID).Scan(&current)

	if current.Locationid == 0 {
		// Row not found — may have been deleted by a duplicate-key cleanup in a prior
		// request, or by a cascade delete. If the user still has other isochrones the
		// desired state is already correct; return success so the frontend re-fetches
		// cleanly instead of surfacing a 404 error.
		var existingCount int64
		db.Raw("SELECT COUNT(*) FROM isochrones_users WHERE userid = ?", myid).Scan(&existingCount)
		if existingCount > 0 {
			return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
		}
		return fiber.NewError(fiber.StatusNotFound, "Not found")
	}

	if current.Userid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	// Fall back to current transport if not provided (handles historical NULL transport rows).
	if req.Transport == "" {
		req.Transport = current.Transport
	}
	if req.Transport == "" {
		req.Transport = "Walk" // Ultimate fallback for NULL transport in DB.
	}

	// Find or create isochrone with new params.
	var isoID uint64
	db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = ? AND minutes = ?",
		current.Locationid, req.Transport, req.Minutes).Scan(&isoID)

	if isoID == 0 {
		// Use the location's own geometry as placeholder polygon.
		// Fall back to a point geometry if the location has no geometry data.
		result := db.Exec("INSERT INTO isochrones (locationid, transport, minutes, polygon) "+
			"SELECT ?, ?, ?, COALESCE(geometry, ST_GeomFromText('POINT(0 0)', 3857)) FROM locations WHERE id = ?",
			current.Locationid, req.Transport, req.Minutes, current.Locationid)
		if result.Error != nil {
			log.Printf("Failed to create isochrone for edit: %v", result.Error)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to create isochrone")
		}
		db.Raw("SELECT id FROM isochrones WHERE locationid = ? AND transport = ? AND minutes = ? ORDER BY id DESC LIMIT 1",
			current.Locationid, req.Transport, req.Minutes).Scan(&isoID)
	}

	// Update the link to point to the new isochrone.
	result := db.Exec("UPDATE isochrones_users SET isochroneid = ? WHERE id = ?", isoID, req.ID)
	if result.Error != nil {
		// Handle duplicate entry (timing window).
		log.Printf("Failed to update isochrone link %d, deleting duplicate: %v", req.ID, result.Error)
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
		return fiber.NewError(fiber.StatusBadRequest, "Missing id")
	}

	db := database.DBConn

	// Verify ownership: the isochrones_users record must belong to the current user.
	var count int64
	db.Raw("SELECT COUNT(*) FROM isochrones_users WHERE id = ? AND userid = ?", id, myid).Scan(&count)
	if count == 0 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Access denied"})
	}

	db.Exec("DELETE FROM isochrones_users WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
