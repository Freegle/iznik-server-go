package noticeboard

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

type PostNoticeboardRequest struct {
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Active      *bool    `json:"active"`
	Action      string   `json:"action"`
	ID          uint64   `json:"id"`
	Comments    *string  `json:"comments"`
}

type PatchNoticeboardRequest struct {
	ID            uint64   `json:"id"`
	Name          *string  `json:"name"`
	Lat           *float64 `json:"lat"`
	Lng           *float64 `json:"lng"`
	Description   *string  `json:"description"`
	Active        *bool    `json:"active"`
	Lastcheckedat *string  `json:"lastcheckedat"`
	Photoid       *uint64  `json:"photoid"`
}

func PostNoticeboard(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	var req PostNoticeboardRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	if req.Action != "" {
		// Action on existing noticeboard
		if req.ID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "id is required for action")
		}

		switch req.Action {
		case "Refreshed":
			db.Exec("INSERT INTO noticeboards_checks (noticeboardid, userid, checkedat, refreshed, inactive) VALUES (?, ?, NOW(), 1, 0)", req.ID, myid)
			db.Exec("UPDATE noticeboards SET lastcheckedat = NOW(), active = 1 WHERE id = ?", req.ID)
		case "Declined":
			db.Exec("INSERT INTO noticeboards_checks (noticeboardid, userid, checkedat, declined, inactive) VALUES (?, ?, NOW(), 1, 0)", req.ID, myid)
		case "Inactive":
			db.Exec("INSERT INTO noticeboards_checks (noticeboardid, userid, checkedat, inactive) VALUES (?, ?, NOW(), 1)", req.ID, myid)
			db.Exec("UPDATE noticeboards SET lastcheckedat = NOW(), active = 0 WHERE id = ?", req.ID)
		case "Comments":
			comments := ""
			if req.Comments != nil {
				comments = *req.Comments
			}
			db.Exec("INSERT INTO noticeboards_checks (noticeboardid, userid, checkedat, comments, inactive) VALUES (?, ?, NOW(), ?, 0)", req.ID, myid, comments)
		default:
			return fiber.NewError(fiber.StatusBadRequest, "Unknown action")
		}

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	// Create new noticeboard
	if req.Lat == nil || req.Lng == nil {
		return fiber.NewError(fiber.StatusBadRequest, "lat and lng are required")
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	name := ""
	if req.Name != nil {
		name = *req.Name
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	srid := utils.SRID
	pointSQL := fmt.Sprintf("ST_GeomFromText('POINT(%f %f)', %d)", *req.Lng, *req.Lat, srid)

	// Use NULL for addedby when user is not logged in (myid=0) to satisfy FK constraint.
	var addedby interface{}
	if myid > 0 {
		addedby = myid
	}

	result := db.Exec(
		"INSERT INTO noticeboards (`name`, `lat`, `lng`, `position`, `added`, `addedby`, `description`, `active`, `lastcheckedat`) "+
			"VALUES (?, ?, ?, "+pointSQL+", NOW(), ?, ?, ?, NOW())",
		name, *req.Lat, *req.Lng, addedby, description, active)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Create failed")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": id})
}

func PatchNoticeboard(c *fiber.Ctx) error {
	var req PatchNoticeboardRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	// Check noticeboard exists and get current name for newsfeed trigger
	var currentName string
	var count int64
	db.Raw("SELECT COUNT(*), COALESCE(name, '') FROM noticeboards WHERE id = ?", req.ID).Row().Scan(&count, &currentName)
	if count == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Noticeboard not found")
	}

	// Update settable attributes
	if req.Name != nil {
		db.Exec("UPDATE noticeboards SET name = ? WHERE id = ?", *req.Name, req.ID)
	}
	if req.Lat != nil {
		db.Exec("UPDATE noticeboards SET lat = ? WHERE id = ?", *req.Lat, req.ID)
	}
	if req.Lng != nil {
		db.Exec("UPDATE noticeboards SET lng = ? WHERE id = ?", *req.Lng, req.ID)
	}
	if req.Description != nil {
		db.Exec("UPDATE noticeboards SET description = ? WHERE id = ?", *req.Description, req.ID)
	}
	if req.Active != nil {
		db.Exec("UPDATE noticeboards SET active = ? WHERE id = ?", *req.Active, req.ID)
	}
	if req.Lastcheckedat != nil {
		db.Exec("UPDATE noticeboards SET lastcheckedat = ? WHERE id = ?", *req.Lastcheckedat, req.ID)
	}

	// Link photo if provided
	if req.Photoid != nil {
		db.Exec("UPDATE noticeboards_images SET noticeboardid = ? WHERE id = ?", req.ID, *req.Photoid)
	}

	// Create newsfeed entry on first name assignment (when name was empty and is now being set)
	if req.Name != nil && currentName == "" && *req.Name != "" {
		isActive := true
		if req.Active != nil {
			isActive = *req.Active
		}

		if isActive {
			// Get the noticeboard data for the newsfeed entry
			var addedby uint64
			var lat, lng float64
			db.Raw("SELECT COALESCE(addedby, 0), COALESCE(lat, 0), COALESCE(lng, 0) FROM noticeboards WHERE id = ?", req.ID).Row().Scan(&addedby, &lat, &lng)

			if addedby > 0 {
				// Create newsfeed entry - use TYPE_NOTICEBOARD = 20 (from PHP Newsfeed class)
				db.Exec(
					fmt.Sprintf("INSERT INTO newsfeed (type, userid, message, added, position) VALUES ('Noticeboard', ?, ?, NOW(), ST_GeomFromText('POINT(%f %f)', %d))",
						lng, lat, utils.SRID),
					addedby, fmt.Sprintf(`{"id":%d,"name":"%s"}`, req.ID, *req.Name))
			}
		}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": req.ID})
}
