package export

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// PostExport creates a new GDPR data export request.
//
// @Summary Request data export
// @Tags export
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/export [post]
func PostExport(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	// Generate a 64-char random tag for security.
	tagBytes := make([]byte, 32)
	rand.Read(tagBytes)
	tag := hex.EncodeToString(tagBytes)

	db := database.DBConn
	result := db.Exec("INSERT INTO users_exports (userid, tag) VALUES (?, ?)", myid, tag)
	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 2, "status": "Failed to create export"})
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"id":     id,
		"tag":    tag,
	})
}

// GetExport returns the status (and data if complete) of a GDPR export.
//
// @Summary Get export status
// @Tags export
// @Produce json
// @Param id query integer true "Export ID"
// @Param tag query string true "Export tag"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/export [get]
func GetExport(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	tag := c.Query("tag", "")

	if id == 0 || tag == "" {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing id or tag"})
	}

	db := database.DBConn

	type ExportRow struct {
		ID        uint64  `json:"id"`
		Userid    uint64  `json:"userid"`
		Requested *string `json:"requested"`
		Started   *string `json:"started"`
		Completed *string `json:"completed"`
		Data      []byte  `json:"-"`
	}

	var row ExportRow
	db.Raw("SELECT id, userid, requested, started, completed, data FROM users_exports WHERE userid = ? AND id = ? AND tag = ?",
		myid, id, tag).Scan(&row)

	if row.ID == 0 {
		return c.JSON(fiber.Map{"ret": 3, "status": "Export not found"})
	}

	if row.Completed != nil && len(row.Data) > 0 {
		// Data is gzdeflate (raw DEFLATE) compressed JSON. Decompress it.
		reader := flate.NewReader(bytes.NewReader(row.Data))
		defer reader.Close()

		inflated, err := io.ReadAll(reader)
		if err == nil {
			// The inflated data is JSON text. Use json.RawMessage to avoid double-encoding.
			return c.JSON(fiber.Map{
				"ret":    0,
				"status": "Success",
				"export": fiber.Map{
					"id":        row.ID,
					"requested": row.Requested,
					"started":   row.Started,
					"completed": row.Completed,
					"data":      json.RawMessage(inflated),
					"infront":   0,
				},
			})
		}
	}

	// Not completed yet — return queue position.
	var infront int64
	db.Raw("SELECT COUNT(*) FROM users_exports WHERE id < ? AND completed IS NULL", id).Scan(&infront)

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"export": fiber.Map{
			"id":        row.ID,
			"requested": row.Requested,
			"started":   row.Started,
			"completed": row.Completed,
			"infront":   infront,
		},
	})
}
