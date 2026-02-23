package export

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// maxDecompressedSize limits decompressed export data to 50MB to prevent memory exhaustion.
const maxDecompressedSize = 50 * 1024 * 1024

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
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Check for existing pending export to prevent abuse.
	db := database.DBConn
	var pendingCount int64
	db.Raw("SELECT COUNT(*) FROM users_exports WHERE userid = ? AND completed IS NULL", myid).Scan(&pendingCount)
	if pendingCount > 0 {
		return fiber.NewError(fiber.StatusConflict, "Export already in progress")
	}

	// Generate a 64-char random tag for security.
	tagBytes := make([]byte, 32)
	if _, err := rand.Read(tagBytes); err != nil {
		log.Printf("Failed to generate random tag for export: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create export")
	}
	tag := hex.EncodeToString(tagBytes)

	result := db.Exec("INSERT INTO users_exports (userid, tag) VALUES (?, ?)", myid, tag)
	if result.Error != nil {
		log.Printf("Failed to insert export for user %d: %v", myid, result.Error)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create export")
	}

	var id uint64
	db.Raw("SELECT id FROM users_exports WHERE userid = ? AND tag = ? ORDER BY id DESC LIMIT 1", myid, tag).Scan(&id)

	return c.JSON(fiber.Map{
		"id":  id,
		"tag": tag,
	})
}

// GetExport returns the status (and data if complete) of a GDPR export.
// The tag can be passed as a query parameter or Authorization header.
//
// @Summary Get export status
// @Tags export
// @Produce json
// @Param id query integer true "Export ID"
// @Param tag query string false "Export tag (prefer Authorization header)"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/export [get]
func GetExport(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)

	// Accept tag from either Authorization header or query parameter.
	tag := c.Get("X-Export-Tag")
	if tag == "" {
		tag = c.Query("tag", "")
	}

	if id == 0 || tag == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Missing id or tag")
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
		return fiber.NewError(fiber.StatusNotFound, "Export not found")
	}

	if row.Completed != nil && len(row.Data) > 0 {
		// Data is gzdeflate (raw DEFLATE) compressed JSON. Decompress with size limit.
		reader := flate.NewReader(bytes.NewReader(row.Data))
		defer reader.Close()

		inflated, err := io.ReadAll(io.LimitReader(reader, maxDecompressedSize))
		if err != nil {
			log.Printf("Failed to decompress export %d for user %d: %v", id, myid, err)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to read export data")
		}

		// The inflated data is JSON text. Use json.RawMessage to avoid double-encoding.
		return c.JSON(fiber.Map{
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

	// Not completed yet — return queue position.
	var infront int64
	db.Raw("SELECT COUNT(*) FROM users_exports WHERE id < ? AND completed IS NULL", id).Scan(&infront)

	return c.JSON(fiber.Map{
		"export": fiber.Map{
			"id":        row.ID,
			"requested": row.Requested,
			"started":   row.Started,
			"completed": row.Completed,
			"infront":   infront,
		},
	})
}
